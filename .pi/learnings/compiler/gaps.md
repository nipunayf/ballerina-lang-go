# Gaps — what the compiler does NOT expose

ALWAYS read before claiming an API exists. One line per gap; prune when a gap
is closed upstream.

## No public semantic access

- **No public API to access the AST** — `moduleContext.getBLangPackage()` is package-private; `Package` has no `AST()` method. From `ls/` you can only walk the red-node syntax tree (`Document.SyntaxTree()`), which has a different node shape and no symbols.
- **No public API to access the BIR** — `moduleContext.getBIRPackage()` is package-private; `Package` has no `BIR()` method.
- **No public API to access the CompilerContext** — `moduleContext.compilerCtx` is private; `PackageCompilation` does not expose it. Symbol queries are unreachable from `ls/`.
- **No `SemanticModel` type** — `PackageCompilation.SemanticModel()` returns `any` with TODO. `explore-codebase/projects/package_compilation.go:223-227`
- **`CompletionManager` is a TODO stub** — returns `any`. `explore-codebase/projects/package_compilation.go:243-247`
- **`CodeActionManager` is a TODO stub** — returns `any`. `explore-codebase/projects/package_compilation.go:235-239`
- **No public API to query imported symbols** — `moduleContext.importedSymbols` is private; no "what does this module import?" from `ls/`.
- **No public API to query the module scope or package scope** — `bLangPkg.Scope` / `BLangPackage.Scope` live on the private AST; no "what symbols are in scope at position X?" from `ls/`.
- **Dependency graphs expose descriptors only** — `PackageResolution.DependencyGraph()` / `ModuleDependencyGraph()` are public but return `*DependencyGraph[PackageDescriptor]` / `[ModuleDescriptor]`, not contexts.

## No position/type queries

- **No `NodeAtPosition(pos)` query** — no API maps byte offset → node in any package; must walk the red-node tree or AST manually.
- **No syntax-tree-to-AST-node mapping** — red nodes and BLang nodes are separate trees; no API maps between them.
- **Resolved (this gap is closed) — `ExpectedType` concept now exists**: `context.ExpectedSlotRecord`/`ExpectedSlotKind` (capture during resolution) and `projects.ExpectedTypeIndex`/`ExpectedTypeFact` (post-compilation projection) were added for ticket 20. See `completion-infrastructure.md`.
- **Expected type is still transient inside the resolver itself** — `expectedType` is a local parameter threaded through `resolveActionOrExpression`; `setExpectedType` (alias for `SetDeterminedType`) stores the *resolved* type, not the *expected* type, on the AST node. The `ExpectedSlotRecord` capture is what makes the transient value durable for LS consumption — see `completion-infrastructure.md`. `semantics/type_resolver.go:3187`, `semantics/semantic_analyzer.go:2113`.
- **No `TypeDescriptor`-to-position mapping** — type descriptors attach to AST nodes; no query for "what type descriptor covers position X".

## Compilation granularity & lifecycle

- **No per-document compilation** — always module/package level.
- **No way to get diagnostics for a single document** — `moduleContext.getDiagnostics()` returns the whole module's.
- **No incremental compilation** — `PackageCompilation.compile()` always recompiles everything; no change-tracking.
- **No cancellation checkpoints in compilation** — `context.Context` is not threaded through `resolveTypesAndSymbols()` or `analyzeAndDesugar()`; `sync.Once` means no abort once started.
- **No `Close()` / resource cleanup** on any project/package/compilation type.

## Concurrency hazards

- **`Environment.Duplicate()` shares `CompilerEnvironment`** — symbol spaces are not deep-copied; a snapshot that modifies symbols would corrupt the original. `explore-codebase/projects/env.go:84`
- **`DiagnosticEnv` is a per-CompilerEnvironment singleton** — shared across all compilations of the same source root; concurrent compilations overwrite each other's file registrations. `explore-codebase/context/env.go:37` (details in diagnostics.md)
- **`CompilerContext.stage` field is unprotected** — concurrent `StartStage`/`EndStage` would race. `explore-codebase/context/context.go:38-42`
- **`Environment.publicSymbols` map is unprotected** — concurrent access during parallel Phase 2 could race. `explore-codebase/projects/env.go:22-28`

## Completion-specific gaps

- **No cursor-context classification API** — no "is cursor in import/expression/type-context/statement" query. Must be built from red-node tree walking + token analysis. Addressed by `classifyContext`/`classifyCompletion` in `ls/core/query/`.
- **No visible-symbols-at-position query** — scope chain is private inside `moduleContext`; no public API to enumerate what's visible at a cursor position. Addressed by `CompletionIndex` (module-level facts) + red-node tree (parameters/locals).
- **No qualifier-resolution API** — `ModuleScope.GetPrefixedSymbol(prefix, name)` exists but is only accessible inside the compiler via `moduleContext.compilerCtx`. Addressed by `ImportCatalog` + `classifyAliasMember`.
- **No documentation-string query** — `MarkdownDocumentationNode` lives on AST nodes (`DocumentableNode`); no public API to retrieve docs for a symbol.
- **No type-display-string API** — `semtypes.SemType` has no `String()` or display-friendly formatter; `sem_type_printer_release.go` exists but is internal. `semtypes.ToString()` used at projection-build time in `ExpectedTypeIndex`.
- **No member-completion API** — `semtypes.MappingMemberTypeInnerVal`, `ObjectMemberType`, `ListMemberType` exist but require a `semtypes.Context` (thread-local) and the type value; no "what members does this type expose?" query. Deferred to ticket 19.
- **No import-completion API** — `moduleContext.importedSymbols` is private; no "what packages can I import?" query from `ls/`. Addressed by `ImportCatalog`.
- **No keyword-completion context** — no API to determine which keywords are valid at a given position. Addressed by hardcoded keyword lists in `ls/core/query/completion.go`.

## Module discovery gaps

- **No "list all available packages" API** — `PackageResolver.ResolveByName(ctx, org, name)` requires knowing org+name upfront; no way to enumerate all packages in a repository. `FileSystemRepository.GetPackageVersions` requires org+name. `workspaceRepository` only knows packages listed in workspace manifest.
- **No "list all stdlib packages" API** — `stdlibs.FS` is an `embed.FS` with no index; discovering what's available requires walking the embedded directory tree manually.
- **No "what packages does this module import?" query from `ls/`** — `moduleContext.importedSymbols` is private. The red-node syntax tree (`Document.SyntaxTree()`) exposes `ModulePart.Imports()` which gives import declarations, but resolving them to package descriptors requires the private `moduleContext.compilerCtx`.
- **No "what alias does this import use?" query** — import aliases are stored in `ast.BLangImportPackage.Alias` (private AST) and in the red-node `ImportDeclarationNode` (accessible but requires walking). No public API to query "what alias does module X use for package Y".
- **`Environment.publicSymbols` is private and unprotected** — the map of compiled package symbols is not exposed from `ls/` and has no mutex. `explore-codebase/projects/env.go:37`
- **No public API to access `PackageCache` contents by org** — `PackageCache.GetPackages(org, name)` is public but only returns already-cached packages; no way to enumerate all cached packages. `explore-codebase/projects/package_cache.go:80-90`
- **No public API to access `PackageResolver.Repositories()` from `ls/`** — `PackageResolver.Repositories()` is public but `Environment.PackageResolver()` is public, so this is accessible. However, `Repository` interface only has `GetPackage` and `GetPackageVersions` — no `ListPackages` or `Search` method.
