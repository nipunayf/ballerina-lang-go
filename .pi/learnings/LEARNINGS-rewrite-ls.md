# LEARNINGS-rewrite-ls

Durable index of what past exploration runs learned about the TypeScript Ballerina Language Server rewrite. Read before searching; update before finishing. Keep entries summarized and pointer-dense — `path` + symbol, one line each.

## Layout

- `ballerina-lang/language-server/` — Java LS (the "pre-Go" rewrite being replaced)
- `ballerina-lang-go/ls/` — Go rewrite (Phase B / Ticket 08)
- `ballerina-language-server-poc/` — TypeScript PoC (diagnostics-only, single-file)

## Key entry points

- Java LS workspace manager: `ballerina-lang/language-server/modules/langserver-core/src/main/java/org/ballerinalang/langserver/workspace/BallerinaWorkspaceManager.java` — `ProjectContext`, per-project `ReentrantLock`, `waitAndGetPackageCompilation`
- Java LS event sync: `ballerina-lang/language-server/modules/langserver-core/src/main/java/org/ballerinalang/langserver/eventsync/publishers/ProjectUpdateEventPublisher.java` — 1s debounce via `CompletableFuture.delayedExecutor`, cancels previous on new change
- Java LS diagnostics: `ballerina-lang/language-server/modules/langserver-core/src/main/java/org/ballerinalang/langserver/diagnostic/DiagnosticsHelper.java` — `compileAndSendDiagnostics`, `schedulePublishDiagnostics`, `latestScheduled` debounce
- Java LS subscribers: `PublishDiagnosticSubscriber.java` and `ResolveModulesSubscriber.java` — both subscribe to `PROJECT_UPDATE`
- Go rewrite workspace: `ballerina-lang-go/ls/ls/core/workspace/workspace.go` — `ProjectService`, `Snapshot` (plain struct, no refcount), `Apply`, `publish`
- Go rewrite compile: `ballerina-lang-go/ls/ls/core/compile/compile.go` — `CompilationService`, synchronous `Compile`, reads published `CurrentPackage`
- Go rewrite event bus: `ballerina-lang-go/ls/ls/core/event/event.go` — synchronous inline dispatch, `ProjectRegistered`/`ProjectEvicted`/`ProjectKindTransitioned`
- Go rewrite project index: `ballerina-lang-go/ls/ls/core/workspace/index.go` — `projectIndex`, LRU eviction, `openDocs` count
- Go rewrite server: `ballerina-lang-go/ls/ls/server/server.go` — synchronous `handleDidOpen`/`handleDidChange` calling `Compile` directly
- Go rewrite package compilation: `ballerina-lang-go/ls/projects/package_compilation.go` — `PackageCompilation`, `sync.Once` compile, two-phase (sequential top-level, parallel local)

## Confirmed facts

### Resolution→compile division (Go rewrite)

- `PackageCompilation.compileModulesInternal()` (`ls/projects/package_compilation.go:80-160`): Phase 1 (sequential) = parse, AST build, import resolution, symbol resolution, top-level type resolution; Phase 2 (parallel) = local node resolution, semantic analysis, CFG creation/analysis, desugaring, BIR generation. Phase 1 errors stop the pipeline before Phase 2.
- `resolveTypesAndSymbols()` (`ls/projects/module_context.go:200-280`): runs stages Parse→ASTBuild→ImportResolution→SymbolResolution→TopLevelTypeResolution sequentially per module, publishes `publicSymbols` map on `Environment` for dependents.
- `analyzeAndDesugar()` (`ls/projects/module_context.go:285-370`): runs LocalNodeResolution→SemanticAnalysis→CFGCreation→CFGAnalysis→Desugaring in parallel across modules via `sync.WaitGroup`.
- `PackageResolution.resolveDependencies()` (`ls/projects/package_resolution.go:200-280`): BFS over package dependency graph, prepends bundled lang libs (10 migrated lang libs + lang.runtime) to the topologically-sorted module list.
- `PackageCompilation` is created once per `packageContext` via `sync.Once` (`ls/projects/package_context.go:130-135`): `getPackageCompilation()` → `newPackageCompilation()` → `compile()` → `compileModulesInternal()`. Re-creation requires a fresh `packageContext` (i.e., a fresh `projects.Load`).

### Snapshot capture after publication (Go rewrite)

- `ProjectService.publish()` (`ls/core/workspace/workspace.go:200-215`): calls `reloadAt()` which does `projects.Load(fsys, sourceRoot)` — a full project load from scratch — then atomically replaces the index entry. `ProjectRegistered` is published on each replacement.
- `reloadAt()` (`ls/core/workspace/workspace.go:220-235`): builds a fresh `palFS` (overlay-augmented filesystem with open-buffer contents), calls `loadProject()`, stores result in `indexEntry.project`, preserves `openDocs` count from old entry.
- `CompilationService.Compile()` (`ls/core/compile/compile.go:100-145`): reads `project.CurrentPackage()` from the freshly-loaded project, then `pkg.Compilation()` triggers `sync.Once` compile. The `PackageCompilation` is a singleton per `packageContext`.
- `server.handleDidOpen()`/`handleDidChange()` (`ls/server/server.go:100-150`): synchronous `Apply` → `Compile` → `publishDiagnostics`. No snapshot capture between Apply and Compile — Compile reads the same project that Apply just published.

### Diagnostic identity/lifetime (Go rewrite)

- `DiagnosticEnv` (`ls/tools/diagnostics/diagnostic_env.go:50-80`): singleton per `CompilerEnvironment` (one per `projects.Load`). `RegisterFile()` panics on re-registration of same name with different `TextDocument` — this is the documented limitation that blocks modifier-chain reuse.
- `registrationKey()` (`ls/projects/document_context.go:50-55`): globally-unique key = `diagKeyPrefix + name`. Prefix is `"<org>/<moduleName>/<version>::"` for bala deps, `"<sourceRoot>/"` for workspace members, `"modules/<moduleNamePart>/"` for build named modules, `""` for default module.
- `buildDiagKeyPrefix()` (`ls/projects/module_context.go:50-70`): constructs the prefix based on project kind and module descriptor.
- `PackageCompilation.collectModuleDiagnostics()` (`ls/projects/package_compilation.go:165-180`): collects per-module diagnostics from `moduleCtx.getDiagnostics()` (which reads `compilerCtx.Diagnostics()`), wraps in `PackageDiagnostic`, filters severity for bala projects.
- Diagnostics are ephemeral per `PackageCompilation` — they live on the `CompilerContext.Diagnostics()` slice and are collected into `PackageCompilation.diagnosticResult` during `compileModulesInternal()`. No diagnostic caching across compilations.

### Per-key supersession/cancellation (Go rewrite)

- **No per-key supersession in Phase B**: `server.handleDidOpen()`/`handleDidChange()` (`ls/server/server.go:100-150`) are synchronous — no debounce, no cancellation, no per-key scheduling. Each notification immediately calls `Apply` then `Compile` then `publishDiagnostics`.
- `ProjectService.Apply()` (`ls/core/workspace/workspace.go:170-200`): checks `ctx.Err()` before publication but Phase B passes `context.Background()` — cancellation is threaded but unused.
- `server.Shutdown()` (`ls/core/workspace/workspace.go:280-285`): no-op, comment says "Ticket 09 fills with real cancellation".
- `event.Bus` (`ls/core/event/event.go:100-140`): synchronous inline dispatch — no queues, no tiers, no cancellation. `Publish` blocks until all subscribers return.
- `projectIndex.putExisting()` (`ls/core/workspace/index.go:80-95`): atomically replaces index entry for same source root — this is the only "supersession" mechanism: a new `reloadAt` replaces the old project in the index, and the old project is dropped (no explicit cancellation).

### Event ordering (Go rewrite)

- `ProjectService.Apply()` → `publish()` → `reloadAt()` → `index.putExisting()` → `bus.Publish(ProjectRegistered)` (`ls/core/workspace/workspace.go:200-215`): publication happens after the index entry is replaced, so subscribers see the new project.
- `CompilationService.handleEvent()` (`ls/core/compile/compile.go:60-80`): updates `knownRoots` set on `ProjectRegistered`/`ProjectEvicted`/`ProjectKindTransitioned`. `knownRoots` is checked in `Compile()` before reading `CurrentPackage`.
- `server.handleDidOpen()`/`handleDidChange()` (`ls/server/server.go:100-150`): `Apply` (which publishes `ProjectRegistered`) → `Compile` (which checks `knownRoots` and reads `CurrentPackage`). Since both are synchronous on the same goroutine, ordering is deterministic: the event is dispatched and the subscriber updates `knownRoots` before `Compile` runs.
- `applyBallerinaTomlChange()` (`ls/core/workspace/workspace.go:240-270`): publishes `ProjectKindTransitioned` (or `ProjectEvicted`+`ProjectRegistered` pair) after `reloadAt()`. The old root is evicted from the memo before the new root is resolved.

### Transferable invariants for Go compiler

- **Two-phase compilation**: Phase 1 (sequential, topologically sorted) → Phase 2 (parallel). This is the core invariant that the Go compiler must preserve. The `erroredModules` set and `dependencyErrored` check (`ls/projects/package_compilation.go:100-120`) prevent cascading noise.
- **`sync.Once` compile**: `PackageCompilation.compile()` uses `sync.Once` (`ls/projects/package_compilation.go:60-65`). The Go compiler must ensure compile is idempotent or use a similar guard.
- **`publicSymbols` map on Environment**: Phase 1 publishes `publicSymbols[semantics.PackageIdentifier{...}] = exported` (`ls/projects/module_context.go:260-265`). The Go compiler must provide a cross-module symbol publication mechanism.
- **`DiagnosticEnv` per `CompilerEnvironment`**: singleton per load, panics on re-registration. The Go compiler must either (a) lift this to per-instance file identity, or (b) accept full re-Load per content change.
- **`registrationKey()` uniqueness**: globally-unique keys prevent cross-package collisions. The Go compiler must use the same prefix scheme.
- **Bundled lang libs**: 10 migrated lang libs + lang.runtime are compiled ahead of the root package and seeded into implicit imports (`ls/projects/package_resolution.go:150-190`). The Go compiler must replicate this.

### Java-specific mechanisms (flag for Go compiler)

- **`sync.Once` vs Java's lazy-init**: Go uses `sync.Once` for `PackageCompilation` and `PackageResolution` (`ls/projects/package_context.go:130-145`). Java uses `CompletableFuture`-based lazy init. Both achieve the same goal; the Go pattern is simpler.
- **`sync.Map` for module cache**: `Package.Module()` uses `sync.Map.LoadOrStore` (`ls/projects/package.go:80-90`). Java uses `ConcurrentHashMap.computeIfAbsent()`. Semantically equivalent.
- **`sync.WaitGroup` for parallel Phase 2**: Go uses `sync.WaitGroup` with goroutines (`ls/projects/package_compilation.go:130-150`). Java uses `CompletableFuture.allOf()`. The Go pattern is simpler and has explicit panic recovery.
- **`sync.Mutex` for `knownRoots`**: `CompilationService` uses `sync.Mutex` for the `knownRoots` set (`ls/core/compile/compile.go:50-55`). Java would use `ReentrantLock` or synchronized. Trivially portable.
- **`context.Context` for cancellation**: threaded through `Apply` and `Compile` but unused in Phase B (`ls/core/workspace/workspace.go:190-195`, `ls/core/compile/compile.go:105`). Java uses `CompletableFuture.cancel()`. The Go compiler should use `context.WithCancel` per-key.

### Choices still requiring HIL

1. **Debounce strategy**: Phase B has no debounce. Ticket 09 must decide: single debounce layer (where?), per-key scheduling (how?), cancellation model (context.WithCancel per source root?).
2. **Dual-snapshot engine**: Snapshot is a plain struct with a "Ticket 09 wraps with release func()" comment. The refcount model, snapshot identity, and release protocol are undefined.
3. **`DiagnosticEnv` per-instance file identity**: The panic-on-re-registration blocks modifier-chain reuse. HIL must decide whether to lift this (preferred) or keep full re-Load.
4. **ADR-042 modifier chain**: Deferred. HIL must decide the modifier API shape (Document.Modify().WithContent().Apply() → setCurrentPackage) and how it interacts with the dual-snapshot engine.
5. **CE lifecycle events**: CE-E1–CE-E7 are reserved in `event.go` but undefined. HIL must define the event types, their payloads, and their ordering relative to WM events.
6. **`ProjectUpdated` (WM-E4)**: Reserved but undefined. HIL must decide whether this replaces or supplements `ProjectRegistered` on content changes.
7. **`expr:` scheme support**: Java's `ClonedWorkspace` duplicates the entire project. HIL must decide the Go approach (overlay FS, virtual documents, or something else).
8. **`compilationCrashed` equivalent**: Go rewrite has no crash flag. HIL must decide whether to add one or handle BAD_SAD/CYCLIC_MODULE as regular diagnostics.
9. **`Shutdown` cancellation**: No-op in Phase B. HIL must define the shutdown protocol: cancel all pending compilations, drain event bus, release snapshots.

### Java LS async compilation model

- `ProjectUpdateEventPublisher.publish()` (`BallerinaTextDocumentService.java:586-593`): debounces with 1s delay, cancels previous `latestScheduled` via `completeExceptionally`
- `DiagnosticsHelper.compileAndSendDiagnostics()` (`DiagnosticsHelper.java:130-140`): second debounce layer with `latestScheduled`, 1s delay, then `thenApplyAsync` → `waitAndGetPackageCompilation` → `thenAccept` → `compileAndSendDiagnostics`
- `BallerinaWorkspaceManager.waitAndGetPackageCompilation()` (`BallerinaWorkspaceManager.java:230-260`): acquires per-project `ReentrantLock`, calls `project.currentPackage().getCompilation()`, checks for `BAD_SAD_FROM_COMPILER`/`CYCLIC_MODULE_IMPORTS_DETECTED` to set `compilationCrashed`
- `BallerinaWorkspaceManager.createOrGetProjectPair()` (`BallerinaWorkspaceManager.java`): creates fresh `Project` via `BuildProject.load()` or `SingleFileProject.load()`, wraps in `ProjectContext` with lock
- `BallerinaWorkspaceManagerProxyImpl` (`BallerinaWorkspaceManagerProxyImpl.java`): dual workspace managers — `baseWorkspaceManager` for `file:` scheme, `ClonedWorkspace` for `expr:` scheme (inlay hints, code actions on virtual documents)
- `ClonedWorkspace` (`BallerinaWorkspaceManagerProxyImpl.java:80-100`): duplicates project via `project.duplicate()`, stores in separate `sourceRootToProject` map, removed on `didClose`

### Go rewrite current state (Phase B / Ticket 08)

- `ProjectService.Apply()` (`workspace.go:170-200`): synchronous, updates `documents` map, resolves source root, calls `publish()` which does `reloadAt()` (fresh `projects.Load` per publication)
- `CompilationService.Compile()` (`compile.go:100-145`): synchronous, reads published `CurrentPackage` via `ProjectService.Project()`, checks `knownRoots`, iterates diagnostics
- `event.Bus` (`event.go:100-140`): synchronous inline dispatch, no goroutines/queues/tiers, panics recovered and logged
- `projectIndex` (`index.go:50-120`): count-bounded LRU, `openDocs` count for background-preferred eviction, `pathToSourceRoot` memo for ADR-048 root-walk
- `Snapshot` (`workspace.go:50-60`): plain struct, no refcount, no `Release` method — comment says "Ticket 09 wraps Snapshot with a release func()"
- `PackageCompilation.compileModulesInternal()` (`package_compilation.go:80-160`): two-phase — sequential top-level resolution (Phase 1), parallel local analysis (Phase 2) with `sync.WaitGroup`
- `server.handleDidOpen()`/`handleDidChange()` (`server.go:100-150`): synchronous — `Apply` then `Compile` then `publishDiagnostics`, no debounce/cancellation
- `server.Shutdown()` (`workspace.go:280-285`): no-op, comment says "Ticket 09 fills with real cancellation"

### Event kinds (Java LS)

- `EventKind.PROJECT_UPDATE` — published on didOpen, didChange, loadProject
- `EventKind` enum: `ballerina-lang/language-server/modules/langserver-commons/src/main/java/org/ballerinalang/langserver/commons/eventsync/EventKind.java`

### Event kinds (Go rewrite)

- `ProjectRegistered` (WM-E1) — published on first load
- `ProjectEvicted` (WM-E2) — published on LRU eviction or kind transition
- `ProjectKindTransitioned` (WM-E3) — published on SingleFile↔Build transition
- `ProjectUpdated` (WM-E4) — reserved for Ticket 09
- CE lifecycle events (CE-E1–CE-E7) — reserved for Ticket 09

## Legacy/workaround behavior (don't cargo-cult)

- **Java LS double debounce**: `ProjectUpdateEventPublisher` debounces at 1s, then `DiagnosticsHelper.compileAndSendDiagnostics` debounces again at 1s — two layers of the same mechanism. The Go rewrite should have a single, well-defined debounce/scheduling layer.
- **Java LS `completeExceptionally` cancellation**: cancels previous `CompletableFuture` by completing it exceptionally with a "Cancelled" throwable. This is a Java idiom; Go should use `context.WithCancel` per-key.
- **Java LS per-project `ReentrantLock`**: coarse-grained lock around the entire project. The Go rewrite's ADR-042 modifier chain (deferred) is a better model.
- **Java LS `ClonedWorkspace` for `expr:` scheme**: duplicates the entire project for virtual documents. The Go rewrite should consider a lighter approach (e.g., overlay FS with virtual documents).
- **Java LS `compilationCrashed` flag**: set on `BAD_SAD_FROM_COMPILER`/`CYCLIC_MODULE_IMPORTS_DETECTED`, blocks further `semanticModel` calls. The Go rewrite should handle these as regular diagnostics rather than a crash flag.
- **Java LS `PackageCompilation` re-creation on every change**: `createOrGetProjectPair` loads a fresh project each time. The Go rewrite's `reloadAt` does the same — both pay full re-Load cost. ADR-042 modifier chain is the deferred fix.
- **Java LS `DiagnosticEnv` singleton per Load**: documented in Go workspace.go comment as "a per-Load singleton that panics on re-registration of a changed file" — the Go rewrite's `reloadAt` works around this by loading a fresh project per publication.

## Dead ends

- No ADR directory exists in `ballerina-lang/language-server/docs/` — ADRs are referenced by number in Go rewrite comments but not stored locally
- No "dual-snapshot" string found anywhere in the Java LS or Go rewrite — the concept is described only in Go rewrite comments as "Ticket 09" work
- The TypeScript PoC (`ballerina-language-server-poc/`) is a single-file diagnostics-only prototype with no relevance to the dual-snapshot engine
