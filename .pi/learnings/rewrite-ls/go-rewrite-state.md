# Go rewrite state (`ballerina-lang-go/ls/`)

Keep entries summarized and pointer-dense — `path` + symbol, one line each.
This code evolves per ticket — verify pointers before reusing. Facts below were
recorded around Phase B / tickets 08-09; items marked "Phase B" may be superseded.

## Key entry points

- Workspace: `ls/core/workspace/workspace.go` — `ProjectService`, `Snapshot` (plain struct), `Apply`, `publish`
- Compile: `ls/core/compile/compile.go` — `CompilationService`, synchronous `Compile`, reads published `CurrentPackage`
- Event bus: `ls/core/event/event.go` — synchronous inline dispatch
- Project index: `ls/core/workspace/index.go` — `projectIndex`, LRU eviction, `openDocs` count, `pathToSourceRoot` memo (ADR-048 root-walk)
- Server: `ls/server/server.go` — synchronous `handleDidOpen`/`handleDidChange` calling `Compile` directly
- Package compilation: `projects/package_compilation.go` — `PackageCompilation`, `sync.Once` compile, two-phase

## Resolution→compile division

- `PackageCompilation.compileModulesInternal()` (`projects/package_compilation.go:80-160`): Phase 1 (sequential) = parse, AST, import resolution, symbol resolution, top-level type resolution; Phase 2 (parallel) = local node resolution, semantic analysis, CFG, desugar, BIR. Phase 1 errors stop before Phase 2.
- `resolveTypesAndSymbols()` (`projects/module_context.go:200-280`): sequential stages per module; publishes `publicSymbols` map on `Environment` for dependents.
- `analyzeAndDesugar()` (`projects/module_context.go:285-370`): parallel across modules via `sync.WaitGroup`.
- `PackageResolution.resolveDependencies()` (`projects/package_resolution.go:200-280`): BFS over package dependency graph; prepends bundled lang libs (10 migrated + lang.runtime) to the topologically-sorted module list.
- `PackageCompilation` is created once per `packageContext` via `sync.Once` (`projects/package_context.go:130-135`); re-creation requires a fresh `packageContext`.

## Workspace publication & snapshot capture

- `ProjectService.Apply()` (`ls/core/workspace/workspace.go:170-200`): synchronous; updates `documents` map, resolves source root, calls `publish()`. Checks `ctx.Err()` before publication.
- `publish()` — ADR-042 modifier-chain publication: first publish does `projects.Load` to seed; subsequent publishes use `Document.Modify().WithContent().Apply()` (`workspace.go:280-300`). (Phase B behavior was full `reloadAt()` re-Load per change.)
- `reloadAt()` (`workspace.go:220-235` / `320-340`): builds fresh palFS (overlay-augmented FS with open-buffer contents), loads brand-new project (fresh CompilerEnvironment), replaces index entry preserving `openDocs`.
- `CompilationService.Compile()` (`ls/core/compile/compile.go:100-145`): reads `project.CurrentPackage()`, checks `knownRoots`, `pkg.Compilation()` triggers `sync.Once` compile.
- `projectIndex.putExisting()` (`ls/core/workspace/index.go:80-95`): atomically replaces the entry for a source root — the basic supersession mechanism.

## Diagnostic identity

- `DiagnosticEnv` (`tools/diagnostics/diagnostic_env.go:50-80`): singleton per `CompilerEnvironment` (one per `projects.Load`). Same-name re-registration semantics blocked the modifier chain pre-ticket-09.
- `registrationKey()` (`projects/document_context.go:50-55`): globally-unique key = `diagKeyPrefix + name`; prefix `"<org>/<moduleName>/<version>::"` for bala deps, `"<sourceRoot>/"` for workspace members, `"modules/<moduleNamePart>/"` for build named modules, `""` for default module. Built by `buildDiagKeyPrefix()` (`projects/module_context.go:50-70`).
- `PackageCompilation.collectModuleDiagnostics()` (`projects/package_compilation.go:165-180`): per-module diagnostics from `compilerCtx.Diagnostics()`, wrapped in `PackageDiagnostic`, severity-filtered for bala projects.

## Event bus & ordering

- `event.Bus` (`ls/core/event/event.go:100-140`): synchronous inline dispatch — no queues/goroutines; `Publish` blocks until all subscribers return; panics recovered and logged.
- Ordering: `Apply` → `publish()` → index replace → `bus.Publish(ProjectRegistered)`; then `Compile` checks `knownRoots` — deterministic because everything is on one goroutine.
- `applyBallerinaTomlChange()` (`workspace.go:240-270`): publishes `ProjectKindTransitioned` (or Evicted+Registered pair) after `reloadAt()`; old root evicted from memo before new root resolved.
- Event kinds: `ProjectRegistered` (WM-E1), `ProjectEvicted` (WM-E2), `ProjectKindTransitioned` (WM-E3), `ProjectUpdated` (WM-E4, reserved), CE lifecycle events (CE-E1–CE-E7, defined in ticket 09+ — see compiler/ls-core-surface.md for the current list).

## Transferable invariants (for the Go compiler work)

- **Two-phase compilation** (sequential topo-sorted Phase 1 → parallel Phase 2); `erroredModules` + `dependencyErrored` checks prevent cascading noise (`projects/package_compilation.go:100-120`).
- **`sync.Once` compile** — compile must stay idempotent-or-guarded.
- **`publicSymbols` on Environment** — cross-module symbol publication mechanism (`projects/module_context.go:260-265`).
- **`DiagnosticEnv` per `CompilerEnvironment`** — must be lifted to per-instance file identity, or accept full re-Load per change.
- **`registrationKey()` uniqueness** — same prefix scheme required to avoid cross-package collisions.
- **Bundled lang libs** — 10 migrated lang libs + lang.runtime compiled ahead of the root package and seeded into implicit imports (`projects/package_resolution.go:150-190`).

## Java ↔ Go mechanism mapping

- `sync.Once` ↔ Java `CompletableFuture` lazy-init; `sync.Map.LoadOrStore` ↔ `ConcurrentHashMap.computeIfAbsent`; `sync.WaitGroup` ↔ `CompletableFuture.allOf`; `sync.Mutex` ↔ `ReentrantLock`; `context.WithCancel` per key ↔ `CompletableFuture.cancel()`.

## Choices recorded as requiring HIL (ticket 09 era — check what has since landed)

1. Debounce strategy (single layer, per-key scheduling, cancellation model).
2. Dual-snapshot engine (refcount model, snapshot identity, release protocol).
3. `DiagnosticEnv` per-instance file identity (lift vs keep full re-Load).
4. ADR-042 modifier chain API shape and dual-snapshot interaction.
5. CE lifecycle events CE-E1–CE-E7 (types, payloads, ordering vs WM events).
6. `ProjectUpdated` (WM-E4): replaces or supplements `ProjectRegistered`?
7. `expr:` scheme support (overlay FS vs virtual documents vs project clone).
8. `compilationCrashed` equivalent (flag vs regular diagnostics for BAD_SAD/CYCLIC_MODULE).
9. Shutdown protocol (cancel pending compiles, drain bus, release snapshots).
