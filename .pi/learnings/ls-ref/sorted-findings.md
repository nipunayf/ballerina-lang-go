# Ticket 36: Derive completion from compiled package — ls-ref findings

## Summary

The Go LS PoC already implements the approach ticket 36 proposes: **all five
completion semantic indexes are built during the compile cycle from the compiled
package's syntax trees and resolved symbols, then published as part of the
stable snapshot**. There is no separate precomputed index that lives outside
the compile cycle. The query layer acquires these indexes through a
non-blocking, generation-matched lease.

---

## 1. The single production point

**File**: `ls/ls/core/compile/compile.go`, function `realCompilePackage()` (line ~280)

```go
func realCompilePackage(pkg *projects.Package) cycleResult {
    comp := pkg.Compilation()
    // ... diagnostics extraction ...
    return cycleResult{
        completionIndex:            projects.BuildCompletionIndex(pkg),
        expectedTypeIndex:          projects.BuildExpectedTypeIndex(comp),
        importCatalog:              projects.BuildImportCatalog(pkg),
        memberCompletionIndex:      projects.BuildMemberCompletionIndex(comp),
        invocationCompletionIndex: projects.BuildInvocationCompletionIndex(comp),
    }
}
```

This is called once per compile cycle. The indexes are built from the compiled
package's data structures while they're still in memory, then copied into
protocol-free DTOs and stored in the `StableSnapshot`.

**Pattern to port directly**: This is the exact pattern ticket 36 needs. The
index builders are called after compilation completes, using the compiled
package's AST and symbol spaces. No separate precomputed index is maintained.

---

## 2. The non-blocking lease — the critical pattern

**File**: `ls/ls/core/compile/compile.go` (line ~200-220)

The `CompilationService.Lease()` method provides generation-matched, non-blocking
access to the published indexes:

```go
func (s *CompilationService) Lease(root string, generation uint64) (CompletionLease, bool) {
    snap, release, ok := s.store.lease(root, generation)
    if !ok {
        return CompletionLease{}, false
    }
    return CompletionLease{index: snap.completionIndex, ..., release: release}, true
}
```

**Key properties**:
- **Never blocks**: returns `ok=false` immediately if no matching snapshot
- **Generation-matched**: only returns a snapshot for the exact generation
- **Pinned**: the snapshot is reference-counted; eviction is deferred while held
- **Idempotent release**: safe for double-release

**File**: `ls/ls/core/compile/snapshot.go` (line ~200-240) — the `SnapshotStore.lease()` implementation

**File**: `ls/ls/core/compile/completion_lease_test.go` — tests verifying:
- `TestCompletionLeaseMatchesGeneration` — matching gen succeeds, wrong gen fails
- `TestCompletionLeaseNoStaleReuseAfterEdit` — after Supersede, old gen not reused
- `TestCompletionLeaseDeferredEviction` — held lease stays valid after eviction

---

## 3. How the query layer consumes the lease

**File**: `ls/ls/core/query/completion.go`, function `completeFunctionBody()` (line ~200-250)

The query layer:
1. **Always** computes current-syntax scope (parameters + preceding locals)
2. **Always** adds fixed keyword/construct catalog
3. **Then** tries to acquire a lease for semantic enrichment:
   - Module-level declaration facts from `CompletionIndex`
   - Expected-type boosting from `ExpectedTypeIndex`
   - Callable snippets from `InvocationCompletionIndex.Callables()`
   - Invocation tier boosting from `InvocationCompletionIndex.SlotAt()`
   - Named arguments from `InvocationCompletionIndex`
   - Member access candidates from `MemberCompletionIndex`
   - Import catalog from `ImportCatalog`
4. If no lease, falls back to syntax/static only — **never blocks**

**Pattern to port directly**: The fallback-first approach ensures completion is
always responsive. Semantic facts are an optional enrichment, not a requirement.

---

## 4. The five index builders — what they walk

### 4a. `BuildCompletionIndex(pkg)` — module-level declarations
- **File**: `ls/projects/completion_index.go` (line ~80)
- **Walks**: `ModulePart.Members()` of every document's syntax tree
- **Extracts**: FunctionDefinition, TypeDefinition, ConstantDeclaration,
  ModuleVariableDeclaration, EnumDeclaration, ClassDefinition
- **Groups by**: module (sibling files' facts are visible from each file)
- **Copies**: only `CompletionFact{Label, Kind, Detail}` — no AST nodes

### 4b. `BuildExpectedTypeIndex(comp)` — contextual expected types
- **File**: `ls/projects/expected_type_index.go` (line ~80)
- **Reads**: `compilerCtx.ExpectedSlotRecords()` — resolver-captured slot records
- **Computes**: display-safe type label + precomputed assignable candidates
- **Keys by**: byte span; innermost span wins at query time
- **Copies**: only `ExpectedTypeFact{Kind, Known, TypeLabel, StartOffset,
  EndOffset, ArgIndex, Compatible []string}`

### 4c. `BuildMemberCompletionIndex(comp)` — member access projections
- **File**: `ls/projects/member_completion_index.go` (line ~100)
- **Walks**: resolved AST with `ast.Walk` for field-access and remote-call nodes
- **Resolves**: receiver type → accessible members (record fields, object
  fields/methods, remote methods)
- **Keys by**: `(fileKey, accessKind, dotOffset)`
- **Copies**: only `MemberAccessSlot{Kind, DotOffset, Candidates []MemberCandidate}`

### 4d. `BuildInvocationCompletionIndex(comp)` — callable catalog + relevance
- **File**: `ls/projects/invocation_completion_index.go` (line ~150)
- **Walks**: resolved AST for call expressions
- **Resolves**: function symbol → parameter facts + precomputed relevance tiers
- **Precomputes**: `ArgRelevance.Direct` (subtype of param type) and
  `ArgRelevance.Check` (subtype after stripping error)
- **Copies**: only `InvocationSlot`, `CallableEntry`, `ParamSlot`,
  `NamedArgEntry`, `ArgRelevance` — no compiler objects

### 4e. `BuildImportCatalog(pkg)` — importable modules + alias exports
- **File**: `ls/projects/import_catalog.go` (line ~130)
- **Reads**: embedded stdlib FS (cached once), project modules, resolved
  public symbol spaces
- **Copies**: only `CatalogModule{Org, ModuleName}` and
  `AliasExport{Alias, Org, ModuleName, Facts []CompletionFact}`

---

## 5. Prototype shortcuts to avoid

| Shortcut | File:line | Why it's a problem | Production fix |
|----------|-----------|-------------------|----------------|
| All 5 indexes built every compile | `compile.go:280` | Wasted work when no completion is pending | Build lazily on first completion request after compile |
| Full AST walk per module | `invocation_completion_index.go:180`, `member_completion_index.go:120` | O(n) in AST size for every compile | Scope to files with open documents |
| All slot records iterated | `expected_type_index.go:90` | O(n) in expression count | Limit to open documents |
| Panic recovery in index builders | `member_completion_index.go:130` | Masks bugs | Use error returns instead |
| No incremental update | All builders | Rebuilds from scratch every cycle | Diff-based incremental update (future work) |

---

## 6. Production-ready patterns to port directly

| Pattern | File:line | Why it's production-quality |
|---------|-----------|----------------------------|
| Protocol-free DTO boundary | All `projects/*_index.go` | No compiler objects leak; safe for concurrent access |
| Generation-matched lease | `compile.go:200`, `snapshot.go:200` | No stale semantic reuse; non-blocking; pinned eviction |
| Fallback-first completion | `completion.go:200` | Always responsive; semantic facts are optional enrichment |
| Innermost-slot selection | `expected_type_index.go:50`, `invocation_completion_index.go:80` | Correct for nested calls/expressions |
| Precomputed assignability tiers | `invocation_completion_index.go:250` | Query layer never re-derives type compatibility |
| Snippet forms with required-only placeholders | `invocation_completion_index.go:280` | Good UX; defaultable/rest params omitted |
| Auto-import with alias collision handling | `completion_module.go:200` | Handles `io` vs `io2` gracefully |
| Server-side lease adapter | `server/completion.go:30` | Keeps query layer free of compile imports |

---

## 7. Contradictions and gaps

### 7a. No gap: the approach matches ticket 36 exactly

The PoC already does what ticket 36 proposes: indexes are derived from the
compiled package during the compile cycle, not precomputed separately. The
lease mechanism provides the non-blocking access pattern.

### 7b. Gap: no lazy index building

The PoC builds all five indexes unconditionally on every compile. A production
system should build only the indexes needed for open documents' cursor
positions, or build them lazily on first completion request.

### 7c. Gap: no incremental index update

Every compile rebuilds all indexes from scratch. For large projects with
frequent small edits, this is wasteful. An incremental approach (tracking
which declarations changed and updating only those facts) would be more
efficient.

### 7d. Gap: index build is synchronous in the compile cycle

The indexes are built in `realCompilePackage`, which runs synchronously in the
compile goroutine. This extends the compile cycle. A production system could
build indexes asynchronously after the compile completes, publishing them
separately.

---

## 8. Key files for ticket 36 implementation

| File | What to read |
|------|-------------|
| `ls/ls/core/compile/compile.go` | `realCompilePackage()`, `Lease()`, `CompletionLease` struct |
| `ls/ls/core/compile/snapshot.go` | `SnapshotStore.lease()`, `StableSnapshot`, pinning/eviction |
| `ls/ls/core/query/completion.go` | `completeFunctionBody()` lease acquisition pattern |
| `ls/ls/core/query/query.go` | `CompletionLeaser` interface, `SetCompletionCompiler()` |
| `ls/ls/server/completion.go` | `completionLeaseAdapter` pattern |
| `ls/projects/completion_index.go` | `BuildCompletionIndex()` — simplest index builder to replicate |
| `ls/projects/expected_type_index.go` | `BuildExpectedTypeIndex()` — slot-based projection pattern |
| `ls/projects/invocation_completion_index.go` | `BuildInvocationCompletionIndex()` — most complex, includes relevance tiers |
| `ls/projects/member_completion_index.go` | `BuildMemberCompletionIndex()` — AST-walk-based projection |
| `ls/projects/import_catalog.go` | `BuildImportCatalog()` — module listing + alias exports |
| `ls/ls/core/compile/completion_lease_test.go` | Lease contract tests |
| `ls/ls/core/compile/completion_query_test.go` | Integration tests for fallback + ranking |
