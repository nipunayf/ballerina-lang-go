# Migration / test-driver constraints

- **txtar-style fixture loader:** ADR-015 §6 — BallerinaTxtarLoader for multi-file project fixtures as text archives. Mirrors gopls/rust-analyzer pattern. `architecture/adrs/ADR-015-observability-testability.md:§6`
- **Injectable WorkspaceCompiler interface:** ADR-015 §3 — extract WorkspaceCompiler from BallerinaWorkspaceManager. MockWorkspaceCompiler returns pre-baked stubs. Mirrors pyright ImportResolver pattern. `architecture/adrs/ADR-015-observability-testability.md:§3`
- **Injectable FileContentProvider:** ADR-015 §4 — all file reads through injectable interface. InMemoryFileSystem for tests. `architecture/adrs/ADR-015-observability-testability.md:§4`
- **Injectable FileWatcherService:** ADR-015 §5 — SyntheticFileWatcher for deterministic event sequencing. Mirrors gopls fake.Workdir. `architecture/adrs/ADR-015-observability-testability.md:§5`
- **DiagnosticAwaiter pattern:** ADR-015 §7 — CompletableFuture-based async diagnostic assertions. Mirrors gopls Awaiter.OnceMet(). `architecture/adrs/ADR-015-observability-testability.md:§7`
- **TelemetryEmitter interface:** ADR-015 §1 — constructor-injected, no-op for tests. `architecture/adrs/ADR-015-observability-testability.md:§1`
- **Three-phase DocumentUri migration:** ADR-034 §Migration Plan — Phase 0 (detect deprecated proxy.get()), Phase 1 (fix routing), Phase 2 (DocumentUri type), Phase 3 (full cleanup). Superseded by ADR-040/059. `architecture/adrs/ADR-034-document-uri-scheme-preservation.md:Migration Plan`
- **Facade compatibility boundary:** ADR-059 — WorkspaceManagerFacadeImpl is compatibility adapter over frozen Path API. May do mechanical Path/URI/DocumentUri conversion. Must not own long-lived domain state. `architecture/adrs/ADR-059-fixed-path-facade-compatibility-boundary.md`
- **CE integration test fixtures must use DS-E2:** ADR-033 — recovery trigger changed from implicit "recovery on state change" to explicit DS-E2. All CE-integration test fixtures must be updated. `architecture/adrs/ADR-033-recovery-loop-prevention.md:Consequences`
- **Phase 6 heap acceptance tests:** ADR-057/ADR-058 — count-based bounds are not proof of heap weight for unusually large projects. Must add explicit heap acceptance tests. `architecture/adrs/ADR-057-uri-resolver-project-index.md:Consequences`
- **Gherkin scenarios for cross-context boundary:** cross-context-boundary.feature — compiler SDK loading behind BallerinaCompilerApi, owned-object navigation allowed inside WM/CE. `architecture/scenarios/cross-context-boundary.feature`
- **Gherkin scenarios for thread safety:** thread-safety.feature — 50 concurrent requests (hover, completion, didChange, didOpen, didClose) produce zero crashes. `architecture/scenarios/thread-safety.feature`
- **Gherkin scenarios for async compilation:** async-compilation-pipeline.feature — compilation on background worker, not request thread. `architecture/scenarios/async-compilation-pipeline.feature`
