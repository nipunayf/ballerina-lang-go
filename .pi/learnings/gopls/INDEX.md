# gopls learnings index

Read this first. Open only topic files matching your query + `dead-ends.md` (always).

- [layout.md](layout.md) — Directory structure, key entry points (server, session, protocol dispatch)
- [snapshot.md](snapshot.md) — Snapshot acquisition, lifecycle, ref-counting, clone/invalidation
- [cancellation.md](cancellation.md) — Request cancellation architecture (jsonrpc2 CancelHandler, handler chain, xcontext.Detach)
- [syntax-tree.md](syntax-tree.md) — Single `*ast.File` with no CST layer, position + source-byte losslessness recovery, types.Info keyed semantics, no red/green facade
- [typechecking.md](typechecking.md) — Type-checking batching (futureCache, typeCheckBatch), no-compile policy
- [completion.md](completion.md) — All completion patterns: CompletionItem, candidate, deep search, snippet builder, postfix/statement/keyword, capability negotiation, dispatch, result shape, sort order, budget, selection, CompletionContext, file kind dispatch, toProtocolConversion, settings, completer struct, collectCompletions dispatch
- [protocol.md](protocol.md) — Protocol dispatch, lifecycle states, response result shapes (hover, completion, definition, references, signature help), file kind dispatch
- [hazards.md](hazards.md) — Hazards in gopls's own design, completion-specific hazards, stale-offset prevention (DidModifyFiles → snapshot clone → FileOf → completion)
- [dead-ends.md](dead-ends.md) — Dead ends (no internal/lsp/, no no-compile policy, unimplemented $/setTrace)

Maintenance: merge into topic headings, never ticket/date sections. Split files >~150 lines and update this index.
