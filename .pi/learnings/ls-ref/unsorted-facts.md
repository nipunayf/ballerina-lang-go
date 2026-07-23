# ls-ref completion architecture: learnings for ticket 36

> **Status**: Verified against actual code in `ls/ls/` and `ls/projects/`.
> This file collects raw findings; see `sorted-findings.md` for the structured
> report.

---

## 1. Core insight: indexes are built *during* compilation, not precomputed separately

The PoC does **not** maintain a persistent, incrementally-updated semantic index
for completion. Instead, all five completion indexes are built **during the
compile cycle** by walking the compiled package's syntax trees and resolved
symbols, then published as part of the `StableSnapshot`.

**Evidence**: `realCompilePackage()` in `ls/ls/core/compile/compile.go` (line ~280):

```go
func realCompilePackage(pkg *projects.Package) cycleResult {
    comp := pkg.Compilation()
    // ... diagnostics extraction ...
    return cycleResult{
        // ...
        completionIndex:       projects.BuildCompletionIndex(pkg),
        expectedTypeIndex:     projects.BuildExpectedTypeIndex(comp),
        importCatalog:         projects.BuildImportCatalog(pkg),
        memberCompletionIndex: projects.BuildMemberCompletionIndex(comp),
        invocationCompletionIndex: projects.BuildInvocationCompletionIndex(comp),
    }
}
```

This is the **single point of production** for all semantic completion data.
The indexes are built from the compiled package's AST and symbol spaces, then
copied into protocol-free DTOs and published with the stable snapshot.

---

## 2. The five completion indexes and how they're built

### 2a. `CompletionIndex` — module-level declaration facts

- **File**: `ls/projects/completion_index.go`
- **Builder**: `BuildCompletionIndex(pkg)` (line ~80)
- **What it does**: Walks every document's syntax tree (`ModulePart.Members()`),
  extracts `FunctionDefinition`, `TypeDefinitionNode`, `ConstantDeclarationNode`,
  `ModuleVariableDeclarationNode`, `EnumDeclarationNode`, `ClassDefinitionNode`
  as `CompletionFact` entries (label, kind, detail string).
- **Key design**: Facts are grouped by **module**, not by file. A file's facts
  include all sibling files' declarations in the same module. The `fileModule`
  map maps fileKey -> moduleKey.
- **Protocol boundary**: Only `CompletionFact` structs (Label, Kind, Detail
  strings) cross the boundary. No AST nodes, symbols, or compiler contexts.

### 2b. `ExpectedTypeIndex` — contextual expected-type projection

- **File**: `ls/projects/expected_type_index.go`
- **Builder**: `BuildExpectedTypeIndex(comp)` (line ~80)
- **What it does**: Reads resolver-captured slot records from
  `compilerCtx.ExpectedSlotRecords()`. For each slot (assignment, return,
  argument, mapping field, list member, etc.), it computes:
  - A display-safe type label
  - The set of module-level value candidates whose type is a subtype of the
    expected type (precomputed assignability)
- **Key design**: Facts are keyed by byte span; the query layer selects the
  **innermost** fact whose span contains the cursor offset.
- **Protocol boundary**: Only `ExpectedTypeFact` structs (Kind, Known, TypeLabel,
  StartOffset, EndOffset, ArgIndex, Compatible []string) cross the boundary.

### 2c. `MemberCompletionIndex` — member-access projection

- **File**: `ls/projects/member_completion_index.go`
- **Builder**: `BuildMemberCompletionIndex(comp)` (line ~100)
- **What it does**: Walks the resolved AST with `ast.Walk` for
  `BLangFieldBaseAccess` and `BLangRemoteMethodCallAction` nodes. For each
  field access, it resolves the receiver's determined type and enumerates
  accessible members (record fields, object fields/methods, remote methods).
- **Key design**: Slots are keyed by `(fileKey, accessKind, dotOffset)` — the
  exact byte offset of the dot uniquely identifies the access slot.
- **Protocol boundary**: Only `MemberAccessSlot` and `MemberCandidate` structs
  (strings, kinds, byte offsets, ranks) cross the boundary.

### 2d. `InvocationCompletionIndex` — callable catalog + argument relevance

- **File**: `ls/projects/invocation_completion_index.go`
- **Builder**: `BuildInvocationCompletionIndex(comp)` (line ~150)
- **What it does**: Walks the resolved AST for `BLangInvocation` and
  `BLangRemoteMethodCallAction` nodes. For each resolved call, it:
  - Captures the call's byte range (CallStart/CallEnd/NameStart/NameEnd)
  - Copies parameter facts (names, categories, type labels)
  - Precomputes **argument relevance tiers**: for each parameter position,
    which module-level candidates are directly assignable (Direct) vs.
    check-compatible (Check)
  - Builds a callable catalog of module-level functions with snippet forms
- **Key design**: The innermost slot (smallest range) wins for nested calls.
  Relevance tiers are precomputed at compile time using semtype subtype checks.
- **Protocol boundary**: Only `InvocationSlot`, `CallableEntry`, `ParamSlot`,
  `NamedArgEntry`, `ArgRelevance` structs cross the boundary.

### 2e. `ImportCatalog` — importable modules + alias exports

- **File**: `ls/projects/import_catalog.go`
- **Builder**: `BuildImportCatalog(pkg)` (line ~130)
- **What it does**: Lists embedded stdlib modules (from `lib/stdlibs/` embed FS),
  project modules (non-default modules), and per-file imported-alias public
  exports (copied from resolved public symbol spaces).
- **Key design**: Stdlib modules are cached once per process via `sync.Once`.
  Alias exports are copied from the compiled environment's `publicSymbols` map.
- **Protocol boundary**: Only `CatalogModule`, `AliasExport`, `CompletionFact`
  structs cross the boundary.

---

## 3. The non-blocking lease mechanism

This is the **most important pattern** for ticket 36. The query layer never
waits for compilation.

### 3a. Lease interface

**File**: `ls/ls/core/query/completion.go` (line ~30-50)

```go
type CompletionLease interface {
    Index() *projects.CompletionIndex
    ExpectedTypeIndex() *projects.ExpectedTypeIndex
    ImportCatalog() *projects.ImportCatalog
    MemberCompletionIndex() *projects.MemberCompletionIndex
    InvocationCompletionIndex() *projects.InvocationCompletionIndex
    Release()
}

type CompletionLeaser interface {
    Lease(root string, generation uint64) (CompletionLease, bool)
}
```

### 3b. Lease implementation in CompilationService

**File**: `ls/ls/core/compile/compile.go` (line ~200-220)

```go
func (s *CompilationService) Lease(root string, generation uint64) (CompletionLease, bool) {
    snap, release, ok := s.store.lease(root, generation)
    if !ok {
        return CompletionLease{}, false
    }
    return CompletionLease{...indexes from snap..., release: release}, true
}
```

### 3c. SnapshotStore.lease — the actual gate

**File**: `ls/ls/core/compile/snapshot.go` (line ~200-240)

The lease:
1. Checks `pendingEvict` — if the root is marked for eviction, no new leases
2. Checks `stable[root]` exists with matching generation
3. Increments `leaseCount[root]` to pin the snapshot
4. Returns a release function (idempotent via `sync.Once`) that decrements the
   count and finalizes deferred eviction if needed

### 3d. Staleness guarantee

**File**: `ls/ls/core/compile/completion_lease_test.go`

`TestCompletionLeaseNoStaleReuseAfterEdit` verifies: after `Supersede` bumps
the generation without publishing a new snapshot, completion cannot acquire the
stale prior-generation index. The old semantic facts are never combined with
current text.

---

## 4. How the query layer uses the lease

**File**: `ls/ls/core/query/completion.go` (line ~200-250, `completeFunctionBody`)

The function-body completion path:
1. Classifies cursor position (statement-start vs expression)
2. Collects current-syntax scope (parameters + preceding locals) — **always**
3. Adds fixed keyword/construct catalog — **always**
4. **Only then** tries to acquire a lease:
   ```go
   if s.compiler != nil {
       // get generation from workspace snapshot
       if lease, ok := s.compiler.Lease(root, gen); ok {
           // add semantic facts from index
           // boost expected-type compatible candidates
           // enrich callable snippets
           // boost invocation tiers
           // add named arguments
           lease.Release()
       }
   }
   ```
5. If no lease, falls back to syntax/static only — **never blocks**

This is the **fallback-first** pattern: current-syntax and static keywords are
always available; semantic facts are an optional enrichment.

---

## 5. Server boundary adapter

**File**: `ls/ls/server/completion.go`

The server adapts the concrete `*compile.CompilationService` to the query
layer's `CompletionLeaser` interface:

```go
type completionLeaseAdapter struct {
    c *compile.CompilationService
}

func (a completionLeaseAdapter) Lease(root string, generation uint64) (query.CompletionLease, bool) {
    lease, ok := a.c.Lease(root, generation)
    if !ok {
        return nil, false
    }
    return completionLease{lease: lease}, true
}
```

This keeps `ls/core/query` free of `compile` imports — a clean separation.

---

## 6. Prototype-quality shortcuts (do NOT copy as-is)

### 6a. `realCompilePackage` builds all indexes unconditionally

**File**: `ls/ls/core/compile/compile.go` (line ~280)

Every compile cycle builds all five indexes, even if no completion request is
pending. This is acceptable for a PoC but wasteful for production. A production
system should either:
- Build indexes lazily on first completion request after a compile
- Or build only the indexes needed for the current document's cursor position

### 6b. Index build is not incremental

The indexes are rebuilt from scratch every compile cycle. No diffing or
incremental update. This is fine for the PoC's scale but may not scale to
large projects.

### 6c. `InvocationCompletionIndex` walks the full AST

**File**: `ls/projects/invocation_completion_index.go` (line ~180)

`buildModuleInvocationCompletionIndex` walks the entire module AST with
`ast.Walk(collector, moduleCtx.bLangPkg)`. For large modules this is O(n) in
the number of AST nodes. A production system might want to limit this to
files that have open documents.

### 6d. `MemberCompletionIndex` also walks the full AST

**File**: `ls/projects/member_completion_index.go` (line ~120)

Same pattern — full AST walk per module. Could be scoped to open documents.

### 6e. `ExpectedTypeIndex` reads all slot records

**File**: `ls/projects/expected_type_index.go` (line ~90)

`buildModuleExpectedTypeIndex` iterates all resolver-captured slot records.
For large modules with many expression slots, this could be expensive.

### 6f. Panic recovery in index builders

**File**: `ls/projects/member_completion_index.go` (line ~130)

```go
defer func() {
    if r := recover(); r != nil {
        idx = &MemberCompletionIndex{slots: make(map[string][]MemberAccessSlot)}
    }
}()
```

The index builders use `defer/recover` to ensure a crash during index
construction doesn't crash the compile. This is pragmatic but masks bugs.

---

## 7. Production-ready patterns worth porting directly

### 7a. Protocol-free boundary with copied DTOs

All five indexes copy only strings, byte offsets, and integer kinds/enums.
No AST nodes, symbols, semtypes, or compiler contexts escape the `projects/`
package. This is the **single most important pattern** — it prevents the
query layer from depending on compiler internals and makes the indexes safe
for concurrent access.

### 7b. Generation-matched lease with pinning

The lease mechanism (`SnapshotStore.lease`) with:
- Exact generation matching (no stale reuse)
- Reference-counted pinning (deferred eviction while lease is held)
- Idempotent release (safe for double-release)
- Non-blocking acquisition (never waits for compilation)

This is a **directly reusable pattern** for any LSP feature that needs
compiler data without blocking.

### 7c. Fallback-first completion

The query layer always computes current-syntax and static keyword candidates
first, then enriches with semantic facts from a lease. If the lease fails
(no matching snapshot yet), the fallback is still a valid completion list.
This means completion is **always responsive**, even during compilation.

### 7d. Innermost-slot selection for expected types and invocations

Both `ExpectedTypeIndex.FactAt` and `InvocationCompletionIndex.SlotAt` use
the **innermost (smallest-range) slot** that contains the cursor. This
correctly handles nested calls like `f(g(x))` — the inner `g(x)` slot wins
over the outer `f(...)` slot.

### 7e. Precomputed assignability tiers

The invocation completion index precomputes `ArgRelevance.Direct` and
`ArgRelevance.Check` lists at compile time using semtype subtype checks.
The query layer only reads label strings — it never re-derives type
compatibility. This is a **performance-critical pattern** for production.

### 7f. Snippet forms with required-parameter placeholders

**File**: `ls/projects/invocation_completion_index.go` (line ~280)

`callableSnippet` builds snippet forms like `name(${1:p1}, ${2:p2})$0` with
placeholders only for required parameters. Defaultable, included-record, and
rest parameters are omitted from the insertion snippet. This is a
**production-quality UX pattern**.

### 7g. Import catalog with auto-import

**File**: `ls/ls/core/query/completion_module.go` (line ~200-250)

The auto-import pattern:
1. Detect `alias.<prefix>` where `alias` matches a catalog module's final segment
2. Offer the module name as a completion candidate
3. Attach an `AdditionalEdit` that inserts the missing import declaration
4. Handle alias collision with `firstFreeAlias` (suffix `2`, `3`, ...)

This is a **directly reusable pattern** for any language with explicit imports.

---

## 8. Data flow summary

```
User types → textDocument/completion
    ↓
Server: handleCompletion()
    ├─ Convert UTF-16 position → byte offset
    ├─ Get workspace snapshot (generation)
    └─ Call query.Service.Completion(uri, byteOffset)
        ↓
query.Service.Completion()
    ├─ Classify cursor context (import/body/module-part/alias-member)
    ├─ For function-body:
    │   ├─ Collect current-syntax scope (params + locals) ← ALWAYS
    │   ├─ Add fixed keyword/construct catalog ← ALWAYS
    │   └─ Try non-blocking lease:
    │       ├─ On success: add semantic facts, boost tiers, enrich snippets
    │       └─ On failure: fallback (syntax/static only) ← NEVER BLOCKS
    └─ Return CompletionResult (byte-offset prefix range + items)
        ↓
Server: toLSPCompletionList()
    └─ Convert byte offsets → UTF-16 positions
    └─ Build TextEdit with prefix range
    └─ Select snippet vs plaintext per client capability
```

---

## 9. Key files reference

| File | Purpose |
|------|---------|
| `ls/ls/core/compile/compile.go` | CompilationService, Lease(), realCompilePackage() builds all indexes |
| `ls/ls/core/compile/snapshot.go` | SnapshotStore, StableSnapshot, lease/pin/evict |
| `ls/ls/core/query/completion.go` | Query Service, Completion(), completeFunctionBody(), lease acquisition |
| `ls/ls/core/query/completion_body.go` | Statement-start/expression keyword catalogs, scope collection |
| `ls/ls/core/query/completion_module.go` | Module-part completion, import completion, alias-member, auto-import |
| `ls/ls/core/query/completion_invocation.go` | Named args, callable snippets, invocation tier boosting |
| `ls/ls/core/query/query.go` | Service struct, SetCompletionCompiler(), DocumentSymbols |
| `ls/ls/server/completion.go` | Server handler, UTF-16 conversion, lease adapter |
| `ls/projects/completion_index.go` | CompletionIndex, BuildCompletionIndex |
| `ls/projects/expected_type_index.go` | ExpectedTypeIndex, BuildExpectedTypeIndex |
| `ls/projects/member_completion_index.go` | MemberCompletionIndex, BuildMemberCompletionIndex |
| `ls/projects/invocation_completion_index.go` | InvocationCompletionIndex, BuildInvocationCompletionIndex |
| `ls/projects/import_catalog.go` | ImportCatalog, BuildImportCatalog |
| `ls/ls/core/compile/completion_lease_test.go` | Lease tests (generation match, stale reuse, deferred eviction) |
| `ls/ls/core/compile/completion_query_test.go` | Integration tests (fallback, expected-type ranking, invocation tiers) |
| `ls/ls/core/query/completion_test.go` | Query-layer unit tests (scope, keywords, prefix filter, dedup) |
| `ls/projects/completion_index_test.go` | Index builder tests (module-level facts, sibling files) |

## 10. PoC (ls-ref) completion patterns — verified against actual code

### 10a. PoC re-parses on every completion request

**File**: `ls-ref/lsp/completion.go:100-110` — `recoveringCompilationUnit()` calls `parser.GetSyntaxTree()` and `ast.NewRecoveringNodeBuilder()` on every request. No cached compilation units are reused for completion.

### 10b. PoC creates a full snapshot copy per request

**File**: `ls-ref/lsp/completion.go:1119-1156` — `snapshotWithRecoveredCU()` shallow-copies all modules, deep-copies the `CompilationUnits` map, resets the target module's `Stage` to `FrontendStageNone`, clears all frontend-derived state, then re-runs `runModuleFrontend` synchronously.

### 10c. PoC uses live AST walks + scope hierarchy for symbol visibility

**File**: `ls-ref/lsp/completion.go:1304-1343` — `visibleSymbolCompletionItemsWithFilter()` walks the scope hierarchy from inner to outer, extracting symbols from `model.SymbolSpace` attached to scopes. No precomputed index.

### 10d. PoC member access uses semtype subtype checks at request time

**File**: `ls-ref/lsp/completion.go:324-356` — `memberAccessCompletionItemsFromReceiver()` calls `completionReceiverType()` which reads `expr.GetDeterminedType()` or `cx.SymbolType(symbol)`, then does `semtypes.IsSubtype(tyCtx, receiverTy, semtypes.MAPPING)` or `semtypes.IsSubtype(tyCtx, receiverTy, semtypes.OBJECT)` at request time. No precomputed member index.

### 10e. PoC has no expected-type or invocation completion

Confirmed: no `ExpectedTypeIndex`, no `InvocationCompletionIndex`, no slot-based expected-type projection, no callable catalog, no argument relevance tiers, no named-argument completion in `ls-ref/lsp/completion.go`.

### 10f. PoC uses `defer/recover` as primary error handling

**File**: `ls-ref/lsp/completion.go:30-35` — `defer func() { if recovered := recover(); recovered != nil { logLS(...) } }()` wraps the entire completion handler.

## 11. Current worktree (ls/ls/) completion patterns — verified against actual code

### 11a. Non-blocking lease mechanism

**File**: `ls/ls/core/compile/compile.go:Lease()` — returns a generation-matched `CompletionLease` with copied index facts. Never waits, parses, compiles, or schedules compilation.

### 11b. Five precomputed indexes built during compile

**File**: `ls/ls/core/compile/compile.go:realCompilePackage()` — builds `completionIndex`, `expectedTypeIndex`, `importCatalog`, `memberCompletionIndex`, `invocationCompletionIndex` from the compiled package.

### 11c. Fallback-first completion

**File**: `ls/ls/core/query/completion.go:completeFunctionBody()` — collects current-syntax scope and keyword catalog first (lines ~250-260), then tries lease acquisition (lines ~270-300). If lease fails, fallback is still a valid completion list.

### 11d. Protocol-free boundary with copied DTOs

**File**: `ls/ls/core/query/completion.go:CompletionItem` — query-layer DTO with `Label`, `Kind`, `Detail`, `InsertText`, `Snippet`, `Rank`, `AdditionalEdits`. No AST nodes, symbols, or compiler contexts escape the `projects/` package.

### 11e. Generation-matched lease with pinning

**File**: `ls/ls/core/compile/snapshot.go:SnapshotStore.lease()` — exact generation matching, reference-counted pinning, idempotent release, non-blocking acquisition.

## 12. Key insight for ticket 36

The PoC proves that completion CAN work without precomputed indexes — it does live AST walks + scope hierarchy walks + semtype subtype checks at request time. But the current worktree has already moved to a lease-based architecture with five precomputed indexes. The question for ticket 36 is whether to:

1. **Drop the indexes and go back to live reads** (PoC pattern): This would eliminate the index build cost but reintroduce request-time compiler reads (scope walks, type queries). The PoC shows this is feasible but requires the module to be compiled to at least `FrontendStageLocalTypeResolved` at request time — which the PoC achieves by re-compiling synchronously. The current worktree's background pipeline already compiles to this stage, so live reads would work against the already-compiled state.

2. **Keep the indexes but make them cheaper** (current worktree pattern): The indexes are already built during the compile cycle (not separately), and the lease mechanism makes them non-blocking. The cost is the index build itself, which is O(n) in AST nodes per module. If this cost is acceptable, the indexes provide faster request-time reads (O(1) slot lookup vs O(n) scope walk).

**Recommendation**: The PoC's live-read approach is a valid alternative to indexes, but the current worktree's lease-based index architecture is more production-ready. If the index build cost is a concern, the PoC proves that live reads are feasible — but the current worktree would need to ensure the background pipeline compiles to `FrontendStageLocalTypeResolved` for all modules (not just the changed one).
