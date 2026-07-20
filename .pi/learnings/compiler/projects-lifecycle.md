# Projects lifecycle: data model, immutability, compilation pipeline

Keep entries summarized and pointer-dense — `path` + symbol, one line each.

## Package → Module → Document hierarchy (fully queryable post-compilation)

- `Package.ModuleIDs()` → `[]ModuleID` — `explore-codebase/projects/package.go:80`
- `Package.Module(id)` → `*Module` — `explore-codebase/projects/package.go:110`; uses `sync.Map.LoadOrStore` — `explore-codebase/projects/package.go:80-90`
- `Package.ModuleByName(name)` → `*Module` — `explore-codebase/projects/package.go:130`
- `Package.DefaultModule()` → `*Module` — `explore-codebase/projects/package.go:140`
- `Module.DocumentIDs()` → `[]DocumentID` — `explore-codebase/projects/module.go:80`
- `Module.Document(id)` → `*Document` — `explore-codebase/projects/module.go:100`
- `Document.SyntaxTree()` → `*tree.SyntaxTree` (red node tree) — `explore-codebase/projects/document.go:60`, `explore-codebase/projects/document_context.go:100-110`
- `Document.TextDocument()` → `text.TextDocument` — `explore-codebase/projects/document.go:65`, `explore-codebase/projects/document_context.go:110-120`
- All lazy-loaded and cached via `sync.Map` — thread-safe reads.

## Immutability / thread-safety

- **`Package`, `Module`, `Document` are immutable** — use `Modify().Apply()` to create modified copies. `explore-codebase/projects/package.go:28`, `explore-codebase/projects/module.go:28`, `explore-codebase/projects/document.go:28`.
- **`PackageCompilation` is not immutable** — `compileOnce sync.Once` means it compiles exactly once; `DiagnosticResult` set after compilation, never updated. `explore-codebase/projects/package_compilation.go:38-48`.
- **`PackageResolution` is not immutable** — built once in `newPackageResolution()`. `explore-codebase/projects/package_resolution.go:30-40`.
- **`CompilerContext` is not thread-safe** — `mu sync.Mutex` protects `diagnostics` slice, but `stage` field is unprotected (concurrent `StartStage`/`EndStage` would race). `explore-codebase/context/context.go:38-42`.
- **`CompilerEnvironment` is partially thread-safe** — `symbolSpacesMu sync.RWMutex` protects `symbolSpaces`; `underlyingSymbol sync.Map` is concurrent-safe. `explore-codebase/context/env.go:37-43`.
- **`PackageCache` is thread-safe** — `mu sync.RWMutex`. `explore-codebase/projects/package_cache.go:16-20`.
- **`DependencyGraph` is thread-safe** — `sortOnce sync.Once` for lazy topological sort. `explore-codebase/projects/dependency_graph.go:24-30`.
- **`Environment` is not thread-safe** — `publicSymbols map` is unprotected; concurrent access during parallel Phase 2 could race. `explore-codebase/projects/env.go:22-28`.

## Compilation pipeline

- **Phase 1** (sequential): parse → AST → import resolution → symbol resolution → top-level type resolution. `explore-codebase/projects/module_context.go:260-310` (`resolveTypesAndSymbols()`).
- **Phase 2** (parallel per module): local node resolution → semantic analysis → CFG → CFG analysis → desugar. `explore-codebase/projects/module_context.go:320-400` (`analyzeAndDesugar()`).
- **BIR generation** is separate — via `generateCodeInternal()`. `explore-codebase/projects/module_context.go:480-487`.
- **Error handling**: if any module has errors after Phase 1, Phase 2 is skipped entirely. `explore-codebase/projects/package_compilation.go:130-136`.
- **Phase 2 panics** are caught and re-panicked (not converted to diagnostics). `explore-codebase/projects/package_compilation.go:170-175`.
- Phase 2 goroutines run with `sync.WaitGroup` and no cancellation channel. `explore-codebase/projects/package_compilation.go:150-175`.

## Resolution vs compilation (both sync.Once, lazy)

- `Package.Resolution()` → `packageContext.getResolution()` uses `sync.Once`. `explore-codebase/projects/package_context.go:130-140`.
- `Package.Compilation()` → `packageContext.getPackageCompilation()` uses `sync.Once`. `explore-codebase/projects/package_context.go:123-128`.
- `getPackageCompilation()` calls `getResolution()` internally — resolution is always a prerequisite. `explore-codebase/projects/package_compilation.go:39-40`.
- After a modifier chain, the new `packageContext` has fresh `sync.Once` fields, so resolution/compilation re-triggers — but the shared `DiagnosticEnv` means new registrations overwrite the old ones (see diagnostics.md).

## Modifier chain

- `Document.Modify().WithContent().Apply()` → new `Document` → `ModuleModifier.updateDocument()` → `ModuleModifier.Apply()` → `PackageModifier.updateModule()` → `PackageModifier.Apply()` → `setCurrentPackage()`. `explore-codebase/projects/document.go:80-90`, `explore-codebase/projects/module.go:130-140`, `explore-codebase/projects/package.go:280-283`.
- `PackageModifier.Apply()` calls `newPackageContextFromMaps()` — fresh `packageContext` with zero-value `sync.Once` fields. `explore-codebase/projects/package.go:280-283`.
- The new `Package` shares the same `Environment` (and thus the same `CompilerEnvironment`/`DiagnosticEnv`).

## Snapshotting `CurrentPackage` safely

- `Package` is immutable — `p.duplicate(project)` creates a deep copy with independent module contexts. `explore-codebase/projects/package.go:280-283`.
- `Project.Duplicate()` creates a deep copy with fresh `Environment` (new caches, same repos). `explore-codebase/projects/build_project.go:155-163`.
- `Environment.Duplicate()` creates a new env with fresh `PackageCache` but the **same `*context.CompilerEnvironment` pointer** — symbol spaces are shared between original and duplicate. `explore-codebase/projects/env.go:82-91`, `explore-codebase/projects/env.go:84`.
- To snapshot: call `Project.Duplicate()` then `dup.CurrentPackage()` — own `PackageCache`, shared `CompilerEnvironment`/symbol spaces (a snapshot that modifies symbols would corrupt the original).

## Cancellation (absence of)

- **No `context.Context` in the compilation pipeline.** The `context` package is the compiler's own `CompilerContext`, not Go's. Zero cancellation checkpoints in `resolveTypesAndSymbols()` (`explore-codebase/projects/module_context.go:260`) or `analyzeAndDesugar()` (`explore-codebase/projects/module_context.go:320`).
- `context.Context` is used only in the resolution layer: `Repository.GetPackage(ctx)`, `PackageResolver.ResolvePackages(ctx)`, `workspaceRepository.GetPackage(ctx)` — these check `ctx.Done()` before I/O.
- `packageContext.getPackageCompilation()` / `getResolution()` use `sync.Once` — no cancellation possible once started. `explore-codebase/projects/package_context.go:123-140`.

## Versioning / generations (absence of)

- No generation counter, snapshot ID, or registration key on `PackageCompilation` (`explore-codebase/projects/package_compilation.go:22-32`), `Package` (`explore-codebase/projects/package.go:22-30`), `packageContext` (`explore-codebase/projects/package_context.go:22-35`), or `DiagnosticEnv` (`explore-codebase/tools/diagnostics/diagnostic_env.go:35-39`).
- Consequence: no way to distinguish stable vs in-progress snapshots at the DiagnosticEnv level.

## Resource / shutdown (absence of)

- **No `Close()` or `Shutdown()` on any type** — `Project`, `Package`, `PackageCompilation`, `Environment` have no cleanup methods.
- `Project.Save()` is a stub (no-op for all implementations). `explore-codebase/projects/build_project.go:155`.
- `BalaProject` is read-only. `explore-codebase/projects/bala_project.go:107`.
- `SingleFileProject.Save()` is a no-op. `explore-codebase/projects/single_file_project.go:103`.

## Module discovery — local/workspace/stdlib

### Workspace project discovery
- `WorkspaceProject.Projects()` → `[]*BuildProject` — all packages in workspace. `explore-codebase/projects/workspace_project.go:60-63`
- `WorkspaceProject.Manifest()` → `WorkspaceManifest` — workspace config. `explore-codebase/projects/workspace_project.go:65-68`
- `WorkspaceManifest.Packages()` → `[]string` — relative paths to packages. `explore-codebase/projects/workspace_manifest.go:32-35`
- `workspaceRepository.GetPackage(ctx, org, name, version)` — resolves workspace packages by org/name. `explore-codebase/projects/workspace_repository.go:55-85`
- `workspaceRepository.GetPackageVersions(ctx, org, name)` — lists versions in workspace. `explore-codebase/projects/workspace_repository.go:90-110`
- `WorkspaceProject.Resolution()` → `*WorkspaceResolution` — dependency graph between workspace packages. `explore-codebase/projects/workspace_project.go:70-75`
- `WorkspaceResolution.DependencyGraph()` → `*DependencyGraph[*BuildProject]`. `explore-codebase/projects/workspace_resolution.go:29-33`

### Stdlib / external package discovery
- `FileSystemRepository` — loads from bala directory structure: `basePath/{org}/{name}/{version}/{platform}/`. `explore-codebase/projects/filesystem_repository.go:52-56`
- `FileSystemRepository.GetPackage(ctx, org, name, version)` — loads specific version. `explore-codebase/projects/filesystem_repository.go:80-100`
- `FileSystemRepository.GetPackageVersions(ctx, org, name)` — lists available versions. `explore-codebase/projects/filesystem_repository.go:103-130`
- `stdlibs.FS` — embedded stdlib filesystem (`embed.FS`). `explore-codebase/lib/stdlibs/embed.go:17`
- `defaultRepositories(ballerinaEnvFs)` → `[]Repository` — bundled (stdlibs.FS) + central cache. `explore-codebase/projects/filesystem_repository.go:175-180`
- `Repository` interface: `GetPackage(ctx, org, name, version, opts)` + `GetPackageVersions(ctx, org, name, opts)`. `explore-codebase/projects/repository.go:31-40`
- `PackageResolver.ResolveByName(ctx, org, name, opts)` → `[]*Package` — resolves latest version. `explore-codebase/projects/package_resolver.go:100-130`
- `PackageResolver.ResolvePackages(ctx, requests, opts)` → `[]ResolutionResponse`. `explore-codebase/projects/package_resolver.go:50-60`

### Package metadata (fully public)
- `Package.Descriptor()` → `PackageDescriptor{org, name, version}`. `explore-codebase/projects/package.go:100-103`
- `Package.Manifest()` → `PackageManifest` — full Ballerina.toml content. `explore-codebase/projects/package.go:105-108`
- `PackageManifest.Dependencies()` → `[]Dependency` — declared deps. `explore-codebase/projects/package_manifest.go:100-103`
- `PackageManifest.ExportedModules()` → `[]string`. `explore-codebase/projects/package_manifest.go:130-133`
- `PackageManifest.Org()`, `Name()`, `Version()`, `Authors()`, `Keywords()`, `License()`, `Description()`, `Repository()`, `BallerinaVersion()`, `Visibility()`, `Icon()`, `Readme()`. `explore-codebase/projects/package_manifest.go:85-160`
- `Dependency{org, name, version, repository}` — `Org()`, `Name()`, `Version()`, `Repository()`. `explore-codebase/projects/package_manifest.go:48-80`
- `PackageResolution.ResolvedDependencies()` → `map[string]*PackageDescriptor` — all resolved deps (org/name → descriptor). `explore-codebase/projects/package_resolution.go:175-178`
- `PackageResolution.DependencyGraph()` → `*DependencyGraph[PackageDescriptor]`. `explore-codebase/projects/package_resolution.go:180-183`
- `PackageResolution.ModuleDependencyGraph()` → `*DependencyGraph[ModuleDescriptor]`. `explore-codebase/projects/package_resolution.go:185-188`

### Module metadata (fully public)
- `Module.Descriptor()` → `ModuleDescriptor{packageDescriptor, name}`. `explore-codebase/projects/module.go:60-63`
- `Module.ModuleID()` → `ModuleID{id, moduleName, packageID}`. `explore-codebase/projects/module.go:55-58`
- `Module.ModuleName()` → `ModuleName`. `explore-codebase/projects/module.go:65-68`
- `Module.IsDefaultModule()` → `bool`. `explore-codebase/projects/module.go:120-123`
- `Module.DocumentIDs()` → `[]DocumentID`. `explore-codebase/projects/module.go:75-78`
- `Module.TestDocumentIDs()` → `[]DocumentID`. `explore-codebase/projects/module.go:80-83`
- `Module.PackageInstance()` → `*Package` — navigation up the hierarchy. `explore-codebase/projects/module.go:70-73`

### Imports/aliases (extracted from syntax tree, resolved during compilation)
- `documentContext.moduleLoadRequests()` → `[]*moduleLoadRequest` — extracts imports from red-node syntax tree. `explore-codebase/projects/document_context.go:171-190`
- `moduleLoadRequest{orgName, moduleName}` — what a document imports. `explore-codebase/projects/module_load_request.go:41-48`
- `moduleContext.importedSymbols` — `map[string]model.ExportedSymbolSpace` keyed by alias — **PRIVATE**. `explore-codebase/projects/module_context.go:60`
- `semantics.ResolveCompilationUnitImports(ctx, cus, implicitImports, publicSymbols, defaultOrg)` → `[]CompilationUnitImports` — resolves imports to symbol spaces. `explore-codebase/semantics/symbol_resolver.go:1011-1021`
- `CompilationUnitImports{CompilationUnit, Imports map[string]model.ExportedSymbolSpace}` — per-CU imports with aliases. `explore-codebase/semantics/symbol_resolver.go:1013-1021`
- `Environment.publicSymbols` — `map[semantics.PackageIdentifier]model.ExportedSymbolSpace` — **PRIVATE, unprotected**. `explore-codebase/projects/env.go:37`
- `semantics.PackageIdentifier{OrgName, ModuleName}` — key type for publicSymbols. `explore-codebase/semantics/symbol_resolver.go:620-623`
- `ExportedSymbolSpace.PublicMainSymbols()` — iterates public symbols. `explore-codebase/model/symbol.go:450-460`
- `ExportedSymbolSpace.GetSymbol(name)` — looks up public symbol by name. `explore-codebase/model/symbol.go:460-470`
- `ModuleScope.GetPrefixedSymbol(prefix, name)` — resolves `prefix:name` references. `explore-codebase/model/symbol.go:350-360`

## Compilation artifacts (private — see gaps.md)

- `moduleContext.bLangPkg` — `*ast.BLangPackage` (the AST); set in Phase 1, survives Phase 2. `explore-codebase/projects/module_context.go:495`.
- `moduleContext.compilerCtx` — `*context.CompilerContext` for symbol queries. `explore-codebase/projects/module_context.go:59`.
- `moduleContext.importedSymbols` — `map[string]model.ExportedSymbolSpace` keyed by module name. `explore-codebase/projects/module_context.go:60`.
- `moduleContext.birPkg` — `*bir.BIRPackage` (set after BIR generation). `explore-codebase/projects/module_context.go:61`.
- Accessible only via package-private `moduleContext.getBLangPackage()` (`explore-codebase/projects/module_context.go:495-500`), `getBIRPackage()`, etc. — NOT exposed through the public `Package`/`Module`/`Document` API.
