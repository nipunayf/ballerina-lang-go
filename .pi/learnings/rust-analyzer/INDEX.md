# rust-analyzer learnings index

Read this first. Open only topic files matching your query + `dead-ends.md` (always).

- [dispatch.md](dispatch.md) — request dispatch modes (on_sync/on/on_latency_sensitive), retry-on-cancel, Salsa snapshot/cancellation, file-edit handling (mem_docs, process_changes), version tracking.
- [completion-context.md](completion-context.md) — top-level `completions()` pipeline, `CompletionContext::new()` construction, scope derivation via `ExprScopes`/`SourceAnalyzer`, cursor classification (`analyze()`, `classify_name_ref`/`classify_name`).
- [completion-providers.md](completion-providers.md) — per-kind providers: expression paths, callable/function snippets, fn params, record fields, snippets, postfix, flyimport.
- [completion-item.md](completion-item.md) — `CompletionItem`/`CompletionRelevance` structure, relevance scoring, LSP conversion + sort/filter/dedup.
- [dead-ends.md](dead-ends.md) — ALWAYS read: no staged/partial completion, no fine-grained cancellation, ExprScopes is a Salsa query.

Maintenance: merge into topic headings, never ticket/date sections. Split files >~150 lines and update this index.
