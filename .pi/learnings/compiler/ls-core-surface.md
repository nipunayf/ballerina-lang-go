# LS-core surface built on the compiler (`ls/core`, `ls/server`)

Facts about how the LS under `ballerina-lang-go/ls/ls/` consumes the compiler.
Paths here are relative to `ballerina-lang-go/ls/`. This code evolves per ticket
— verify pointers before reusing.

## Snapshot types (ls/core/compile)

- `CompilationKey` — `SourceRoot string`, `Generation uint64` — `ls/core/compile/snapshot.go:20-25`
- `StableSnapshot` — `key`, `project projects.Project`, `pkg *projects.Package`, `byFile map[string][]CompilerDiagnostic`, `resByFile`, `resolutionErrored bool` — `ls/core/compile/snapshot.go:30-40`; accessors `Package()`, `Project()`, `Diagnostics()`, `Key()` — `snapshot.go:40-60`
- `InProgressSnapshot` — `key`, `done <-chan struct{}`, `result func() (StableSnapshot, error)` — `ls/core/compile/snapshot.go:60-70`
- `SnapshotStore` — bounded dual-snapshot repository: `stable` + `inProgress` maps, LRU eviction; `Stable(root)`, `InProgress(root)` — `ls/core/compile/snapshot.go:80-125`
- Stale-publication gate (CE-E3) in `SnapshotStore.publishStable` — discards snapshots whose `key.Generation != current`.

## CompilationService (ls/core/compile)

- `Compile(ctx, req)` — synchronous fast path: reads `SnapshotStore.Stable` first (cache hit), falls back to inline compile — `ls/core/compile/compile.go:200-230`. Accepts `ctx` but ignores it (`_ = ctx`).
- `DiagnosticsFor(root)` — per-open-document diagnostics from stable snapshot — `compile.go:180-200`
- `Flush()` — blocks until in-flight compiles finish and CE events drain — `compile.go:250-270`
- `Shutdown()` — stops accepting, bounded-waits for in-flight, flushes bus, clears store — `compile.go:280-300`
- `Cancel()` — bumps generation of every active root (boundary-only cancellation, all roots) — `compile.go:310-330` / `compile.go:455`
- `runCycle` — pre-compile stale check: `if req.gen != current { publish CompilationCancelled; return }` — `compile.go:runCycle`
- `CompileRequest{URI}` / `CompileResult{Diagnostics}` / `CompilerDiagnostic{StartLine, StartChar, EndLine, EndChar, Severity, Code, Message}` — `compile.go:35-70`
- `realCompilePackage(pkg)` — runs `pkg.Compilation()`, extracts diagnostics by file — `compile.go:280-290`
- Subscribes to WM events to maintain `knownRoots` set (guarded by `sync.Mutex`) — `compile.go:50-80`

## ProjectService (ls/core/workspace)

- `Apply(ctx, change)` — applies document change, publishes project — `ls/core/workspace/workspace.go:200-280`
- `CurrentProject(root)` → `(Project, generation, ok)` — `workspace.go:350-360`
- `Generation(root)` → per-root monotonic counter — `workspace.go:330-340`
- `Supersede(root)` — bumps generation without publishing (cancel path; `index.supersede` does `entry.generation++`) — `workspace.go:340-350` / `workspace.go:372`
- `Project(uri)` / `Snapshot(uri)` / `OpenDocumentsUnder(root)` — `workspace.go:500-530`, `370-390`
- `DocumentChange{Kind, URI, Text, Version, LanguageID}`; `Snapshot{Text, Version, LanguageID, SourceRoot, Generation}` — `workspace.go:60-95`
- `publish()` — ADR-042 modifier-chain publication: first publish does `projects.Load` to seed; subsequent publishes use `Document.Modify().WithContent().Apply()` — `workspace.go:280-300` (earlier Phase B behavior was full `reloadAt()` re-Load per change — superseded by ticket 09's per-instance DiagnosticEnv work)
- `applyModifierChain()` — updates document through immutable modifier chain on persistent project — `workspace.go:300-320`
- `reloadAt()` — builds fresh palFS, loads brand-new project (fresh CompilerEnvironment), replaces index entry — `workspace.go:320-340`
- `projectIndex` — count-bounded, source-root-keyed LRU; `pathToSourceRoot` memo avoids repeated ADR-048 root walks; eviction publishes `ProjectEvicted` — `ls/core/workspace/index.go:30-110`

## Event bus (ls/core/event)

- `event.Kind` — `ProjectRegistered`, `ProjectEvicted`, `ProjectKindTransitioned`, `ProjectUpdated`, `CompilationStarted`, `CompilationSucceeded`, `CompilationFailed`, `CompilationCancelled`, `ResolutionDiagnosticsReady`, `CompilationDiagnosticsReady` — `ls/core/event/event.go:30-55`
- `event.Tier` — `TierCritical`, `TierCoalesceable`, `TierBestEffort` — `event.go:70-80`
- `event.Event` interface — `Kind()`, `SourceRoot()`, `Generation()` — `event.go:60-65`
- `event.Bus` — synchronous dispatch: `Publish(e)`, `Subscribe(kinds, handler)`, `SubscribeWithTier(kinds, tier, handler)`, `Flush()` — `event.go:100-150`
- CE events carry `SourceRoot` + `Generation` (+ descriptor for Succeeded/DiagnosticsReady) — `event.go:200-300`

## Query service (ls/core/query)

- `query.Service` — wraps `*workspace.ProjectService`; currently only exposes `DocumentSymbols(uri)` — `ls/ls/core/query/query.go:60-70`
- `DocumentSymbols` walks the red-node syntax tree (`tree.ModulePart.Members()`) and classifies top-level nodes by type — `ls/ls/core/query/query.go:70-100`
- No completion, hover, definition, or references methods exist yet.
- `query.SymbolKind` — core-defined enum (Function, Method, Constructor, Class, Object, Struct, Interface, TypeParameter, Variable, Constant, Enum, EnumMember, Namespace, Property, Field) — `ls/ls/core/query/query.go:10-30`
- `query.DocumentSymbol` — `Name`, `Kind`, `Range`, `Deprecated`, `Children` — `ls/ls/core/query/query.go:35-45`
- `query.ByteRange` — `StartLine/StartChar/EndLine/EndChar` (byte offsets, not UTF-16) — `ls/ls/core/query/query.go:30-35`
- The query service is protocol-free; the server (`ls/server/symbols.go`) converts to LSP types.

## Query service (ls/core/query)

- `query.Service` — wraps `*workspace.ProjectService`; exposes `DocumentSymbols(uri)` and `Completion(uri, byteOffset, ctx)`. `ls/ls/core/query/query.go:60-70`, `ls/ls/core/query/completion.go:100-130`
- `Completion` is the protocol-free entry point: classifies cursor context via `classifyContext()` (red-node tree only), dispatches to `completeFunctionBody`, `completeModulePart`, `completeImport`, `completeAliasMember`. `ls/ls/core/query/completion.go:100-130`
- `classifyContext(part, offset, text)` — classifies cursor as `contextFunctionBody`, `contextModulePart`, `contextImport`, `contextAliasMember`, or `contextUnsupported`. Walks red-node tree only; no compiler objects. `ls/ls/core/query/completion_module.go:100-140`
- `classifyCompletion(part, offset, text)` — for function-body positions, extracts parameters and preceding locals from red-node tree. `ls/ls/core/query/completion.go:200-240`
- `classifyAliasMember(part, offset, text)` — detects `alias.<prefix>` access. `ls/ls/core/query/completion_module.go:200-230`
- `classifyImport(part, offset, text)` — classifies import sub-position (org, module, `as` alias). `ls/ls/core/query/completion_module.go:150-190`
- `CompletionResult` — `Items []CompletionItem`, `PrefixStart int`, `PrefixEnd int`. `ls/ls/core/query/completion.go:70-80`
- `CompletionItem` — `Label`, `Kind`, `Detail`, `InsertText`, `Snippet`, `Rank`, `AdditionalEdits`. `ls/ls/core/query/completion.go:55-65`
- `CompletionItemKind` — `CompletionKindKeyword`, `CompletionKindVariable`, `CompletionKindConstant`, `CompletionKindFunction`, `CompletionKindType`, `CompletionKindModule`. `ls/ls/core/query/completion.go:35-45`
- `CompletionLease` / `CompletionLeaser` — non-blocking, generation-matched lease for `CompletionIndex`, `ExpectedTypeIndex`, `ImportCatalog`. `ls/ls/core/query/completion.go:20-30`
- `SetCompletionCompiler(compiler)` — wires the non-blocking lease provider. `ls/ls/core/query/completion.go:380-385`
- `completionKeywords` — fixed keyword list: `if`, `while`, `foreach`, `return`, `check`, `checkpanic`, `panic`, `fail`, `new`, `var`. `ls/ls/core/query/completion.go:85-90`
- Completion ranks: `rankExpectedMatch=-1`, `rankParameter=0`, `rankLocal=1`, `rankKeyword=2`, `rankModuleVar=3`, `rankConstant=4`, `rankFunction=5`, `rankType=6`, `rankSnippet=0`, `rankImportModule=1`. `ls/ls/core/query/completion.go:90-100`
- `boostExpectedCompatible(items, fact)` — lowers rank of items precomputed compatible with expected type. `ls/ls/core/query/completion.go:170-185`
- `semanticItem(fact)` — converts `projects.CompletionFact` to `CompletionItem`. `ls/ls/core/query/completion.go:190-205`
- `modulePartCompletions` — fixed snippet matrix: `function`, `type`, `const`, `class`, `enum`, `listener`, `final`, `public`, `private`, `import`, `main`. `ls/ls/core/query/completion_module.go:30-50`
- `importModuleItems`, `importOrgItems`, `aliasMemberItems`, `autoImportItems` — catalog-backed completion builders. `ls/ls/core/query/completion_module.go:250-380`
- `buildImportEdit` — constructs additional edit for auto-import. `ls/ls/core/query/completion_module.go:230-250`
- `collectPrecedingLocals` — walks statement sequence collecting local var declarations before cursor, recursing into cursor-containing blocks only. `ls/ls/core/query/completion.go:240-290`
- `extractParameters` — extracts named parameters from function signature. `ls/ls/core/query/completion.go:220-235`
- `cursorInBody`, `cursorInComment`, `rangeContains`, `identifierPrefixStart` — utility helpers. `ls/ls/core/query/completion.go:295-360`
- `filterDedupSort` — deduplicates by label, sorts by rank then label. `ls/ls/core/query/completion.go:365-380`

## Server dispatch (ls/server)

- `handleMessage` routes by `len(message.ID) == 0` → notification vs request; `handleRequest` is synchronous dispatch → write response, no goroutines, no context propagation — `ls/ls/server/server.go:129-148`
- `dispatchRequest` handles `initialize`, `shutdown`, `textDocument/documentSymbol`; notifications: `didOpen`, `didChange`, `didClose`, `didSave`, `didChangeWatchedFiles`, `$/cancelRequest`, `$pal/flush` — `ls/ls/server/server.go:148`, `200-220`
- `$/cancelRequest` handled as notification; `CancelParams.ID` intentionally ignored — `handleCancelRequest` calls `s.compiler.Cancel()` (all roots) — `ls/ls/server/server.go:255-265`
- **No request-tracking infrastructure** — no `pendingRequests`, no requestID→sourceRoot mapping, no per-request `context.CancelFunc`.
- No `textDocument/completion` handler yet. Protocol types exist: `CompletionParams`, `CompletionList`, `CompletionItem`, `CompletionTriggerKind` — `ls/protocol/types_generated.go:295-4100`
- Tests: `TestCancelRequestRoutesAsNotification` (routing boundary only) — `ls/ls/server/diagnostics_test.go:349`; `TestEngineCancelSupersedesInFlight` (CE-E3 gating) — `ls/ls/core/compile/engine_test.go:317`; corpus tests use `$pal/flush` sentinel for deterministic drain. No per-request cancellation tests.

## What per-request cancellation (ticket 16) would require

1. Server: `pendingRequests map[string]requestInfo` (requestID → {sourceRoot, cancelFunc}); goroutine-per-cancellable-handler; `handleCancelRequest` parses `CancelParams.ID` and targets the request.
2. CompilationService: `Compile(ctx, req)` must actually honor `ctx`; `Cancel()` needs a root parameter.
3. Tests: per-request cancellation, concurrent cancellation, cancel-then-new-request.
