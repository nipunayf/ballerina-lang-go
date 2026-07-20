# ls-ref learnings index

Read this first. Open only topic files matching your query + `dead-ends.md` (always).

- [layout.md](layout.md) — directory layout, key entry points, core-service seam
- [completion.md](completion.md) — full completion pipeline: context classification, 3 routing paths, scope walking, snippets, builtins, auto-import, member access, test fixtures, extension points
- [snapshot.md](snapshot.md) — SnapshotManager, module state reuse, snapshot ID wrap, open file preservation, project/single-file modes
- [diagnostics.md](diagnostics.md) — runDiagnostics, runModuleFrontend (7 stages), topological sort, debounced scheduling, stale-job guards, UTF-16 position conversion
- [server.md](server.md) — dispatchRequest/handleNotification, message framing, watched file routing, protocol types (CompletionItem, TextEdit, etc.), logging
- [symbols.md](symbols.md) — document/workspace symbols, fuzzy match, symbol kind mapping
- [definition-references.md](definition-references.md) — definition (11 AST node types), references (parallel collection, candidate modules)
- [code-actions.md](code-actions.md) — missing/unused import quick-fixes, import insertion/deletion, known importable modules
- [prototype-shortcuts.md](prototype-shortcuts.md) — synchronous compile, no refcounting, full re-Load, no stale suppression, no async event bus, no cancellation, no ProjectUpdated event, query fragility
- [unsorted-facts.md](unsorted-facts.md) — facts pulled from a past run that describe THIS repo (`ls/core`, `ls/server`, `ls/projects`), not the ls-ref PoC; not yet reconciled, see file header
- [dead-ends.md](dead-ends.md) — ALWAYS read (currently empty)

Maintenance: merge into topic headings, never ticket/date sections. Split files >~150 lines and update this index.
