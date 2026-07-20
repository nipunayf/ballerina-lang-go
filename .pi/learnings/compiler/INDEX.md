# Compiler learnings index

Read this first. Open only topic files matching your query + `gaps.md` + `dead-ends.md` (always).

- [layout-entrypoints.md](layout-entrypoints.md) — package map, Load/Compilation/Modify entry points, semantics resolvers.
- [projects-lifecycle.md](projects-lifecycle.md) — Package/Module/Document model, immutability, modifier chain, two-phase pipeline, thread-safety, snapshot duplication, no-cancellation. Read for: sync, snapshots, concurrency.
- [symbols-scopes.md](symbols-scopes.md) — Symbol types, SymbolRef, scopes, CompilerContext queries. Read for: hover, definition, references, completion.
- [semtypes.md](semtypes.md) — SemType algebra, member-type queries, Context thread-locality.
- [ast-syntax-tree.md](ast-syntax-tree.md) — BLang AST shapes, ast.Walk, red/green tree, parser recovery, positions.
- [diagnostics.md](diagnostics.md) — Diagnostic/DiagnosticEnv/Location, offset→line/col, DiagnosticEnv identity pitfall.
- [ls-core-surface.md](ls-core-surface.md) — LS side (`ls/core`, `ls/server`): SnapshotStore, CompilationService, ProjectService, event bus, cancellation state.
- [semantic-query-design.md](semantic-query-design.md) — design notes for a future semantic query surface.
- [completion-infrastructure.md](completion-infrastructure.md) — CompletionIndex, ExpectedTypeIndex, ImportCatalog, expected-type capture, query-layer classification, server adapter.
- [gaps.md](gaps.md) — what the compiler does NOT expose. ALWAYS read before claiming an API exists.
- [dead-ends.md](dead-ends.md) — ALWAYS read.

Maintenance: merge into topic headings, never ticket/date sections. Split files >~150 lines and update this index.
