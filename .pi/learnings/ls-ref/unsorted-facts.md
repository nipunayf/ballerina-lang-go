# Unsorted facts — pending reconciliation

This file is a holding pen, not a topic file. It was carried over verbatim from
the old flat `LEARNINGS-ls-ref.md`'s "Confirmed facts (quick reference)"
section during the INDEX.md migration (2026-07-20).

Many of these bullets cite paths (`server/diagnostics.go`, `server/documents.go`,
`workspace.go`, `event.go`, `query.go`, `core/query/query.go`, `server/symbols.go`)
that do not exist in the ls-ref PoC checkout (`ballerina-lang-go/ls-ref`, flat
`lsp/` package — see `layout.md`). They match paths in THIS repo
(`ballerina-lang-go/ls`, `ls/core/...`, `ls/server/...`) instead. It's unclear
whether a past run confused the two checkouts, or whether these facts belong in
a different (currently nonexistent) learnings dir for this repo itself.

Do not treat these as verified ls-ref facts until reconciled — verify the path
against the actual checkout (`cwd` in `.pi/agents/explore-ls-ref.md`) before
reusing any of them. Once reconciled, move each bullet to its correct topic
file (in this dir, or elsewhere) and delete it from here.

## Confirmed facts (quick reference), as originally written

- **Core-service seam**: `ls/server` handles UTF-16↔byte-offset conversion; `ls/core` is protocol-free. `server/diagnostics.go:7-9` and `server/documents.go:7-9` both document this boundary explicitly.
- **Snapshot is plain struct**: `workspace.Snapshot` (`workspace.go:58-63`) has Text/Version/LanguageID/SourceRoot, no refcount. Comment says "Ticket 09 wraps Snapshot with a release func() when the dual-snapshot engine adds refcounting."
- **Compile is synchronous**: `compile.go:99-148` — `Compile()` is a blocking call. `context.Context` is accepted but ignored (`_ = ctx` at line 101).
- **Full re-Load per content change**: `workspace.go:218-226` — `publish()` calls `reloadAt()` which does `projects.Load` fresh.
- **Version monotonicity enforced**: `workspace.go:130-133` — `applyDocumentMap` rejects `change.Version <= current.Version` with error.
- **Event bus is synchronous**: `event.go:97-115` — `Publish` dispatches inline on caller's goroutine.
- **Shutdown is a no-op**: `workspace.go:310-315` — `Shutdown()` only calls `bus.Close()`.
- **palFS overlay**: `workspace/palfs.go` — overlay-augmented fs.FS carries open-buffer content for ReadFile/Stat; ReadDir delegates to PAL.
- **LRU eviction**: `workspace/index.go:80-103` — evicts background (openDocs==0) entries first, then LRU active entries.
- **Watched-file routing**: `server.go:168-196` — `handleDidChangeWatchedFiles` routes file: events to `ProjectService.ApplyWatchedFile`; non-file: events ignored.
- **$pal/writeFile sentinel**: `corpus/corpus_test.go:103-108` — transcript driver supports `$pal/writeFile` method for mid-session on-disk writes through PAL only.
- **Diagnostic conversion**: `server/diagnostics.go:14-37` — `convertDiagnostics` maps `compile.CompilerDiagnostic` (byte-offset positions) to `protocol.Diagnostic` (UTF-16 positions). Core never imports `ls/protocol`.
- **UTF-16 boundary helpers**: `server/documents.go:18-82` — `applyChanges`, `utf16Offset`, `nextLineStart`, `utf16Length` resolve protocol.TextEdit ranges to byte offsets before calling workspace.Apply.
- **Query service pattern**: `core/query/query.go:1-200` — `query.Service` takes `*workspace.ProjectService`, exposes `DocumentSymbols(uri.DocumentURI) []DocumentSymbol`. Protocol-free; returns core types (ByteRange, SymbolKind).
- **Query walks syntax tree directly**: `query.go:62-80` — `DocumentSymbols()` navigates `project.CurrentPackage().Module().Document().SyntaxTree().RootNode.(*tree.ModulePart).Members()`. No type/symbol resolution needed for document symbols.
- **Symbol kind mapping**: `query.go:85-195` — `symbolForNode()` type-switches on 15+ tree node types (FunctionDefinition, TypeDefinitionNode, ClassDefinitionNode, etc.) to extract name, kind, range, deprecated, children.
- **Deprecated detection**: `query.go:195-200` — `isDeprecated()` checks `@deprecated` annotation via `nodeSource(annotReference) == "deprecated"`.
- **ByteRange from LineRange**: `query.go:175-185` — `byteRange()` extracts `node.LineRange()` (StartLine/EndLine with Line/Column) into `ByteRange`.
- **Server symbol dispatch**: `server/symbols.go:14-30` — `handleDocumentSymbol` checks `documentSymbolHierarchySupport` to choose hierarchical vs flat response.
- **Hierarchical conversion**: `server/symbols.go:32-50` — `documentSymbols()` recursively converts `query.DocumentSymbol` → `protocol.DocumentSymbol` with children.
- **Flat conversion**: `server/symbols.go:52-75` — `flatDocumentSymbols()` flattens hierarchy into `protocol.SymbolInformation` with ContainerName.
- **SymbolKind LSP mapping**: `server/symbols.go:77-115` — `toLSPSymbolKind()` maps all 15 `query.SymbolKind` values to `protocol.SymbolKind`.
- **Compile as semantic cache**: `compile.go:99-148` — `Compile()` reads `SnapshotStore.Stable` first (cache hit), falls back to inline compile. Pattern for type-resolution queries.
- **Dual-snapshot engine**: `compile/snapshot.go:1-200` — `SnapshotStore` with `StableSnapshot` (frozen result) + `InProgressSnapshot` (pending/running). Stale-publication gate discards superseded generations (CE-E3).
- **Tiered event bus**: `event/event.go:60-80` — three tiers: CRITICAL (bounded channel, drop on full), COALESCEABLE (last-write-wins per CoalesceKey), BEST_EFFORT (ring buffer, head-drop on full).
- **CE subscriber pattern**: `server/server.go:subscribeDiagnostics` — subscribes to CE-E5a/CE-E5b (BEST_EFFORT), reads stable snapshot via `compile.DiagnosticsFor`, converts to protocol, writes per open document.
- **Generation-staleness guard**: `server/server.go:publishRootDiagnostics` — checks `projects.Generation(root) == gen` before publishing; first-wins per generation (whichever of E5a/E5b arrives first publishes).
- **Eviction clears dedup mark**: `server/server.go:subscribeEvictions` — clears `lastPublished[root]` on ProjectEvicted so reload's gen-1 diagnostics are not suppressed.
- **DocumentSymbol corpus fixture**: `corpus/documentSymbol/testdata/document-symbols.json` — golden-JSON transcript with hierarchical symbols, deprecated tags, children, UTF-16 positions.
