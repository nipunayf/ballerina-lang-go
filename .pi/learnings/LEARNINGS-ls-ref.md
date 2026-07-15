# LEARNINGS-ls-ref

Durable index of what past exploration runs learned about the Go LS proof-of-concept (ls-ref/lsp). Read before searching; update before finishing. Keep entries summarized and pointer-dense — `path` + symbol, one line each.

## Layout

- `ls/ls/` — the PoC itself: `server/`, `core/`, `protocol/`, `corpus/`
- `ls/ls/server/` — protocol-aware handlers, UTF-16 conversion, diagnostic publication
- `ls/ls/core/` — protocol-free core: `compile/`, `workspace/`, `event/`, `uri/`
- `ls/ls/protocol/` — LSP 3.18 JSON-RPC envelope, framing, generated types
- `ls/ls/corpus/` — golden-JSON transcript tests

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

## Confirmed facts

- **Core-service seam**: `ls/server` handles UTF-16↔byte-offset conversion; `ls/core` is protocol-free. `server/diagnostics.go:7-9` and `server/documents.go:7-9` both document this boundary explicitly.
- **Snapshot is plain struct**: `workspace.Snapshot` (`workspace.go:58-63`) has Text/Version/LanguageID/SourceRoot, no refcount. Comment says "Ticket 09 wraps Snapshot with a release func() when the dual-snapshot engine adds refcounting."
- **Compile is synchronous**: `compile.go:99-148` — `Compile()` is a blocking call. `context.Context` is accepted but ignored (`_ = ctx` at line 101). Comment says "context.Context is threaded for ADR-018 cancellation ownership, even though Phase B's calls are synchronous."
- **Full re-Load per content change**: `workspace.go:218-226` — `publish()` calls `reloadAt()` which does `projects.Load` fresh. Comment at `workspace.go:196-203` documents this as a Phase-B limitation: "the compiler's DiagnosticEnv is a per-Load singleton that panics on re-registration of a changed file."
- **Version monotonicity enforced**: `workspace.go:130-133` — `applyDocumentMap` rejects `change.Version <= current.Version` with error.
- **Event bus is synchronous**: `event.go:97-115` — `Publish` dispatches inline on caller's goroutine. Comment at `event.go:22-24` says "Ticket 09 extends the same bus with ProjectUpdated (WM-E4), compilation-engine (CE) lifecycle events, and async delivery."
- **Shutdown is a no-op**: `workspace.go:310-315` — `Shutdown()` only calls `bus.Close()`. Comment says "Shutdown is the lifecycle contract that ticket 09 fills with real cancellation."
- **Diagnostic version support**: `server.go:68-72` — server checks `capabilities.PublishDiagnostics.VersionSupport` at initialize time.
- **palFS overlay**: `workspace/palfs.go` — overlay-augmented fs.FS carries open-buffer content for ReadFile/Stat; ReadDir delegates to PAL.
- **LRU eviction**: `workspace/index.go:80-103` — evicts background (openDocs==0) entries first, then LRU active entries.
- **Watched-file routing**: `server.go:168-196` — `handleDidChangeWatchedFiles` routes file: events to `ProjectService.ApplyWatchedFile`; non-file: events ignored.
- **$pal/writeFile sentinel**: `corpus/corpus_test.go:103-108` — transcript driver supports `$pal/writeFile` method for mid-session on-disk writes through PAL only.
- **Diagnostic conversion**: `server/diagnostics.go:14-37` — `convertDiagnostics` maps `compile.CompilerDiagnostic` (byte-offset positions) to `protocol.Diagnostic` (UTF-16 positions). Core never imports `ls/protocol`.
- **UTF-16 boundary helpers**: `server/documents.go:18-82` — `applyChanges`, `utf16Offset`, `nextLineStart`, `utf16Length` resolve protocol.TextEdit ranges to byte offsets before calling workspace.Apply.

## Prototype shortcuts (don't copy as-is)

- **Synchronous Compile**: `compile.go:99-148` — no goroutines, no async scheduling, context ignored. Ticket 09 must add async compile with proper cancellation.
- **No refcounting on Snapshot**: `workspace.go:58-63` — plain struct. Ticket 09 needs refcount + release func for dual-snapshot engine.
- **Full re-Load per publication**: `workspace.go:218-226` — `reloadAt()` does `projects.Load` fresh per content change. Ticket 09 should use ADR-042 modifier chain (Document.Modify().WithContent().Apply() → setCurrentPackage).
- **No stale-result suppression**: No version tracking in CompileResult. Ticket 09 needs to track which version a compile result corresponds to and discard if superseded.
- **No async delivery on event bus**: `event.go:97-115` — synchronous dispatch. Ticket 09 needs async delivery for CE lifecycle events.
- **Shutdown is a no-op**: `workspace.go:310-315` — no cancellation. Ticket 09 needs real cancellation of in-flight compiles.
- **No compile cancellation**: `compile.go:101` — `_ = ctx`. Ticket 09 must check ctx.Err() and cancel in-flight compiles on shutdown or newer version.
- **No ProjectUpdated event**: `event.go:22-24` — only three event types exist. Ticket 09 adds ProjectUpdated (WM-E4).

## Dead ends

- (none yet)
