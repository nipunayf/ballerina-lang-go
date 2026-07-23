# Diagnostics pipeline

## Ticket-08 (legacy) diagnostics

- `lsp/diagnostics.go:runDiagnostics` — full compile pipeline per snapshot, debounced via `scheduleDiagnostics` with stale-job guard
- `lsp/diagnostics.go:runModuleFrontend` — 7 stages: Parse → SymbolResolve → TopLevelTypeResolve → LocalTypeResolve → SemanticAnalyze → CFGBuilt → CFGAnalyzed
- `lsp/diagnostics.go:runLocalPackagePipelines` — goroutine per module for stages 5-7
- `lsp/diagnostics.go:runChangedModuleDiagnostics` — runs only changed module through full pipeline, dependencies through TopLevelTypeResolve
- `lsp/diagnostics.go:parseModuleImportCompilationUnits` — uses `parser.GetImportSyntaxTree` for faster import-only parsing
- `lsp/diagnostics.go:parseModuleCompilationUnits` — skips re-parse if `module.CompilationUnits[uri]` exists
- `lsp/diagnostics.go:topologicalModuleOrder` — Kahn's algorithm, cycle detection
- `lsp/diagnostics.go:lspPositionMapper` — precomputes line starts, batch-converts byte offsets to UTF-16 positions
- `lsp/diagnostics.go:lspPositionMapper.Positions` — sorts offsets, walks content once for all positions
- `lsp/diagnostics.go:lspSeverity` — maps Warning→2, Info→3, Hint→4, default→1
- `lsp/diagnostics.go:diagnosticSource = "ballerina-go"`
- `lsp/diagnostics.go:lspPositionMapper.Position` — surrogates (>=0x10000) count as 2 UTF-16 code units
- `lsp/diagnostics.go:convertDiagnostics` — maps `compile.CompilerDiagnostic` (byte-offset positions) to `protocol.Diagnostic` (UTF-16 positions)
- `lsp/server.go:scheduleDiagnostics` — goroutine with `time.Sleep(delay)` before enqueueing
- `lsp/server.go:handleDiagnosticJob` — checks `job.seq == latestSeq && job.updateSeq == latestUpdateSeq` before publishing
- `lsp/server.go:publishDiagnostics` — checks `updateSeq == s.updateSequence.Load()` and `manager.IsCurrent(snapshot)` before publishing
- `lsp/server.go:updateSequence` — atomic int64 incremented on every didOpen/didChange/didClose/didSave/didChangeWatchedFiles
- `lsp/server.go:diagnosticSequence` — atomic int64 for debounced diagnostic job ordering
- `lsp/server.go:beginIndexingProgress`/`endIndexingProgress` — `window/workDoneProgress/create` + `$/progress` notifications

## Ticket-09 diagnostics (current)

- `ls/ls/core/compile/compile.go:CompilationService` — dual-snapshot compilation engine. Subscribes to `ProjectUpdated` (CRITICAL) to enqueue compile cycles on a bounded worker pool.
- `ls/ls/core/compile/compile.go:enqueueCycle()` — CRITICAL-tier subscriber for ProjectUpdated. With non-zero debounce, coalesces rapid changes via per-root timer (`scheduleDebounced`). With zero debounce, enqueues immediately.
- `ls/ls/core/compile/compile.go:runCycle()` — pre-compile stale check (generation match), compile (panic-recovered → CE-E2), then `SnapshotStore.publishStable` (gate + store + CE-E1/E5a/E5b).
- `ls/ls/core/compile/compile.go:realCompilePackage()` — runs the package's compile and extracts all diagnostics grouped by file, plus resolution subset and resolution-error flag. Also builds completion-specific semantic indices.
- `ls/ls/core/compile/compile.go:extractByFile()` — extracts diagnostics grouped by `env.FileName` key (= document's SyntaxTree.FilePath). Documents resolved within the captured immutable package so extraction never reads the live currentPackage (which a concurrent Apply's modifier chain may swap).
- `ls/ls/core/compile/compile.go:DiagnosticsFor()` — reads the latest stable snapshot for root and returns per-open-document diagnostics. The caller performs the generation-staleness guard before publishing.
- `ls/ls/core/compile/compile.go:Compile()` — synchronous fast path for semantic-query consumers. Reads `SnapshotStore.Stable` first (cache hit) and falls back to compiling inline.
- `ls/ls/server/server.go:subscribeDiagnostics()` — subscribes to CE-E5a/CE-E5b (BEST_EFFORT). On each event, reads stable snapshot via `compile.DiagnosticsFor`, applies generation-staleness guard per document, converts to protocol.Diagnostic, and writes publishDiagnostics per open document.
- `ls/ls/server/server.go:publishRootDiagnostics()` — publishes the current stable snapshot's diagnostics for every open document under root, gated by the event's generation. First-wins per generation: whichever of CE-E5a/CE-E5b delivers first publishes the complete set; the other is a no-op.
- `ls/ls/server/server.go:handleDidOpen/Change/Close()` — applies the document change and returns nil. Diagnostics arrive later via the CE subscriber (branch 5). No synchronous Compile or publishDiagnostics.

## DiagnosticEnv registration and BeginCompile

- `tools/diagnostics/diagnostic_env.go:DiagnosticEnv` — resolves byte-offset-based Locations to line/column numbers. Maps file names to integer indices for compact storage in Location. Thread-safe via RWMutex.
- `tools/diagnostics/diagnostic_env.go:BeginCompile()` — mints a new `compileInstance` token, marks it active on the env, and allocates its fileName namespace. Must be called before the compile registers files or resolves Locations. Under the LS single-flight-per-root rule at most one compile is active on a given env at a time. Returns the token to pass to `EndCompile`.
- `tools/diagnostics/diagnostic_env.go:EndCompile()` — clears the active instance. The instance's fileName namespace is retained so Locations built during that compile keep resolving via their fileIndex.
- `tools/diagnostics/diagnostic_env.go:RegisterFile()` — adds or updates a file in the environment. Assigns 1-based indices so zero-value Location (fileIndex=0) is unknown.
  - **Default namespace** (no active compile instance): same-name/same-doc no-ops; same-name/different-doc panics (legacy contract).
  - **Under an active compile instance**: a same-name registration under a different instance allocates a new file index (no panic). A same doc pointer reused across instances no-ops to the existing index (symbol-Location stability).
- `projects/package_compilation.go:compileModulesInternal()` — the actual call site:
  1. `de := c.compilerEnv.DiagnosticEnv()` — gets the shared DiagnosticEnv from the persistent CompilerEnvironment
  2. `inst := de.BeginCompile()` — mints a new compile instance
  3. `defer de.EndCompile(inst)` — ensures cleanup
  4. For each module's documents: `de.RegisterFile(docCtx.registrationKey(), docCtx.getTextDocument())` — registers source files with the shared DiagnosticEnv. The key includes a per-package prefix so same-basename files across packages don't collide.
  5. Runs Phase 1 (sequential: parse, symbol resolution, top-level type resolution) and Phase 2 (parallel: local node resolution, semantic analysis, CFG, desugar, BIR).
- **Key insight**: The DiagnosticEnv is shared and persistent per source root. `BeginCompile` namespaces the registration by compile instance so re-compiling the same root allocates new indices for changed files and no-ops for unchanged ones (by doc pointer), keeping symbol-Locations stable across generations. This is the production-ready pattern for the target's `scoped diagnostic environment handle`.
