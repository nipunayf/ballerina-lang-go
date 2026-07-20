# Prototype shortcuts (don't copy as-is)

- **Synchronous Compile**: `compile.go:99-148` — no goroutines, no async scheduling, context ignored. Must add async compile with proper cancellation.
- **No refcounting on Snapshot**: `workspace.go:58-63` — plain struct. Needs refcount + release func for dual-snapshot engine.
- **Full re-Load per publication**: `workspace.go:218-226` — `reloadAt()` does `projects.Load` fresh per content change. Should use modifier chain (Document.Modify().WithContent().Apply() → setCurrentPackage).
- **No stale-result suppression**: No version tracking in CompileResult. Needs to track which version a compile result corresponds to and discard if superseded.
- **No async delivery on event bus**: `event.go:97-115` — synchronous dispatch. Needs async delivery for CE lifecycle events.
- **Shutdown is a no-op**: `workspace.go:310-315` — no cancellation. Needs real cancellation of in-flight compiles.
- **No compile cancellation**: `compile.go:101` — `_ = ctx`. Must check ctx.Err() and cancel in-flight compiles on shutdown or newer version.
- **No ProjectUpdated event**: `event.go:22-24` — only three event types exist. Needs ProjectUpdated (WM-E4).
- **Query returns nil on any error**: `query.go:62-80` — `DocumentSymbols()` returns nil on nil service, nil project, missing document, nil syntax tree, wrong root node type. No error reporting.
- **No query result caching**: `query.go:62-80` — full syntax-tree re-walk on every `DocumentSymbols()` call. No incremental update.
- **Only syntax-tree queries work**: `query.go:62-80` — no type-resolution queries (definition, references, hover) are implemented. Those would need the compile service's snapshot.
- **No per-document cancellation**: `server/server.go:handleCancelRequest` — `$/cancelRequest` applies to all active roots (no per-document id→root mapping in 09).
