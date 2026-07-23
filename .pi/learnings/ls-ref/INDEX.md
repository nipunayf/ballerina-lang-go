# ls-ref learnings index

Read this first. Open only topic files matching your query + `dead-ends.md` (always).

- [layout.md](layout.md) — directory layout (flat `lsp/` package, no core/query/workspace split), key entry points
- [poc-completion-dto.md](poc-completion-dto.md) — PoC completion DTO design: CompletionItemKind ownership, mapping, hardcoded values, dual-mapping inconsistency, assessment
- [snapshot.md](snapshot.md) — SnapshotManager, module state reuse, snapshot ID wrap, open file preservation, single-file URI tracking
- [diagnostics.md](diagnostics.md) — runDiagnostics, runModuleFrontend (7 stages), topological sort, debounced scheduling, stale-job guards, UTF-16 position conversion
- [server.md](server.md) — dispatchRequest/handleNotification, message framing, watched file routing, protocol types (CompletionItem, TextEdit, etc.), logging
- [symbols.md](symbols.md) — document/workspace symbols, fuzzy match, symbol kind mapping
- [definition-references.md](definition-references.md) — definition (11 AST node types), references (parallel collection, candidate modules)
- [completion-cursor-classification.md](completion-cursor-classification.md) — cursor position → byte offset, identifier prefix, AST node chain, completion context classification (12+ node types), invokable context, bad node detection, position utilities, assessment
- [completion-data-model.md](completion-data-model.md) — member-access read logic (live AST walk + type queries, no index), module-level/expression/type completion (scope hierarchy walk + symbol-space reads), data structures persisted post-compile (Package, CompilationUnits, ImportedSymbols, scopes+types attached to nodes), no invocation/expected-type completion, recompile is scheduling choice not required by reads
- [prototype-shortcuts.md](prototype-shortcuts.md) — no `$/cancelRequest` support, panic-recover as primary error handling, no signature help, `time.Sleep`-debounced diagnostics, full frontend rerun per references/workspace-symbol request
- [dead-ends.md](dead-ends.md) — ALWAYS read (currently empty)
- [ticket36-re-evaluation.md](ticket36-re-evaluation.md) — Re-evaluation of ticket 36: PoC completion prerequisites vs current worktree, five-index architecture, prototype shortcuts, production-ready patterns, remaining gaps

Maintenance: merge into topic headings, never ticket/date sections. Split files >~150 lines and update this index.
