# Server dispatch and protocol handling

## Ticket-08 (legacy) server

- `lsp/server.go:dispatchRequest` — method switch, returns `(any, errorCode, errorMessage)`
- `lsp/server.go:handleNotification` — method switch for all LSP notifications
- `lsp/server.go:readMessage` — Content-Length header parsing
- `lsp/server.go:writeMessage` — Content-Length framing
- `lsp/server.go:handleWatchedFileChanges` — routes to `refreshChangedBuildFile` (changed .bal), `rebuildBuildProject` (create/delete .bal, toml changes), `scheduleOpenFileDiagnostics` (open buffers)
- `lsp/server.go:scheduleOpenFileDiagnostics` — skips rebuild for open files
- `lsp/server.go:refreshChangedBuildFile` — checks open status first
- `lsp/server.go:handleRenamedFiles`/`handleCreatedFiles`/`handleDeletedFiles` — rebuild project snapshot
- No `$/cancelRequest` handling exists anywhere in the PoC — `dispatchRequest`'s method switch has no cancellation case
- `lsp/log.go:logLS` — writes to `.bal/lsp.log` in project root, gated by `BAL_LSP_LOG` env var

## Ticket-09 server (current)

- `ls/ls/server/server.go:Server` — holds `requestRegistry` for in-flight non-lifecycle requests, `writeMu` for serialized framed writes, `ceDone` WaitGroup for CE→publish tasks.
- `ls/ls/server/server.go:handleRequest()` — routes to `handleLifecycleRequest` (initialize/shutdown, synchronous in Serve loop) or `handleTrackedRequest` (non-lifecycle, cancellable goroutine).
- `ls/ls/server/server.go:handleTrackedRequest()` — registers the request in the `requestRegistry`, spawns a tracked goroutine, and returns immediately. A new request reusing an in-flight id is rejected with Invalid Request (-32600). New work is gated out once the server is shutting down.
- `ls/ls/server/server.go:requestRegistry` — keys canonical string/integer JSON-RPC ids to `requestEntry` (cancel func, replyOnce, release channel). `register()` returns false if id already in flight. `unregister()` removes on completion. `cancelAll()` cancels every in-flight request's context (used by shutdown/exit).
- `ls/ls/server/server.go:handleCancelRequest()` — parses `$/cancelRequest` notification, looks up the request id in the registry, and cancels only that request's context. Does NOT call `CompilationService.Cancel` or supersede any source root. Unknown/malformed/completed/duplicate cancellations are no-ops.
- `ls/ls/server/server.go:write()` — serializes framed writes under `writeMu` so concurrent writes from the Serve goroutine and the CE-subscriber goroutine cannot interleave header/body bytes.
- `ls/ls/server/server.go:handleShutdown()` — calls `cancelAndDrainTrackedRequests()` (cancels all in-flight requests, waits for goroutines to finish), then shuts down workspace and compiler.
- `ls/ls/server/server.go:handleFlush()` — corpus-only `$pal/flush` sentinel: calls `s.Flush()` which waits for compile engine in-flight cycles, CE→publish tasks, and tiered bus delivery.

## CompilationService concurrency protection

- `ls/ls/core/compile/compile.go:CompilationService` — `cycleMu sync.Mutex` guards `inFlight` and `pending` maps. `workSem chan struct{}` bounds concurrent compiles across roots to `maxWorkers` (default: min(runtime.NumCPU(), 4)).
- `ls/ls/core/compile/compile.go:enqueueImmediately()` — single-flight-per-root: if a cycle is already in flight for root, the new request is parked in the depth-1 pending slot (latest-wins). When the in-flight cycle finishes, `finishCycle()` checks for a pending request and starts it.
- `ls/ls/core/compile/compile.go:runCycle()` — pre-compile stale check: reads current generation and compares against the request's generation. If stale, publishes CE-E3 and returns without compiling.
- `ls/ls/core/compile/compile.go:SnapshotStore.publishStable()` — stale-publication gate: if the snapshot's generation is no longer current, discards with CE-E3 and does not store.
- `ls/ls/core/compile/compile.go:Cancel()` — `$/cancelRequest` mapping (design branch 3). Go cannot interrupt a running compile, so cancellation is boundary-only: bumps the workspace generation of every root with an in-flight or depth-1 pending cycle, so the running compile finishes but its result is gated out at the stale-publication gate (CE-E3) and the pending cycle is dropped on dequeue.
- `ls/ls/core/compile/compile.go:SnapshotStore.lease()` — non-blocking, generation-matching completion lease. Increments per-root lease count so eviction defers final disposal until the lease is released. The returned release func is idempotent (sync.Once).
- `ls/ls/core/compile/compile.go:SnapshotStore.storeStable()` — LRU eviction with lease-aware deferral: an LRU victim with an active completion lease is not disposed immediately; it is removed from future acquisition (pendingEvict) and dropped from the LRU order, but its snapshot stays pinned until the held lease is released.

## Event bus concurrency

- `ls/ls/core/event/event.go:Bus` — synchronous inline dispatch for 08 subscribers; tiered async delivery for 09 subscribers.
- `ls/ls/core/event/event.go:SubscribeWithTier()` — registers a handler with a delivery tier (CRITICAL, COALESCEABLE, BEST_EFFORT). Each tiered subscriber gets a dedicated drainer goroutine.
- `ls/ls/core/event/event.go:TierCritical` — bounded buffered channel (256). On full, the event is dropped and logged. Never coalesced.
- `ls/ls/core/event/event.go:TierCoalesceable` — last-write-wins per `CoalesceKey{Kind, SourceRoot, DocumentURI}`. Wake signal via `signal` channel.
- `ls/ls/core/event/event.go:TierBestEffort` — bounded ring buffer (64), head-drop on full. Wake signal via `signal` channel.
- `ls/ls/core/event/event.go:Flush()` — blocks until every tiered subscriber's channel is empty AND its drainer goroutine has finished processing every queued event. The deterministic drain point for corpus tests.
- `ls/ls/core/event/event.go:Close()` — stops accepting new subscribers, drops further publishes, signals every tiered drainer goroutine to exit.

## Protocol types

- `lsp/protocol/types.go` — LSP 3.18 types: CompletionParams, CompletionList, CompletionItem, Position, Range, TextEdit, etc.
- `lsp/protocol/types.go:CompletionItemKindFunction=3, Variable=6, Class=7, Module=9, Keyword=14, Constant=21`
- `lsp/protocol/types.go:InsertTextFormatPlainText=1, InsertTextFormatSnippet=2`
- `lsp/protocol/types.go:CompletionOptions.TriggerCharacters` — `[":", ".", "{", "\n", " "]`
- `lsp/protocol/types.go:ServerCapabilities` — CompletionProvider, DefinitionProvider, ReferenceProvider, CodeActionProvider, DocumentSymbolProvider, WorkspaceSymbolProvider
- `lsp/protocol/types.go:TextDocumentSyncOptions.Change=1` — full content sync (not incremental)
- `lsp/protocol/types.go:WatchKindCreate=1, Change=2, Delete=4`
- `lsp/protocol/types.go:FileChangeTypeCreated=1, Changed=2, Deleted=3`
- `lsp/protocol/types.go:WorkDoneProgressBegin/End` — `$/progress` notification payloads
- `lsp/protocol/types.go:RegistrationParams` — dynamic capability registration
- `lsp/protocol/types.go:FileSystemWatcher` — `GlobPattern` + `WatchKind`
- `lsp/protocol/types.go:CodeAction` — Title, Kind, Diagnostics, Edit (WorkspaceEdit)
- `lsp/protocol/types.go:WorkspaceEdit.Changes` — `map[DocumentURI][]TextEdit`
- `lsp/protocol/types.go:ReferenceContext.IncludeDeclaration`
- `lsp/protocol/types.go:CodeActionContext.Diagnostics`
- `lsp/protocol/types.go:PublishDiagnosticsParams` — URI, Version (optional), Diagnostics
- `lsp/protocol/types.go:Diagnostic` — Range, Severity, Code, Source, Message
- `lsp/protocol/types.go:SymbolKind` — File=1 through TypeParameter=26
- `lsp/protocol/types.go:DocumentSymbol` — Name, Kind, Range, SelectionRange, Children
- `lsp/protocol/types.go:SymbolInformation` — Name, Kind, Location
- `lsp/protocol/types.go:CompletionItem` — Label, Kind, Detail, InsertText, InsertTextFormat, AdditionalTextEdits
- `lsp/protocol/types.go:CompletionList` — IsIncomplete, Items
- `lsp/protocol/types.go:TextEdit` — Range, NewText
- `lsp/protocol/types.go:Location` — URI, Range
- `lsp/protocol/types.go:Range/Position` — Range (Start, End); Position (Line, Character)
