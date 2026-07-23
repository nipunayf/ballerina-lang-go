# Snapshot management

## Ticket-08 (legacy) snapshot model

- `lsp/snapshot.go:SnapshotManager` — `Current()`/`Publish()`/`IsCurrent()`; no locks, simple pointer swap (lines 104-126). Snapshots reused eagerly: updateSnapshot calls nextBuildSnapshot synchronously on every didChange (server.go:303-306), no separate background compile of snapshot metadata; diagnostics compile is background but post-hoc (not integrated with request handling)
- `lsp/snapshot.go:reuseModuleState` — carries forward CompilationUnits, Stage, Imports, Package, CFG across snapshot generations
- `lsp/snapshot.go:newSingleFileSnapshot` — single default module
- `lsp/snapshot.go:scanBuildProject` — walks `modules/` dir, creates per-module entries with `PackageID`
- `lsp/snapshot.go:nextSnapshotID` — wraps to `initialSnapshotID` after `maxIncrementalSnapshotID` (100000)
- `lsp/snapshot.go:newBuildSnapshot` — when `id == initialSnapshotID`, creates fresh `CompilerEnvironment`
- `lsp/snapshot.go:newCompilerEnvironment` — `context.NewCompilerEnvironment(semtypes.CreateTypeEnv(), false)` — creates a fresh env with its own `DiagnosticEnv`, `SymbolSpace` list, `anonTypeCount`, `anonFuncCount`, `packageInterner`, `distinctTypes` tracker. This is the shared mutable state that lives across requests.
- `lsp/snapshot.go:newBuildSnapshot` — when `old != nil && old.Env != nil`, reuses `old.Env` directly (line ~175). The `CompilerEnvironment` is shared across snapshot generations until the snapshot ID wraps around to `initialSnapshotID`.
- `lsp/snapshot.go:openSnapshotFiles` — carries forward only `file.Open == true` files
- `lsp/snapshot.go:scanModuleFiles` — reads `.bal` files from disk, overlays open files
- `lsp/snapshot.go:readPackageDescriptor` — parses `Ballerina.toml` for org/name/version
- `lsp/snapshot.go:uriFromPath` — `file:` scheme
- `lsp/snapshot.go:normalizePath` — `filepath.Abs` + `filepath.Clean`
- `lsp/server.go:pathFromURI` — URL-decoded file path
- `lsp/server.go:isUnder` — normalized prefix check with separator
- `lsp/server.go:invalidateChangedDependents` — resets module state for changed module + all transitive dependents in topo order
- `lsp/server.go:updateSnapshot` — ignores documents not matching `singleFileURI`
- `lsp/snapshot_test.go:writeBuildProjectFiles` — writes Ballerina.toml + main.bal
- `lsp/snapshot_test.go:TestDidSaveDoesNotCreateSnapshotWhenContentUnchanged`
- `lsp/snapshot_test.go:TestBuildSnapshotCanRefreshOpenFileContent`
- `lsp/snapshot_test.go:TestBuildSnapshotResetsGenerationAndCompilerEnvironment`
- `lsp/snapshot_test.go:TestProjectModeUsesInitializedRootAsSnapshotKey`
- `lsp/snapshot_test.go:TestSingleFileModeMaintainsOneSnapshot`

## Ticket-09 dual-snapshot model (current)

- `ls/ls/core/compile/snapshot.go:SnapshotStore` — bounded dual-snapshot repository (ADR-058). Stable snapshots retained by count (latest per source root, plus global LRU bound); one in-progress slot per source root.
- `ls/ls/core/compile/snapshot.go:StableSnapshot` — frozen, fully-materialised result of the last successful accepted cycle. Its Package and diagnostics live on the persistent per-source-root CompilerEnvironment, so its symbols and file indices stay valid for the snapshot's life.
- `ls/ls/core/compile/snapshot.go:InProgressSnapshot` — current generation's pending/running compilation, with a `done` channel.
- `ls/ls/core/compile/snapshot.go:SnapshotStore.lease()` — non-blocking, generation-matching completion lease. Returns the stable snapshot for root only when one exists for exactly the current document generation AND the root has not been marked for eviction. Increments per-root lease count so eviction defers final disposal until the lease is released. Never waits, compiles, or schedules compilation.
- `ls/ls/core/compile/snapshot.go:SnapshotStore.publishStable()` — stale gate: if the snapshot's generation is no longer current, discards with CE-E3 and does not store. Otherwise stores and publishes CE-E1/E5a/E5b.
- `ls/ls/core/compile/snapshot.go:SnapshotStore.storeStable()` — records the snapshot, evicting LRU stable root when bound exceeded. An LRU victim with an active completion lease is not disposed immediately: removed from future acquisition (pendingEvict) and dropped from LRU order, but snapshot stays pinned until the held lease is released.
- `ls/ls/core/compile/snapshot.go:SnapshotStore.evictRoot()` — drops stable and in-progress state for root. If a completion lease is currently held, eviction is deferred until the last lease is released.
- `ls/ls/core/compile/snapshot.go:SnapshotStore.evictAll()` — drops every stable and in-progress snapshot (shutdown).

## CompilerEnvironment and CompilerContext (shared vs per-request)

- `context/env.go:63-80` — `CompilerEnvironment` struct: holds `symbolSpaces` (slice, append-only), `typeEnv`, `underlyingSymbol` (sync.Map), `distinctTypes` tracker, `diagnosticContext`. This is the long-lived shared state, not the per-request context.
- `context/context.go:61-70` — `CompilerContext` struct: holds `env *CompilerEnvironment`, `mu sync.Mutex`, `diagnostics []diagnostic`, `moduleStats`, `stage`. This is the cheap per-request wrapper — `NewCompilerContext(env)` just sets the `env` pointer, no allocation of shared state.
- `context/context.go:200-204` — `NewCompilerContext(env)` — trivial struct literal, no heap allocation of shared resources. The `CompilerContext` is a lightweight handle that delegates all symbol/scope/type operations to the `CompilerEnvironment`.
- **Key insight**: The `CompilerEnvironment` is the heavy shared state (symbol spaces, type env, diagnostic env). The `CompilerContext` is a trivial per-request wrapper that costs one struct allocation. Every request handler creates its own `CompilerContext` from the snapshot's `Env`. This is the pattern to replicate for the target's `scoped environment handle`.

## CompilerEnvironment lifecycle (ticket-09)

- `projects/project_environment_builder.go:Build()` — creates `context.NewCompilerEnvironment(semtypes.CreateTypeEnv(), buildOptions.Stats())` once per source root. This is the persistent per-source-root CompilerEnvironment.
- `projects/env.go:Environment` — wraps `*context.CompilerEnvironment` plus `PackageCache`, `PackageResolver`, `publicSymbols`. Created once per source root, shared across all generations.
- `projects/base_project.go:BaseProject` — holds `*Environment` (shared). `initBaseWithEnv` sets it; `initBase` creates a fresh one via `NewProjectEnvironmentBuilder`.
- `projects/package_compilation.go:newPackageCompilation()` — reads `rootPkgCtx.project.Environment().compilerEnvironment()` to get the shared `*context.CompilerEnvironment`. This is the same env across all compilations of the same source root.
- `ls/ls/core/workspace/workspace.go:ProjectService.publish()` — first publish for a source root does one `projects.Load` to seed the persistent per-source-root CompilerEnvironment and initial package. Subsequent content publishes reuse the SAME project via `Document.Modify().WithContent().Apply()` so the shared CompilerEnvironment/DiagnosticEnv persists across generations (symbol-Location stability).
- `ls/ls/core/workspace/workspace.go:applyModifierChain()` — updates document content through the immutable modifier chain on the persistent project, which sets a new current package on the same project (and thus the same CompilerEnvironment).

## Completion-specific snapshot pattern (ticket-09, current)

- **Completion does NOT construct a fresh CompilerContext or Environment.** It uses a non-blocking lease on a pre-compiled stable snapshot's completion index.
- `ls/ls/core/query/completion.go:completeFunctionBody()` — calls `s.compiler.Lease(root, gen)` to acquire a generation-matched lease. The lease returns copied, protocol-free completion facts (CompletionIndex, ExpectedTypeIndex, ImportCatalog, MemberCompletionIndex, InvocationCompletionIndex). No CompilerContext is created.
- `ls/ls/core/query/completion.go:CompletionLeaser` interface — the query layer's compile-free lease provider. Implemented by `CompilationService.Lease()`.
- `ls/ls/server/completion.go:completionLeaseAdapter` — adapts `compile.CompletionLease` to `query.CompletionLease` at the server boundary, keeping `ls/core/query` free of compile imports.
- `ls/ls/server/completion.go:handleCompletion()` — calls `s.query.Completion(documentURI, byteOffset, ctx)` which internally acquires the lease. No snapshot cloning, no recompilation.
- `ls/ls/core/compile/compile.go:CompilationService.Lease()` — non-blocking, never waits, parses, compiles, or schedules compilation. Returns a pinned CompletionLease with copied index facts.
- `ls/ls/core/compile/snapshot.go:SnapshotStore.lease()` — the actual lease implementation: checks `pendingEvict`, checks stable snapshot exists for exact generation, increments lease count, returns release func.
- **Production-ready pattern**: The lease-based approach is the correct production design. Completion never blocks on compilation, never creates a CompilerContext, and reads only copied, protocol-free facts from a generation-matched snapshot.

## Legacy completion-specific snapshot pattern (ticket-08, deprecated)

- `lsp/completion.go:1119-1156` — `snapshotWithRecoveredCU(snapshot, module, uri, recoveredCU)` — creates a **shallow copy** of the entire snapshot for completion. Copies all modules (struct copy), deep-copies only the `CompilationUnits` map, then resets the target module's `Stage` to `FrontendStageNone` and clears `Imports`, `ImportsResolved`, `ImportedByCU`, `ImportedSymbols`, `Package`, `Exported`, `CFG`. This is a prototype shortcut — copying the entire snapshot per request is wasteful.
- The completion snapshot is ephemeral — created per request, used only for the duration of the completion handler, then discarded. It is never published back to the `SnapshotManager`.
- Dependency modules in the completion snapshot retain their `Stage` from the original snapshot (via `reuseModuleState`), so `runModuleFrontend` skips their already-completed stages. Only the target module is recompiled.
- **Key insight for the target**: The PoC's approach of "clone snapshot, mutate module, compile, discard" is a prototype shortcut. A production design should either (a) mutate the module in-place with a rollback mechanism, or (b) use a lighter-weight overlay that doesn't copy the entire module map.
