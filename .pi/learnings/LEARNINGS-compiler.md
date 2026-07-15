# LEARNINGS-compiler

Durable index of what past exploration runs learned about this repo's compiler packages (ast, model, semantics, projects, context). Read before searching; update before finishing. Keep entries summarized and pointer-dense — `path` + symbol, one line each.

## Layout

- `ls-ref/` is the canonical Go compiler; `main/` is a mirror. All paths below are relative to `ls-ref/`.
- `context/` — front-end state: `CompilerContext` (per-module), `CompilerEnvironment` (shared across modules).
- `projects/` — project/package/module/document model, resolution, compilation orchestration.
- `model/` — symbol types, scopes, symbol spaces, package IDs.
- `semantics/` — symbol resolution, type resolution, semantic analysis, CFG.
- `ast/` — BLangPackage, BLangCompilationUnit, AST node types.
- `tools/diagnostics/` — `Diagnostic`, `DiagnosticEnv`, `Location` (byte-offset based).

## Key entry points

- `projects.Load()` — entry point for loading a project from fs.FS. Returns `ProjectLoadResult`.
- `Package.Compilation()` — triggers full compilation, returns `*PackageCompilation`.
- `PackageCompilation.DiagnosticResult()` — returns `DiagnosticResult` with all diagnostics.
- `PackageCompilation.DiagnosticEnv()` — returns `*diagnostics.DiagnosticEnv` for offset→line/col resolution.
- `Package.Modify()` / `Module.Modify()` / `Document.Modify()` — immutable modification chain.
- `context.NewCompilerContext(env)` — creates per-module compiler context.
- `context.NewCompilerEnvironment(typeEnv, statsEnabled)` — shared environment.

## Confirmed facts

### Cancellation / context propagation
- **No `context.Context` in compilation pipeline.** The `context` package is the compiler's own `CompilerContext`, not Go's `context.Context`. Zero cancellation checkpoints exist in `resolveTypesAndSymbols()` or `analyzeAndDesugar()`.
- `context.Context` is used only in the resolution layer: `Repository.GetPackage(ctx)`, `PackageResolver.ResolvePackages(ctx)`, `workspaceRepository.GetPackage(ctx)`. These check `ctx.Done()` before I/O.
- `packageContext.getPackageCompilation()` uses `sync.Once` — no cancellation possible once compilation starts.
- `packageContext.getResolution()` uses `sync.Once` — no cancellation possible once resolution starts.
- `PackageCompilation.compileModulesInternal()` runs Phase 2 goroutines with `sync.WaitGroup` and no cancellation channel.

### Safety / immutability / concurrency
- **`Package`, `Module`, `Document` are immutable** — use `Modify().Apply()` to create modified copies. `projects/package.go:28`, `projects/module.go:28`, `projects/document.go:28`.
- **`PackageCompilation` is not immutable** — `compileOnce sync.Once` means it compiles exactly once, but the `DiagnosticResult` is set after compilation and never updated. `projects/package_compilation.go:38-48`.
- **`PackageResolution` is not immutable** — built once in `newPackageResolution()`, fields are set during construction. `projects/package_resolution.go:30-40`.
- **`CompilerContext` is not thread-safe** — `mu sync.Mutex` protects `diagnostics` slice, but `stage` field is unprotected. `context/context.go:38-42`.
- **`CompilerEnvironment` is partially thread-safe** — `symbolSpacesMu sync.RWMutex` protects `symbolSpaces` slice; `underlyingSymbol sync.Map` is concurrent-safe. `context/env.go:37-43`.
- **`SymbolSpace` is thread-safe** — `mu sync.RWMutex` on lookup table and symbols slice. `model/symbol.go:180-185`.
- **`DiagnosticEnv` is thread-safe** — `mu sync.RWMutex` protects file registrations. `tools/diagnostics/diagnostic_env.go:35-39`.
- **`PackageCache` is thread-safe** — `mu sync.RWMutex`. `projects/package_cache.go:16-20`.
- **`DependencyGraph` is thread-safe** — `sortOnce sync.Once` for lazy topological sort. `projects/dependency_graph.go:24-30`.
- **`Environment` is not thread-safe** — `publicSymbols map` is unprotected. `projects/env.go:22-28`.

### Snapshot / diagnostics capture
- **`DiagnosticResult` is a value type** — created via `NewDiagnosticResult(diags)` which clones the slice and categorizes by severity. `projects/diagnostic_result.go:22-48`.
- **`CompilerContext.Diagnostics()` returns the raw `[]diagnostics.Diagnostic` slice** — not a copy. Callers must clone if they need to retain across mutations. `context/context.go:103`.
- **`DiagnosticEnv` stores `text.TextDocument` for each registered file** — used to resolve byte offsets to line/column. `tools/diagnostics/diagnostic_env.go:37`.
- **`DiagnosticEnv.RegisterFile(fileName, doc)`** — called during compilation to register source files. `projects/package_compilation.go:87-91`.
- **`Location` stores `fileIndex int` + `startOffset/endOffset int`** — fileIndex is 1-based index into DiagnosticEnv's file list. `tools/diagnostics/location.go:17-21`.
- **`DiagnosticEnv` is shared across all modules** — registered in `compileModulesInternal()` before Phase 1. `projects/package_compilation.go:87-91`.

### How to snapshot `CurrentPackage` safely
- `Package` is immutable — `p.duplicate(project)` creates a deep copy with independent module contexts. `projects/package.go:280-283`.
- `Project.Duplicate()` creates a deep copy with fresh `Environment` (new caches, same repos). `projects/build_project.go:155-163`.
- `Environment.Duplicate()` creates a new env with fresh `PackageCache` but same `CompilerEnvironment` (shared symbol spaces!). `projects/env.go:82-91`.
- **Critical risk**: `Environment.Duplicate()` shares the same `*context.CompilerEnvironment` pointer — symbol spaces are shared between original and duplicate. `projects/env.go:84`.
- To snapshot safely: call `Project.Duplicate()` then `dup.CurrentPackage()`. The duplicate has its own `PackageCache` but shares `CompilerEnvironment`/symbol spaces.

### Resource / shutdown
- **No `Close()` or `Shutdown()` on any type** — `Project`, `Package`, `PackageCompilation`, `Environment` have no cleanup methods.
- `Project.Save()` is a stub (no-op for all implementations). `projects/build_project.go:155`.
- `BalaProject` is read-only. `projects/bala_project.go:107`.
- `SingleFileProject.Save()` is a no-op. `projects/single_file_project.go:103`.

### Compilation pipeline details
- **Phase 1** (sequential): parse → AST → import resolution → symbol resolution → top-level type resolution. `projects/module_context.go:260-310`.
- **Phase 2** (parallel per module): local node resolution → semantic analysis → CFG → CFG analysis → desugar. `projects/module_context.go:320-400`.
- **BIR generation** is separate — called via `generateCodeInternal()`. `projects/module_context.go:480-487`.
- **Error handling**: if any module has errors after Phase 1, Phase 2 is skipped entirely. `projects/package_compilation.go:130-136`.
- **Phase 2 panics** are caught and re-panicked (not converted to diagnostics). `projects/package_compilation.go:170-175`.

## Known gaps (feature needs the compiler doesn't expose)

- **No `SemanticModel` type** — `PackageCompilation.SemanticModel()` returns `any` with TODO. `projects/package_compilation.go:223-227`.
- **No cancellation checkpoints in compilation** — `context.Context` not threaded through `resolveTypesAndSymbols()` or `analyzeAndDesugar()`. No way to cancel an in-flight compilation.
- **No incremental compilation** — `PackageCompilation.compile()` always recompiles everything. No change-tracking.
- **No per-document compilation** — compilation is always at module/package level.
- **No way to get diagnostics for a single document** — `moduleContext.getDiagnostics()` returns all diagnostics for the module. No per-document filtering.
- **`Environment.Duplicate()` shares `CompilerEnvironment`** — symbol spaces are not deep-copied. A snapshot that modifies symbols would corrupt the original.
- **No `Close()` / resource cleanup** on any project/package/compilation type.
- **`CompilerContext` stage field is unprotected** — concurrent `StartStage`/`EndStage` would race.
- **`publicSymbols` map in `Environment` is unprotected** — concurrent access during parallel Phase 2 could race.

## Ticket 09 findings (DiagnosticEnv identity + resolution/compilation split)

### DiagnosticEnv is a per-CompilerEnvironment singleton, shared across all compilations
- `CompilerEnvironment.diagnosticContext` is created once in `NewCompilerEnvironment()` and never replaced. `context/env.go:37`.
- `Environment.Duplicate()` shares the same `*context.CompilerEnvironment` pointer — no deep copy. `projects/env.go:84`.
- `PackageCompilation.DiagnosticEnv()` returns `c.compilerEnv.DiagnosticEnv()` — the shared singleton. `projects/package_compilation.go:218`.
- `compileModulesInternal()` registers all module source files into this shared env. `projects/package_compilation.go:87-91`.
- `DiagnosticEnv.RegisterFile()` updates in-place when same name re-registered. `tools/diagnostics/diagnostic_env.go:52-56`.
- **Consequence**: two concurrent compilations (stable + in-progress) share the same DiagnosticEnv; the second overwrites the first's text documents, corrupting offset→line/col resolution for the first's diagnostics.

### Resolution and Compilation are already separate APIs, but both use sync.Once
- `Package.Resolution()` → `packageContext.getResolution()` uses `sync.Once`. `projects/package_context.go:130-140`.
- `Package.Compilation()` → `packageContext.getPackageCompilation()` uses `sync.Once`. `projects/package_context.go:123-128`.
- `getPackageCompilation()` calls `getResolution()` internally — resolution is always a prerequisite. `projects/package_compilation.go:39-40`.
- Both are lazy: first access triggers the work, subsequent accesses return cached result.
- **Consequence**: after modifier chain, the new `packageContext` has fresh `sync.Once` fields, so resolution/compilation re-triggers. But the shared `DiagnosticEnv` means the new compilation's file registrations overwrite the old one's.

### Modifier chain creates new Package with fresh packageContext
- `Document.Modify().WithContent().Apply()` → new `Document` → `ModuleModifier.updateDocument()` → `ModuleModifier.Apply()` → `PackageModifier.updateModule()` → `PackageModifier.Apply()` → `setCurrentPackage()`. `projects/document.go:80-90`, `projects/module.go:130-140`, `projects/package.go:280-283`.
- `PackageModifier.Apply()` calls `newPackageContextFromMaps()` which creates a fresh `packageContext` with zero-value `sync.Once` fields. `projects/package.go:280-283`.
- The new `Package` shares the same `Environment` (and thus same `CompilerEnvironment`/`DiagnosticEnv`).
- **Consequence**: modifier chain publication is safe for single-snapshot use, but dual-snapshot (stable + in-progress) needs per-instance DiagnosticEnv.

### LS workspace currently avoids modifier chain, pays full re-Load
- `workspace.go:195-200` comment explicitly documents: "The DiagnosticEnv same-name/different-TextDocument panic blocks the ADR-042 modifier chain until a later ticket lifts DiagnosticEnv to per-instance file identity."
- `workspace.publish()` calls `reloadAt()` which does `projects.Load()` with a fresh palFS — full re-Load per content change. `workspace.go:210-220`.
- `CompilationService.Compile()` reads the published `CurrentPackage()` and calls `pkg.Compilation()`. `compile.go:100-120`.
- **Consequence**: Phase B pays full re-Load cost. Ticket 09's per-instance DiagnosticEnv enables the modifier chain, eliminating re-Load.

### No generation/version key on PackageCompilation or DiagnosticEnv
- `PackageCompilation` has no generation counter, snapshot ID, or document registration key. `projects/package_compilation.go:22-32`.
- `Package` has no generation counter. `projects/package.go:22-30`.
- `packageContext` has no generation counter. `projects/package_context.go:22-35`.
- `DiagnosticEnv` has no generation counter. `tools/diagnostics/diagnostic_env.go:35-39`.
- **Consequence**: no way to distinguish stable vs in-progress snapshots at the DiagnosticEnv level.

### No cancellation in compilation pipeline
- `resolveTypesAndSymbols()` takes no `context.Context`. `projects/module_context.go:260`.
- `analyzeAndDesugar()` takes no `context.Context`. `projects/module_context.go:320`.
- Phase 2 goroutines use `sync.WaitGroup` with no cancellation channel. `projects/package_compilation.go:150-175`.
- `sync.Once` means no way to abort once started. `projects/package_context.go:123-128`.
- **Consequence**: no way to cancel an in-flight compilation when a newer edit arrives.

### LS workspace index provides source-root memoization and LRU eviction
- `projectIndex` is a count-bounded, source-root-keyed LRU cache. `ls/core/workspace/index.go:30-38`.
- `pathToSourceRoot` memo avoids repeated ADR-048 root walks. `ls/core/workspace/index.go:35`.
- Eviction publishes `ProjectEvicted` on the event bus. `ls/core/workspace/index.go:100-110`.
- `CompilationService` subscribes to maintain `knownRoots` set. `ls/core/compile/compile.go:60-75`.
- **Consequence**: the index is the right place to add generation tracking for dual-snapshot support.

## Dead ends

- `context/` package is the compiler's own context, NOT Go's `context.Context`. Don't confuse them.
- `main/` is a mirror of `ls-ref/` — always use `ls-ref/` as canonical.
- `model.SymbolRef` must never be used as a map key (per AGENTS.md). Use `model.SymbolRef` as value, not key.
