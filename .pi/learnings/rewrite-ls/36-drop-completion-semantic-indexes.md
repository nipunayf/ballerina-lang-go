# Ticket 36: Drop completion's precomputed semantic indexes; derive from the compiled package directly

## Research question

Does the Go rewrite derive module/import/member/invocation/expected-type completion candidates from a **live semantic model** (the compiled package's bound AST, scopes, and types at request time) or from **precomputed indexes** (copied projections built during compilation and read at request time)?

**Answer: The Go rewrite uses precomputed projections (indexes) built during the compile cycle, not live reads from the compiled package at request time.** The indexes are protocol-free, immutable, generation-scoped copies of compiler facts. The query layer reads only these copies — it never touches the compiled package, AST nodes, scopes, symbols, or semtypes at request time.

This is a deliberate architectural choice: the indexes are built once per compile cycle and served through non-blocking generation-matched leases. The query layer is **compile-free by design**.

---

## Architecture overview

```
Compile cycle (background):
  realCompilePackage() → builds 5 indexes → publishes as cycleResult

Request time (foreground):
  query.Service.Completion() → acquires lease → reads indexes → returns items
```

### The 5 indexes (all in `ls/projects/`)

| Index | Builder | When built | What it contains |
|-------|---------|------------|------------------|
| `CompletionIndex` | `BuildCompletionIndex(pkg)` | After full compilation | Module-level declaration facts (label/kind/detail) |
| `ExpectedTypeIndex` | `BuildExpectedTypeIndex(comp)` | After local-node resolution | Expected-type slot facts (kind/type-label/byte-span/compatible-labels) |
| `MemberCompletionIndex` | `BuildMemberCompletionIndex(comp)` | After local-node resolution | Member-access slot facts (kind/dot-offset/candidates) |
| `InvocationCompletionIndex` | `BuildInvocationCompletionIndex(comp)` | After local-node resolution | Invocation slot facts (params/named-args/relevance-tiers) + callable catalog |
| `ImportCatalog` | `BuildImportCatalog(pkg)` | After full compilation | Importable modules + per-file alias exports |

### Build site

All five indexes are built in `realCompilePackage()` at:
- **`ls/ls/core/compile/compile.go:651-655`** — `completionIndex`, `expectedTypeIndex`, `importCatalog`, `memberCompletionIndex`, `invocationCompletionIndex`

They are published as fields of `cycleResult` (`compile.go:91-101`) and made available through the lease mechanism.

---

## Index-by-index analysis

### 1. CompletionIndex — module-level declaration facts

**File**: `ls/projects/completion_index.go:86-131` — `BuildCompletionIndex(pkg)`

**What it does**: Walks every document's syntax tree in every module of the compiled package. For each top-level member (function, type, constant, module variable, enum, class), extracts a `CompletionFact{Label, Kind, Detail}`. Groups facts by module key, maps file keys to module keys.

**What it does NOT do**: No type resolution, no scope walking, no symbol lookup. Pure syntax tree walk + source text extraction.

**Request-time read**: `query/completion.go:297-300` — `lease.Index().Facts(fileKey, offset)` returns the precomputed facts for the document's module.

**Edge cases**:
- `_` (underscore) module variables are excluded (`completion_index.go:130`)
- Missing tokens produce empty labels and are excluded (`completion_index.go:95-100`)
- `nil` package returns empty index (`completion_index.go:91-93`)

### 2. ExpectedTypeIndex — contextual expected-type slots

**File**: `ls/projects/expected_type_index.go:112-130` — `BuildExpectedTypeIndex(comp)`

**What it does**: Reads resolver-captured `ExpectedSlotRecords` from each module's `CompilerContext` (captured during type resolution at `ls/semantics/type_resolver.go:3201-3230`). For each record, computes a display-safe type label and precomputes module-level value candidates (variables + constants) whose resolved type is a subtype of the expected type.

**What it does NOT do**: No live type queries at request time. The subtype check (`compatibleCandidates` at `expected_type_index.go:205-215`) runs at projection-build time.

**Request-time read**: `query/completion.go:302-307` — `lease.ExpectedTypeIndex().FactAt(fileKey, offset)` returns the innermost (smallest-span) fact. `boostExpectedCompatible()` (`query/completion.go:170-185`) lowers the rank of precomputed-compatible candidates to `rankExpectedMatch`.

**Edge cases**:
- Panic recovery: `buildModuleExpectedTypeIndex` has a `recover()` that publishes no facts on failure (`expected_type_index.go:130-135`)
- Only root package modules contribute facts; dependency modules are excluded (`expected_type_index.go:118-122`)
- Zero expected type → `Known=false`, no compatible candidates (`expected_type_index.go:180` — `if !semtypes.IsZero(rec.Expected)`)

### 3. MemberCompletionIndex — member-access slots

**File**: `ls/projects/member_completion_index.go:122-140` — `BuildMemberCompletionIndex(comp)`

**What it does**: Walks the resolved AST with `fieldAccessCollector` (an `ast.Visitor`). For each `BLangFieldBaseAccess` and `BLangRemoteMethodCallAction`, resolves the receiver's determined type (`GetDeterminedType()`), enumerates accessible members via `semtypes.ToMappingAtomicType()` / `semtypes.ToObjectAtomicType()`, and copies candidate facts (label/kind/detail/insertText/snippet/rank).

**What it does NOT do**: No live type queries at request time. The receiver type resolution and member enumeration run at projection-build time.

**Request-time read**: `query/completion.go:463-495` — `completeMemberAccess()` acquires a lease, calls `idx.SlotAt(fileKey, kind, dotOffset)` to find the exact matching slot, then converts candidates to `CompletionItem`s.

**Edge cases**:
- Panic recovery: `buildModuleMemberCompletionIndex` has a `recover()` that publishes no slots on failure (`member_completion_index.go:150-155`)
- Unknown receiver type → no slot (`member_completion_index.go:195-197`)
- No usable source position → no slot (`member_completion_index.go:200-205`)
- Generated member names (`$`-prefixed) are excluded (`member_completion_index.go:447-449` — `isGeneratedMemberName()` checks `name[0] == '$'`)
- Remote methods are `$remote$`-prefixed and only offered via `->` operator (`member_completion_index.go:230-260`)
- Fields sort before methods (`rankMemberField=0`, `rankMemberMethod=1`)

### 4. InvocationCompletionIndex — call slots + callable catalog

**File**: `ls/projects/invocation_completion_index.go:202-230` — `BuildInvocationCompletionIndex(comp)`

**What it does**: Walks the resolved AST with `invocationCollector`. For each `BLangInvocation` and `BLangRemoteMethodCallAction`, resolves the callable's `FunctionSymbol`, copies parameter facts (name/category/detail), named-argument entries, and precomputed relevance tiers. Also builds a module-level callable catalog (`moduleFunctionCallables`) with snippet insertion forms.

**What it does NOT do**: No live symbol resolution or type queries at request time.

**Request-time read**: Three uses in `query/completion.go:310-312`:
1. `enrichCallableSnippets()` — sets snippet form on function-kind items matching a callable catalog entry
2. `boostInvocationTiers()` — applies precomputed Direct/Check relevance tiers for the exact positional argument index
3. `namedArgsFromIndex()` — derives named-argument candidates from the slot's parameter facts

**Edge cases**:
- Deferred method symbols (unresolved) → no slot (`invocation_completion_index.go:280-290`)
- Panic recovery: `buildModuleInvocationCompletionIndex` has a `recover()` (`invocation_completion_index.go:245-250`)
- Only root package modules contribute; dependency modules excluded (`invocation_completion_index.go:215-220`)
- Rest parameters never become named arguments (`invocation_completion_index.go:455` — rest param added to `params` but not to `named`)
- Included-record parameters expand to one named-arg entry per field (`invocation_completion_index.go:360-370`)

### 5. ImportCatalog — importable modules + alias exports

**File**: `ls/projects/import_catalog.go:139-150` — `BuildImportCatalog(pkg)`

**What it does**: Lists embedded stdlib modules (from `stdlibs.FS`, cached once), enumerates current-project non-default modules, and copies each document's imported-alias public exports from the environment's `publicSymbols`.

**Request-time read**: Multiple uses in `query/completion_module.go`:
- `importModuleItems()` — catalog modules for import completion
- `importOrgItems()` — distinct org names
- `aliasMemberItems()` — imported alias's public exports
- `autoImportItems()` — unimported module candidates with additional edit

**Edge cases**:
- Embedded stdlib list is built once and cached (`import_catalog.go:100-120`)
- Unresolved imports contribute no exports (safe fallback) (`import_catalog.go:195-200`)
- Duplicate aliases are merged (`import_catalog.go:180-185`)
- Default module is excluded from project modules (`import_catalog.go:165-170`)

---

## What is NOT precomputed (live reads at request time)

The query layer does live reads from the **current syntax tree** (not the compiled package) for:

1. **Cursor classification**: `classifyContext()` (`completion_module.go:98-145`) walks the module part's imports and members using `TextRange()` byte offsets. Pure syntax tree walk.

2. **Identifier prefix**: `identifierPrefixStart()` (`completion.go:723-740`) scans left from cursor over identifier characters. Pure text scan.

3. **Parameters and preceding locals**: `collectScope()` (`completion_body.go:388-407`) reads the function definition's parameter list and walks the statement sequence for local variable declarations. Pure syntax tree walk.

4. **Body position classification**: `classifyBodyPosition()` (`completion_body.go:140-175`) scans left from cursor for the last non-whitespace character. Pure text scan.

5. **Loop/else gating**: `loopEncloses()` (`completion_body.go:180-190`) and `canFollowWithElse()` (`completion_body.go:336-355`) walk the statement descent chain. Pure syntax tree walk.

6. **Alias-member detection**: `classifyAliasMember()` (`completion_module.go:236-270`) scans left from cursor for `identifier.` pattern. Pure text scan.

7. **Import classification**: `classifyImport()` (`completion_module.go:159-170`) walks import declarations. Pure syntax tree walk.

8. **Member-access detection**: `detectMemberAccess()` (`completion.go:411-450`) scans left from cursor for `.`/`?.`/`->` operator. Pure text scan.

---

## Contradictions and gaps

### Contradiction: The indexes ARE precomputed

Despite the ticket title "Drop completion's precomputed semantic indexes", the Go rewrite **builds precomputed indexes during compilation**. The difference from the Java LS is:

| Aspect | Java LS (old) | Go rewrite (new) |
|--------|---------------|-------------------|
| When built | Lazily at request time | During compile cycle |
| What's stored | Compiler objects (AST nodes, scopes, symbols) | Copied strings, byte offsets, integer kinds |
| How accessed | Direct read from compiled state | Generation-matched lease |
| Protocol coupling | Leaks compiler types | Protocol-free by design |

The Go rewrite's indexes are "precomputed" in the sense that they are built before request time. But they are **not** "precomputed semantic indexes" in the Java LS sense — they are **projections** that copy only display-safe facts and never expose compiler internals.

### Gap: Scope analysis is still live but limited

Parameters and preceding locals are derived from the current syntax tree at request time. This is the one area where the query layer does "derive from the compiled package directly" (via the syntax tree). However, this is limited to:
- Function parameters (from `FunctionDefinition.FunctionSignature().Parameters()`)
- Local variable declarations (from `VariableDeclarationNode` in the statement sequence)
- Foreach loop variables (from `ForEachStatementNode.TypedBindingPattern()`)

It does NOT include:
- Module-level variables from sibling files (those come from `CompletionIndex`)
- Imported symbols (those come from `ImportCatalog.AliasExports()`)
- Type aliases or type definitions in scope

### Gap: Expected-type index is a two-step process

The expected-type facts are:
1. **Captured** by the type resolver during compilation (`context.CompilerContext.RecordExpectedSlot()` at `ls/semantics/type_resolver.go:3201-3230`)
2. **Projected** into the index during the compile cycle (`buildModuleExpectedTypeIndex()` at `expected_type_index.go:120-170`)

This means the expected-type index is only as fresh as the last compile cycle. If the user is typing in an unsaved buffer, the expected-type facts may be stale until the next compile.

### Gap: Invocation completion requires LocalTypeResolved

The invocation and member-access indexes require `LocalTypeResolved` stage (call bindings resolved, receiver types determined). If the background pipeline stops at `TopLevelTypeResolved` (as it does for non-changed modules in build projects — see `ls/ls/core/compile/compile.go` diagnostics path), these indexes will be empty for those modules.

### Gap: No argument-position completion in ls-ref

The ls-ref PoC (Java LS rewrite) does NOT have a dedicated argument-position completion handler. The Go rewrite fills this gap with `InvocationCompletionIndex` + `namedArgsFromIndex()` + `boostInvocationTiers()`.

---

## Behavioral requirements for the Go rewrite

### Module-level completion
- Offer module-level functions, variables, constants, and types from the current module (including sibling files)
- Filtered by prefix (case-sensitive)
- Deduplicated by label (lowest rank kept)
- Sorted by rank then label
- Rank order: expected-match (-2) < check (-1) < parameter (0) < local (1) < keyword (2) < module-var (3) < constant (4) < function (5) < type (6)

### Member-access completion
- For `recv.<prefix>`: offer accessible fields and methods of the receiver type
- For `recv?.<prefix>`: offer accessible fields (optional field access)
- For `recv->method(...)`: offer remote methods only
- Fields sort before methods (rank 0 vs 1), then by label
- Generated member names (`$`-prefixed) are excluded
- Remote methods are `$remote$`-prefixed internally, stripped for display

### Invocation completion
- For `funcName(<cursor>`: offer named-argument candidates for the resolved callable's parameters
- Precomputed relevance tiers: Direct-compatible candidates → rankExpectedMatch (-2), Check-compatible → rankCheck (-1)
- Named arguments sort with parameters (rank 0)
- Rest parameters never become named arguments
- Included-record parameters expand to one named-arg entry per field

### Expected-type completion
- When a known expected type covers the cursor, boost precomputed-compatible candidates to rankExpectedMatch (-2)
- Innermost (smallest-span) fact wins
- Only module-level value candidates (variables + constants) are precomputed for compatibility
- Functions and types are NOT precomputed for compatibility (they remain at their default rank)

### Import completion
- Offer importable modules (embedded stdlib + current-project modules)
- Full-path filtering: `math.` → `math.vector`, never `math.math.vector`
- Active-segment-only replacement
- Already-imported modules are suppressed
- Auto-import: unimported module referenced as `alias.` gets an additional edit inserting the import

### Alias-member completion
- Offer imported alias's public exports for `alias.<prefix>` access
- Public exports are copied as CompletionFacts (label/kind/detail)
- Auto-import candidate when alias matches an unimported stdlib module

### Function-body completion
- Statement-start keywords: if/else/while/foreach/do/return/fail/panic/break/continue/lock/transaction/retry/match/fork/worker/var/final/type
- Expression keywords: check/checkpanic/trap/start/wait/flush/from/new/typeof/function/?/is/<>/[]/{}/object/error/true/false/null/""/0/base16/base64/re/string/xml
- Loop-only gating: break/continue only inside while/foreach (not across fork boundaries)
- Else follow-on: `else` only after if/else-if without else clause
- Scope: parameters (outermost) + preceding locals (innermost wins on shadowing)

### Module-part completion
- Declaration/type/keyword snippet matrix: function/type/const/class/enum/listener/final/public/private/import/main
- `import` only in import region (before first non-import member)
- `main` only when no main function exists
- Qualifier suppression: after `public`/`private`/`final`, don't offer qualifier-only candidates

---

## Key files

| File | Role |
|------|------|
| `ls/projects/completion_index.go` | Module-level declaration fact index |
| `ls/projects/expected_type_index.go` | Expected-type slot index |
| `ls/projects/member_completion_index.go` | Member-access slot index |
| `ls/projects/invocation_completion_index.go` | Invocation slot + callable catalog index |
| `ls/projects/import_catalog.go` | Importable modules + alias exports catalog |
| `ls/ls/core/query/completion.go` | Query layer: main completion dispatch, member access, scope analysis |
| `ls/ls/core/query/completion_module.go` | Query layer: module-part, import, alias-member completion |
| `ls/ls/core/query/completion_body.go` | Query layer: function-body completion (keywords, scope, gating) |
| `ls/ls/core/query/completion_invocation.go` | Query layer: invocation completion (named args, relevance tiers) |
| `ls/ls/core/query/query.go` | Query service definition, document symbols |
| `ls/ls/core/compile/compile.go` | Compile cycle: builds all indexes in `realCompilePackage()` (lines 651-655) |
| `ls/ls/server/completion.go` | Server adapter: converts query results to LSP protocol |
| `ls/context/context.go` | Compiler context: expected-slot capture (lines 65-110, 280-315) |
| `ls/semantics/type_resolver.go` | Type resolver: `recordExpectedSlot()` (lines 3201-3230) |
