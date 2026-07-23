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

## Completion architecture (ticket 33 era)

### Cursor-context classifier (AST-only, no CST trivia dependency)

- `classifyContext()` at `ls/core/query/completion_module.go:classifyContext` — walks `tree.ModulePart` members and imports; classifies into `contextFunctionBody`, `contextModulePart`, `contextImport`, `contextAliasMember`, or `contextUnsupported`. Purely AST-based: iterates `part.Members()` and `part.Imports()`, checks `rangeContains(m, offset)` for each member. No CST token/trivia dependency.
- `classifyCompletion()` at `ls/core/query/completion.go:classifyCompletion` — further classifies function-body positions into `bodyStatementStart` vs `bodyExpression`. Uses `classifyBodyPosition()` which scans raw text left of cursor for last non-whitespace char — a text heuristic, not CST-based.
- `cursorInComment()` at `ls/core/query/completion.go:cursorInComment` — text-based scanner tracking string/template literals and line/block comments. Does NOT use CST trivia. Acceptable limitation: `//` inside `${}` interpolation treated as part of template.
- `classifyBodyPosition()` at `ls/core/query/completion_body.go:classifyBodyPosition` — scans left from cursor over whitespace to last non-whitespace char. `{;}` → statement-start; `=(,.+-*/%<>!&|?~^@"'\`` → expression; ident chars → expression; anything else → statement-start (conservative fallback).

### Scope collection (AST-only)

- `collectScope()` at `ls/core/query/completion_body.go:collectScope` — walks AST parameters + preceding local declarations via `tree.NodeList` iterators. No semantic model access. Shadowing: innermost-wins via `seen` map.
- `collectLocalsScope()` at `ls/core/query/completion_body.go:collectLocalsScope` — recurses only into the nested block containing the cursor. Sibling branches (else vs if) are excluded.
- `loopEncloses()` at `ls/core/query/completion_body.go:loopEncloses` — walks statement chain via `descendStatementChain()`, returns true at first while/foreach, false at first fork (scope barrier).

### Semantic facts via non-blocking lease

- `CompletionLease` interface at `ls/core/query/completion.go:CompletionLease` — provides generation-matched, non-blocking access to compiler-precomputed indices: `CompletionIndex`, `ExpectedTypeIndex`, `ImportCatalog`, `MemberCompletionIndex`, `InvocationCompletionIndex`. Query layer never waits for compilation.
- `completeFunctionBody()` at `ls/core/query/completion.go:completeFunctionBody` — acquires lease via `s.compiler.Lease(root, gen)`, reads facts from `lease.Index()`, boosts expected-type via `lease.ExpectedTypeIndex()`, enriches callable snippets and invocation tiers via `lease.InvocationCompletionIndex()`. Falls back to syntax/static only when no matching lease.
- `completeMemberAccess()` at `ls/core/query/completion.go:completeMemberAccess` — reads `lease.MemberCompletionIndex()` for the exact source access slot (kind + dotOffset). No matching slot → empty result.
- `importCatalog()` at `ls/core/query/completion.go:importCatalog` — acquires lease for import module/alias-export catalog. Nil catalog → fall back to syntax/static only.

### Ticket 33: scoped environment handle for AST-only completion

- The query layer currently accesses compiler indices through `CompletionLease` (5 separate index pointers). A "scoped environment handle" would be a narrower interface providing only AST-level context (syntax tree, scope chain, cursor position) without requiring compiler indices.
- Current AST-only paths already exist: `classifyContext`, `classifyCompletion`, `collectScope`, `bodyCatalogItems`, `modulePartItems` — all work without any lease. The handle would formalize this boundary.
- The `Service` struct at `ls/core/query/query.go:Service` holds `projects *workspace.ProjectService` and `compiler CompletionLeaser`. A scoped handle would extract the `projects`-derived AST context (document, module, syntax tree) into a standalone value that `Completion()` methods can use without touching `s.compiler`.
- Key invariant: the handle must be generation-scoped (matched to the current document text) to avoid stale AST reuse after edits. The `Snapshot.Generation` field at `ls/core/workspace/workspace.go` provides this.

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
