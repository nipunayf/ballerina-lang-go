# Rewrite-LS learnings index

Durable memory for explore-rewrite-ls runs, covering the Java LS
(`ballerina-lang/language-server/`), the TypeScript PoC, and the Go rewrite
(`ballerina-lang-go/ls/`). Read this file first, then open ONLY the topic files
matching your query — plus `dead-ends.md`, always.

- [java-ls-features.md](java-ls-features.md) — Java LS feature architecture: completion, hover, definition, references, signature help; context hierarchy, semantic-model access chain, cancellation/error/URI-scheme patterns. Read for: what a feature must do behaviorally (the spec the Go rewrite matches or diverges from).
- [java-ls-invocation-completion.md](java-ls-invocation-completion.md) — Java LS invocation and type-directed completion: function/method/remote-method call completion, new-expression (explicit/implicit), mapping/list constructor completion, named argument completion, initializer completion, snippet and sort/filter behavior, recovery. Read for: porting invocation and type-directed completion to the Go rewrite.
- [java-ls-expectedType.md](java-ls-expectedType.md) — Every completion provider's use of `SemanticModel.expectedType()` / `BallerinaCompletionContext.getContextType()`: evidence table with 21 direct callers + 4 indirect callers, fallback behavior, and key behavioral notes. Read for: porting expected-type filtering and assignability-based ranking to the Go rewrite.
- [java-ls-async-model.md](java-ls-async-model.md) — Java LS async compilation model (double debounce, per-project locks, ClonedWorkspace) and legacy/workaround behaviors NOT to cargo-cult. Read for: scheduling, debounce, cancellation design; always read before porting Java sync behavior.
- [go-rewrite-state.md](go-rewrite-state.md) — Go rewrite current state: workspace/compile/event-bus entry points, resolution→compile division, diagnostic identity, event ordering, transferable invariants, Java-vs-Go mechanism mapping, choices still requiring HIL. Read for: what exists in `ballerina-lang-go/ls` today and what's still open.
- [completion-tests.md](completion-tests.md) — Java LS completion test infrastructure: class hierarchy, fixture schema, assertion mechanism, fixture counts, skip lists, mocked packages. Read for: porting or replicating completion tests.
- [completion-item-kinds.md](completion-item-kinds.md) — How completion item kinds are represented: Java LS uses LSP `CompletionItemKind` directly everywhere; Go rewrite uses a three-layer protocol-free vocabulary (`projects.CompletionFactKind` → `query.CompletionItemKind` → `protocol.CompletionItemKind`). Read for: ticket 31 target shape, kind mapping, behavioral divergences.
- [dead-ends.md](dead-ends.md) — ALWAYS read (it's short).

Maintenance: merge findings into the matching topic file under its existing
headings — never append ticket-named or date-named sections. If a topic file
exceeds ~150 lines, split it and update this index.
