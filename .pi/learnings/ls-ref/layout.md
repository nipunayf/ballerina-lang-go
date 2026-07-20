# Layout

- `ls/ls/` — the PoC itself: `server/`, `core/`, `protocol/`, `corpus/`
- `ls/ls/server/` — protocol-aware handlers, UTF-16 conversion, diagnostic publication
- `ls/ls/core/` — protocol-free core: `compile/`, `workspace/`, `event/`, `uri/`
- `ls/ls/protocol/` — LSP 3.18 JSON-RPC envelope, framing, generated types
- `ls/ls/corpus/` — golden-JSON transcript tests
- `ls/ls/core/query/` — semantic query service (protocol-free, syntax-tree walk)
- `ls/ls/core/compile/snapshot.go` — StableSnapshot/InProgressSnapshot/SnapshotStore (dual-snapshot engine)

## Key entry points

- `ls/ls/server/server.go` — Server struct, message dispatch, didOpen/didChange/didClose/didSave/didChangeWatchedFiles handlers
- `ls/ls/server/diagnostics.go` — convertDiagnostics (byte-offset → UTF-16 protocol.Diagnostic)
- `ls/ls/server/documents.go` — applyChanges (UTF-16 range → byte-offset full text)
- `ls/ls/core/compile/compile.go` — CompilationService.Compile (synchronous, context-ignoring)
- `ls/ls/core/workspace/workspace.go` — ProjectService (documents map, palFS overlay, project index, publish/reload)
- `ls/ls/core/workspace/index.go` — projectIndex (LRU cache, source-root memo, eviction)
- `ls/ls/core/workspace/palfs.go` — palFS (overlay-augmented io/fs.FS for open-buffer content)
- `ls/ls/core/event/event.go` — synchronous event bus (ProjectRegistered/Evicted/KindTransitioned)
- `ls/ls/core/uri/uri.go` — DocumentURI (file:/expr:/ai:/bala: scheme-typed identity)
- `ls/ls/protocol/framing.go` — ReadMessage/WriteMessage (Content-Length framing)
- `ls/ls/corpus/corpus_test.go` — transcript driver with $pal/writeFile interleaving
- `ls/ls/core/query/query.go` — query.Service (DocumentSymbols via syntax-tree walk)
- `ls/ls/server/symbols.go` — handleDocumentSymbol (protocol conversion, hierarchy/flat dispatch)
- `ls/ls/core/compile/snapshot.go` — SnapshotStore (bounded LRU, stale gate, in-progress tracking)
- `ls/ls/core/event/event.go` — tiered event bus (CRITICAL/COALESCEABLE/BEST_EFFORT)
