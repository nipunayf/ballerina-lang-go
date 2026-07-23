# Completion projection timing: pre-desugar vs. post-compilation derivation

Ticket 36: what completion data can be derived on-demand from a completed
`*projects.Package`/`*projects.PackageCompilation` without compiling, versus
what requires resolver-time pre-desugar state.

## The three projections that REQUIRE pre-desugar state

All three are built inside `analyzeAndDesugar` (Phase 2), AFTER local-node
resolution but BEFORE desugar. They cannot be derived post-compilation because
they depend on private `moduleContext` fields that are not exposed through the
public `Package`/`Module`/`Document` API.

### 1. ExpectedTypeIndex (`ls/projects/module_context.go:348`)

- Built by `buildModuleExpectedTypeIndex(moduleCtx)` at `ls/projects/expected_type_index.go:120-170`
- Reads `moduleCtx.compilerCtx.ExpectedSlotRecords()` — captured during
  `resolveActionOrExpression` in the type resolver (`ls/semantics/type_resolver.go:3201-3230`)
- Reads `moduleCtx.bLangPkg.Scope` for module-level value candidates
- Uses `semtypes.ContextFrom(moduleCtx.compilerCtx.GetTypeEnv())` for subtype checks
- **Cannot be derived post-compilation** because:
  - `moduleCtx.compilerCtx` is private (no public accessor on `PackageCompilation`)
  - `moduleCtx.bLangPkg` is private (no public `AST()` method on `Package`)
  - The desugar pass may rewrite/empty the AST's constant/global arrays
  - `PackageCompilation.SemanticModel()` returns `any` (TODO stub, returns nil)

### 2. MemberCompletionIndex (`ls/projects/module_context.go:349`)

- Built by `buildModuleMemberCompletionIndex(moduleCtx)` at `ls/projects/member_completion_index.go:130-170`
- Walks `moduleCtx.bLangPkg` with `ast.Walk` to find `BLangFieldBaseAccess`
  and `BLangRemoteMethodCallAction` nodes
- Reads receiver's `GetDeterminedType()` (set during local-node resolution)
- Uses `semtypes.ContextFrom(moduleCtx.compilerCtx.GetTypeEnv())` for member enumeration
- **Cannot be derived post-compilation** because:
  - `moduleCtx.bLangPkg` is private
  - `moduleCtx.compilerCtx` is private
  - The AST is modified by desugar (field accesses may be rewritten)

### 3. InvocationCompletionIndex (`ls/projects/module_context.go:350`)

- Built by `buildModuleInvocationCompletionIndex(moduleCtx)` at `ls/projects/invocation_completion_index.go:200-260`
- Walks `moduleCtx.bLangPkg` with `ast.Walk` to find `BLangInvocation` and
  `BLangRemoteMethodCallAction` nodes
- Reads `inv.RawSymbol` (resolved during local-node resolution)
- Resolves function symbols via `moduleCtx.compilerCtx.GetSymbol(ref)`
- Uses `semtypes.ContextFrom(moduleCtx.compilerCtx.GetTypeEnv())` for relevance tiers
- **Cannot be derived post-compilation** because:
  - `moduleCtx.bLangPkg` is private
  - `moduleCtx.compilerCtx` is private
  - The AST is modified by desugar

## What CAN be derived post-compilation (from the public surface)

### CompletionIndex (`ls/projects/completion_index.go:80-110`)

- Built by `BuildCompletionIndex(pkg)` — takes `*Package`, not `*PackageCompilation`
- Walks `Document.SyntaxTree()` (public red-node tree) for top-level declarations
- Reads text from `st.TextDocument().String()` (public)
- **CAN be derived post-compilation** — only uses the public
  `Package` → `Module` → `Document` → `SyntaxTree()` chain
- Currently built in `realCompilePackage` at `ls/ls/core/compile/compile.go:290`
  alongside the pre-desugar projections for consistency

### ImportCatalog (`ls/projects/import_catalog.go:56-70`)

- Built by `BuildImportCatalog(pkg)` — takes `*Package`
- Reads embedded stdlib FS (static, cached once via `sync.Once`)
- Reads `Package.ModuleIDs()` and `Module.DocumentIDs()` (public)
- Reads `Document.SyntaxTree()` for import declarations (public)
- Reads `Environment.publicSymbols` (PRIVATE) via `compiledEnvironment(pkg)` →
  `env.publicSymbols` — this is the blocker for post-compilation derivation
- **PARTIALLY derivable post-compilation**: stdlib and project module lists are
  derivable, but alias exports require `Environment.publicSymbols` which is
  private and only accessible during the compile worker (the env that performed
  the compilation, not the package's own project reference)
- Currently built in `realCompilePackage` where the env is available

## The CompletionLease boundary

### Query-layer interface (`ls/ls/core/query/completion.go:20-40`)

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

### Compilation-service implementation (`ls/ls/core/compile/compile.go:340-370`)

- `compile.CompletionLease` wraps the snapshot's five projections
- `compile.CompilationService.Lease(root, generation)` delegates to
  `SnapshotStore.lease(root, generation)` at `ls/ls/core/compile/snapshot.go:200-240`
- Non-blocking, generation-matched: returns `ok=false` if no stable snapshot
  for exactly `generation` exists
- Increments per-root lease count to pin the snapshot against deferred eviction
- Release func is idempotent (wrapped in `sync.Once`)

### SnapshotStore lease mechanics (`ls/ls/core/compile/snapshot.go:200-240`)

- `lease(root, generation)` checks `pendingEvict[root]` first — evicted roots
  are removed from future acquisition
- Matches `snap.key.Generation == generation` exactly
- Increments `leaseCount[root]` to pin the snapshot
- On `Release()`: decrements count; if count reaches 0 and `pendingEvict[root]`
  is set, final eviction runs
- This keeps the LRU order bounded while honoring in-flight leases

### Server adapter (`ls/ls/server/completion.go:20-35`)

- `completionLeaseAdapter` adapts `compile.CompilationService.Lease()` to
  `query.CompletionLeaser`, keeping `ls/core/query` free of `compile` imports
- `completionLease` wraps `compile.CompletionLease` to satisfy the interface

## What the query layer CANNOT derive on-demand

The query layer (`ls/core/query/`) can derive from the red-node tree alone:
- Cursor context classification (import/function-body/module-part/alias-member)
- Preceding locals and parameters
- Keyword/construct catalogs (hardcoded)

The query layer reads from the five projections (via lease):
- Module-level facts (from `CompletionIndex`)
- Expected-type facts (from `ExpectedTypeIndex`)
- Member-access candidates (from `MemberCompletionIndex`)
- Invocation candidates (from `InvocationCompletionIndex`)
- Import catalog (from `ImportCatalog`)

The query layer CANNOT derive:
- **Member types of a SemType** — requires `semtypes.Context` (thread-local)
  and the type value; no "what members does this type expose?" query
- **Subtype checks** — requires `semtypes.Context` and the compiler's semtype
  machinery
- **Symbol resolution** — requires `CompilerContext.GetSymbol(ref)` which is
  private
- **Scope chain enumeration** — requires `moduleContext.compilerCtx` which is
  private
- **Documentation strings** — `MarkdownDocumentationNode` lives on AST nodes;
  no public API
- **Type display strings** — `semtypes.ToString()` exists but requires
  `semtypes.Context`

## Design conclusion

The three pre-desugar projections (expected-type, member-access, invocation)
are NOT optimizations — they are the only way to capture resolver-derived
state that would be lost after desugar. They require:
1. The private `moduleContext.bLangPkg` (BLang AST, not red-node tree)
2. The private `moduleContext.compilerCtx` (symbol resolution + semtype context)
3. Pre-desugar state (desugar rewrites the AST)

The `CompletionIndex` and `ImportCatalog` could theoretically be derived
post-compilation from the public surface, but they're already built in the
same `realCompilePackage` function for consistency and because `ImportCatalog`
needs the private `Environment.publicSymbols`.

The `CompletionLease` boundary is correct: it provides all five projections
through a non-blocking, generation-matched lease. The query layer never
touches compiler objects. The projections are protocol-free (no LSP types,
no compiler objects, only copied strings and integers).

## Key files and line numbers

| File | Line | Symbol |
|------|------|--------|
| `ls/projects/module_context.go` | 348-350 | Projection build calls in `analyzeAndDesugar` |
| `ls/projects/expected_type_index.go` | 120-170 | `buildModuleExpectedTypeIndex` |
| `ls/projects/member_completion_index.go` | 130-170 | `buildModuleMemberCompletionIndex` |
| `ls/projects/invocation_completion_index.go` | 200-260 | `buildModuleInvocationCompletionIndex` |
| `ls/projects/completion_index.go` | 80-110 | `BuildCompletionIndex` (post-compile derivable) |
| `ls/projects/import_catalog.go` | 56-70 | `BuildImportCatalog` (partially derivable) |
| `ls/ls/core/query/completion.go` | 20-40 | `CompletionLease`/`CompletionLeaser` interfaces |
| `ls/ls/core/compile/compile.go` | 280-300 | `realCompilePackage` builds all projections |
| `ls/ls/core/compile/compile.go` | 340-370 | `CompletionLease` implementation |
| `ls/ls/core/compile/snapshot.go` | 200-240 | `SnapshotStore.lease` mechanics |
| `ls/ls/server/completion.go` | 20-35 | `completionLeaseAdapter` |
| `ls/projects/package_compilation.go` | 223-227 | `SemanticModel()` TODO stub |
| `ls/projects/package_compilation.go` | 243-247 | `CompletionManager()` TODO stub |
