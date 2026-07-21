# Protocol-free core DTOs and completion item kinds

## Core-service seam (protocol-free core, server-only LSP adaptation)

- **Explicit boundary documented in code:** `ls/server/documents.go:7-9` — "UTF-16 boundary: The helpers below resolve protocol.TextEdit ranges (UTF-16 code-unit positions) to byte offsets in the full text. The server does this before calling workspace.Apply with resolved full text, keeping ls/core protocol-free. These helpers stay in ls/server per the core-service seam design."
- **Diagnostic conversion boundary:** `ls/server/diagnostics.go:7-9` — "UTF-16 boundary: The helpers below convert core CompilerDiagnostic values (byte-offset-derived positions) to protocol.Diagnostic with UTF-16 character positions. Core must not import ls/protocol; the conversion stays here."
- **Workspace package protocol-free:** `ls/core/workspace/workspace.go:7-9` — "Apply carries resolved full text — the server resolves protocol.TextEdit ranges to full text before calling Apply, keeping this package protocol-free."
- **Semantic query surface decision:** `docs/raw/decisions/2026-07-15-semantic-query-surface.md:Decision` — "ls/core/query remains the only feature-facing core facade: it composes private primitives and returns protocol-free feature DTOs. ls/server alone adapts UTF-16 positions, Markdown/snippets, capability behavior, and LSP types."
- **Completion vertical slice decision:** `docs/raw/decisions/2026-07-16-completion-vertical-slice.md:Decision` — "ls/core/query privately reads current syntax to classify the cursor and find parameters/preceding locals, composes semantic facts from a matching index, and returns protocol-free completion DTOs. ls/server converts UTF-16 positions/ranges and DTOs to LSP CompletionList/plain TextEdit values."
- **Module/import/declaration completion decision:** `docs/raw/decisions/2026-07-16-module-import-and-declaration-completion.md:Decision` — "ls/core/query exposes protocol-free item DTOs; ls/server only applies UTF-16/LSP adaptation."

## CompletionItemKind: protocol-free in core, mapped at server

- **Core definition:** `ls/core/query/completion.go:45-55` — `CompletionItemKind` is a `uint8` enum with 7 values: `CompletionKindKeyword`, `CompletionKindVariable`, `CompletionKindConstant`, `CompletionKindFunction`, `CompletionKindType`, `CompletionKindModule`, `CompletionKindSnippet`. Comment: "CompletionItemKind is the protocol-free kind of a completion candidate, owned by the query layer. The server maps it to an LSP CompletionItemKind."
- **Server mapping:** `ls/server/completion.go:135-155` — `toLSPCompletionItemKind()` maps the 7 query kinds to LSP `protocol.CompletionItemKind` values.
- **No protocol types in core:** `ls/core/query/completion.go` imports only `context`, `slices`, `strings`, `ballerina-lang-go/ls/core/uri`, `ballerina-lang-go/parser/tree`, `ballerina-lang-go/projects` — no `ls/protocol` import.
- **Server imports protocol:** `ls/server/completion.go` imports `ballerina-lang-go/ls/protocol` for the LSP type mapping.

## Projects package: protocol-free index facts

- **CompletionFactKind:** `ls/projects/completion_index.go:24-30` — `CompletionFactKind` is a `uint8` enum (Function, ModuleVar, Constant, Type). Comment: "CompletionFactKind is the compiler-defined, protocol-free kind of a completion candidate fact. It is owned by the compiler package so the index never leaks LSP types or compiler objects (AST nodes, scopes, symbols, CompilerContext)."
- **MemberAccessKind:** `ls/projects/member_completion_index.go:23-30` — "MemberAccessKind is the protocol-free, compiler-defined kind of a source member-access slot, owned by the projects package so the projection never leaks compiler objects (AST nodes, symbols, semtypes, CompilerContext) into ls/core/query."
- **MemberCandidateKind:** `ls/projects/member_completion_index.go:45-50` — "MemberCandidateKind is the protocol-free kind of one member candidate, owned by the projects package. The query layer maps it to its own CompletionItemKind."
- **ExpectedSlotKind:** `ls/projects/expected_type_index.go:24-30` — "ExpectedSlotKind is the protocol-free, compiler-defined kind of a contextual expected-type slot, owned by the projects package so the projection never leaks compiler objects (AST nodes, symbols, semtypes, CompilerContext) into ls/core/query."
- **ParamCategory:** `ls/projects/invocation_completion_index.go:27` — "ParamCategory is the protocol-free, compiler-defined kind of one callable parameter slot."

## Two-tier kind mapping chain

1. **Compiler (`projects` package):** defines `CompletionFactKind`, `MemberAccessKind`, `MemberCandidateKind`, `ExpectedSlotKind`, `ParamCategory` — all protocol-free, compiler-defined enums. These are the "source of truth" for what the compiler observed.
2. **Query layer (`ls/core/query`):** defines `CompletionItemKind` — a protocol-free, query-layer enum. The query layer maps from `projects` kinds to its own `CompletionItemKind` when building `CompletionItem` DTOs.
3. **Server (`ls/server`):** maps `query.CompletionItemKind` to `protocol.CompletionItemKind` (LSP type) in `toLSPCompletionItemKind()`.

## Dependency direction

- `ls/core/query` imports `projects` (for index facts) — never imports `ls/protocol`.
- `ls/server` imports `ls/core/query` and `ls/protocol` — adapts query DTOs to LSP types.
- `ls/projects` imports neither `ls/core/query` nor `ls/protocol` — it is the lowest layer.
- `ls/core/compile` imports `projects` and provides lease adapters — never imports `ls/protocol`.
- The `completionLeaseAdapter` in `ls/server/completion.go:15-40` adapts `compile.CompletionLease` to `query.CompletionLeaser`, keeping `ls/core/query` free of `compile` imports.

## Key architectural invariants

- Core never imports `ls/protocol` — enforced by Go import graph.
- Server is the only layer that converts between byte offsets and UTF-16 positions.
- Server is the only layer that converts between core DTOs and LSP protocol types.
- The `projects` package owns the compiler-facing kind enums; the query layer owns the feature-facing kind enum; the server owns the LSP kind enum. Each layer has its own kind type, preventing accidental coupling.
- No compiler objects (AST nodes, symbols, semtypes, CompilerContext) cross the `projects` → `ls/core/query` boundary — only copied, protocol-free facts.
