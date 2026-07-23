# Ticket 36 Evaluation: Production, Exact-Generation, No-Request-Compile Completion

**Date**: 2026-07-23  
**Evaluator**: compiler-explorer agent  
**Worktree**: `ls/` (current rebased worktree)  
**Prior conclusion**: ls-ref's live bound-AST approach could not be used by production LS.

## Summary

The current `ls/` worktree has replaced ls-ref's request-compile approach with a **dual-snapshot compilation engine** that builds **copied, protocol-free projections** during background compilation. The query layer (`ls/core/query`) reads these projections through a **non-blocking, generation-matched lease** and never touches compiler objects (AST nodes, CompilerContext, symbols, scopes, types, semtypes).

**Verdict**: The architecture now provides a production, exact-generation, no-request-compile completion path — but only for the specific features covered by the existing projections. It does NOT provide a retained bound AST, CompilerContext, or SemanticModel to the query layer.

---

## What exists now (sufficient)

### 1. Dual-snapshot compilation engine
**Path**: `ls/ls/core/compile/compile.go`  
**Key types**: `CompilationService`, `StableSnapshot`, `CompletionLease`, `SnapshotStore`

- `CompilationService` manages a bounded worker pool (max 4 workers), debounce (150ms), and per-root single-flight queue
- `SnapshotStore` retains stable snapshots by count (LRU, max 16), with deferred eviction for active leases
- `CompletionLease` is the non-blocking, generation-matched lease: `Lease(root, generation) → (CompletionLease, ok)`
- The lease never waits, parses, compiles, or schedules compilation
- `StableSnapshot` stores only copied projections — no compiler objects

### 2. CompletionIndex — module-level facts
**Path**: `ls/projects/completion_index.go`  
**Key types**: `CompletionIndex`, `CompletionFact`, `CompletionFactKind`

- Built by `BuildCompletionIndex(pkg)` after full package compilation
- Copies only: label, kind (function/module-var/constant/type), detail string
- Grouped by module; a module's declarations are visible from every file in that module
- No AST node, scope, symbol, or compiler context escapes

### 3. ExpectedTypeIndex — expected-type ranking
**Path**: `ls/projects/expected_type_index.go`  
**Key types**: `ExpectedTypeIndex`, `ExpectedTypeFact`, `ExpectedSlotKind`

- Built by `BuildExpectedTypeIndex(comp)` after local-node resolution
- Captures resolver-derived expected-type facts via `CompilerContext.ExpectedSlotRecords()` (enabled by `EnableExpectedSlotCapture()`)
- Precomputes compatible module-level value candidates (subtype check at projection-build time)
- No semtype, symbol, or compiler context escapes

### 4. ImportCatalog — import completion
**Path**: `ls/projects/import_catalog.go`  
**Key types**: `ImportCatalog`, `CatalogModule`, `AliasExport`

- Built by `BuildImportCatalog(pkg)` after full package compilation
- Lists embedded stdlib modules (from `stdlibs.FS`), project modules, and per-file alias exports
- Alias exports are copied as `CompletionFact` — no symbol objects escape

### 5. MemberCompletionIndex — member access
**Path**: `ls/projects/member_completion_index.go`  
**Key types**: `MemberCompletionIndex`, `MemberAccessSlot`, `MemberCandidate`

- Built by `BuildMemberCompletionIndex(comp)` after local-node resolution
- Walks resolved `BLangFieldBaseAccess` and `BLangRemoteMethodCallAction` nodes
- Copies: label, kind (field/method), detail, insert text, snippet, rank
- Keyed by access kind + dot byte offset for exact slot matching
- No AST node, symbol, semtype, or compiler context escapes

### 6. InvocationCompletionIndex — callable catalog + named args + relevance
**Path**: `ls/projects/invocation_completion_index.go`  
**Key types**: `InvocationCompletionIndex`, `InvocationSlot`, `CallableEntry`, `NamedArgEntry`, `ArgRelevance`

- Built by `BuildInvocationCompletionIndex(comp)` after local-node resolution
- Walks resolved `BLangInvocation` and `BLangRemoteMethodCallAction` nodes
- Copies: callable catalog (label, kind, detail, snippet, param counts), invocation slots (byte ranges, params, named args), relevance tiers (direct/check)
- Precomputes assignability tiers against module-level candidates at projection-build time
- No AST node, symbol, semtype, or compiler context escapes

### 7. Current-syntax analysis (red-node tree)
**Path**: `ls/ls/core/query/completion.go` (classifyCompletion, collectScope, etc.)

- Reads `Document.SyntaxTree()` — the parsed, recovered red-node tree
- Extracts parameters, preceding local declarations, loop/else gating
- No compiler objects involved — pure tree walking + token analysis

### 8. Non-blocking lease mechanism
**Path**: `ls/ls/core/compile/snapshot.go` (SnapshotStore.lease)

- Generation-matched: returns snapshot only when `snap.key.Generation == generation`
- Pins snapshot via lease count; deferred eviction while leases are held
- Never waits, compiles, or schedules compilation

---

## What does NOT exist (remaining gaps)

### No retained bound AST
- `moduleContext.bLangPkg` is private (`ls/projects/module_context.go:37`)
- `Package` has no `AST()` method
- The query layer cannot access `BLangCompilationUnit`, `BLangPackage`, or any AST node with symbols/types

### No CompilerContext access
- `moduleContext.compilerCtx` is private (`ls/projects/module_context.go:39`)
- `PackageCompilation` does not expose it
- Symbol queries (`GetSymbol`, `SymbolType`, `SymbolKind`, `SymbolName`) are unreachable from `ls/core/query`

### No SemanticModel
- `PackageCompilation.SemanticModel()` returns `any` with TODO (`ls/projects/package_compilation.go:223-227`)
- No public API for semantic queries

### No symbol/scope/type queries
- No `NodeAtPosition(pos)` query
- No visible-symbols-at-position query
- No qualifier-resolution API
- No documentation-string query
- No type-display-string API (except at projection-build time via `semtypes.ToString()`)

### No per-document compilation
- Always module/package level
- No way to compile a single file in isolation

### No cancellation checkpoints
- `context.Context` is not threaded through `resolveTypesAndSymbols()` or `analyzeAndDesugar()`
- No abort once started

---

## Comparison to ls-ref

| Aspect | ls-ref (request-compile) | Current ls/ (projection-based) |
|--------|------------------------|-------------------------------|
| Compilation on completion | Yes — fresh parse + frontend stages | No — reads precomputed projections |
| AST access | Full — recovered BLang AST | Red-node syntax tree only |
| CompilerContext access | Full — created fresh per request | None — private in moduleContext |
| Symbol/scope/type queries | Full — via cx.SymbolType/Name/Kind | None — only copied facts |
| semtypes access | Full — via semtypes.IsSubtype, ToMappingAtomicType, etc. | None — only at projection-build time |
| Member completion | Runtime type query | Precomputed projection |
| Expected-type ranking | Runtime type query | Precomputed projection |
| Invocation completion | Runtime callable resolution | Precomputed projection |
| Latency | Parse + compile per request | Zero (lease hit) or current-syntax only (lease miss) |
| Correctness | Always exact (fresh compile) | Exact for covered features (generation-matched) |

---

## What a feature needing more would require

Any feature that needs information beyond what the existing projections provide must either:

1. **Add a new projection** built during the background compile (e.g., a documentation-string projection, a type-display-string projection)
2. **Add a request-compile fallback** (like ls-ref's `snapshotWithRecoveredCU` + `runModuleFrontend`)
3. **Expose a new public API** on the compiler that the query layer can call through the lease mechanism

The current architecture's constraint — "no request-compile" — means the query layer cannot access the compiler's live data structures. It can only read precomputed, copied facts from the background compile's projections.

---

## Key files referenced

- `ls/ls/core/compile/compile.go` — CompilationService, CompletionLease, realCompilePackage
- `ls/ls/core/compile/snapshot.go` — SnapshotStore, StableSnapshot, lease mechanism
- `ls/ls/core/query/completion.go` — query-layer completion, classifyCompletion, collectScope
- `ls/ls/core/query/query.go` — Service, CompletionLeaser interface
- `ls/projects/completion_index.go` — CompletionIndex, BuildCompletionIndex
- `ls/projects/expected_type_index.go` — ExpectedTypeIndex, BuildExpectedTypeIndex
- `ls/projects/import_catalog.go` — ImportCatalog, BuildImportCatalog
- `ls/projects/member_completion_index.go` — MemberCompletionIndex, BuildMemberCompletionIndex
- `ls/projects/invocation_completion_index.go` — InvocationCompletionIndex, BuildInvocationCompletionIndex
- `ls/projects/module_context.go` — moduleContext (private fields), resolveTypesAndSymbols, analyzeAndDesugar
- `ls/context/context.go` — CompilerContext, ExpectedSlotRecord, EnableExpectedSlotCapture
- `ls/context/env.go` — CompilerEnvironment (public GetSymbol, FindSymbol)
