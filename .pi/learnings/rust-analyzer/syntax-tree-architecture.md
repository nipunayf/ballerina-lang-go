# Syntax tree, HIR, and their integration via Salsa

## Red/green split: rowan CST

- `crates/syntax/src/syntax_node.rs:1-7` — "Concrete Syntax Tree (CST), used by rust-analyzer. The CST includes comments and whitespace, provides a single node type, `SyntaxNode`, and a basic traversal API (parent, children, siblings). The *real* implementation is in the (language-agnostic) `rowan` crate."
- `crates/syntax/src/syntax_node.rs:13` — `pub(crate) use rowan::{GreenNode, GreenToken, NodeOrToken};` — `GreenNode` is rowan's immutable, shareable node pool.
- `crates/syntax/src/syntax_node.rs:29` — `pub type SyntaxNode = rowan::SyntaxNode<RustLanguage>;` — type alias for rowan's on-demand position/parent-aware red facade.
- `crates/syntax/src/syntax_node.rs:39-50` — `SyntaxTreeBuilder` wraps `GreenNodeBuilder`, constructs green nodes, wraps result in `Parse<SyntaxNode>` (rowan red tree).

## Parse is a Salsa query, not eliminated

- `crates/base-db/src/lib.rs:244-246` — trait `RootQueryDb` declares `#[salsa::invoke(parse)] #[salsa::lru(128)] fn parse(&self, file_id: EditionedFileId) -> Parse<ast::SourceFile>;` — **memoized**, never rebuilt until file changes.
- `crates/base-db/src/lib.rs:360-365` — `fn parse(db: &dyn RootQueryDb, file_id: EditionedFileId) -> Parse<ast::SourceFile>` — implementation: reads file text, calls `ast::SourceFile::parse()` (which uses the parser and rowan builder).
- `crates/base-db/src/lib.rs:367-375` — `parse_errors` query (line 368 `#[salsa_macros::tracked]`) depends on `db.parse(file_id)`, confirming parse is a cached query.

## HIR (ItemTree) constructed from syntax via Salsa

- `crates/hir-def/src/db.rs:96-99` — trait `DefDatabase` declares `#[salsa::invoke(file_item_tree_query)] fn file_item_tree(&self, file_id: HirFileId) -> &ItemTree;` — semantic query, memoized.
- `crates/hir-def/src/item_tree.rs:124-149` — `file_item_tree_query(db, file_id)`: calls `db.parse_or_expand(file_id)` (line 129), pattern matches on `ast::SourceFile`/`ast::MacroItems`/`ast::MacroStmts` syntax tree, calls `ctx.lower_module_items(&file)` to lower syntax to ItemTree.
  - **Proof of dependency:** syntax tree is the direct input; HIR is constructed by traversing and lowering it.

## Source maps: HIR retains pointers back to syntax

- `crates/hir-def/src/expr_store.rs` — Uses `InFile<SyntaxNodePtr>` for semantic facts (e.g., `InactiveCode { node: InFile<SyntaxNodePtr>, cfg, opts }`), maintaining traceability from HIR back to source syntax.
- `crates/hir-def/src/signatures.rs` — Source maps populate HIR signatures with `InFile::new(fields.file_id, SyntaxNodePtr::new(field.syntax()))`, linking HIR items to their syntax origin.

## Completion queries directly navigate syntax tree

- `crates/ide-completion/src/context.rs:724` — `let original_token = original_file.syntax().token_at_offset(offset).left_biased()?;` — direct access to CST for cursor classification.
- `crates/ide-completion/src/context.rs:728-747` — token boundary/trivia logic via `original_token.kind()`, `original_token.prev_token()`, navigating syntax siblings/parents directly, **not** via HIR.
- `crates/ide-completion/src/context/analysis.rs:57-96` — `expand_and_analyze()` takes `original_file: InFile<SyntaxNode>`, calls `analyze(sema, expansion, ...)` passing **both** semantic analyzer and syntax tree; cursor classification uses syntax tree for token/trivia identification, semantics for scope.
- `crates/ide-completion/src/context/analysis.rs:4, 10-22` — Function imports `hir::{...}` and `syntax::{SyntaxNode, SyntaxToken, ...}`, showing dual consumption.

## Architecture summary

Three-layer structure, not two:
1. **Green** (rowan immutable pool) — internal to rowan, not directly queried.
2. **Red + typed AST views** (SyntaxNode facade + ast:: wrappers) — full-fidelity CST with position, parent, trivia; returned by `parse` Salsa query; alive for entire session.
3. **HIR** (ItemTree, Body, Type, ExprScopes, etc.) — semantic layer built **on top** of syntax via Salsa queries; queries take syntax as input, build and cache semantic facts, maintain source maps back to syntax.

**Key invariant:** HIR does not eliminate the syntax tree. Salsa memoization wraps both layers, so changing a file invalidates both parse and all downstream HIR queries. Completion and other IDE features query both layers: syntax for position/token/trivia facts, HIR for scopes/types/resolution.

## Abstraction boundaries

### Layer cake (top to bottom)
1. **`crates/rust-analyzer/src/`** — LSP server: `GlobalState`, `GlobalStateSnapshot`, dispatch, handlers. Owns the event loop, thread pools, VFS, `AnalysisHost`. Converts LSP types ↔ internal types.
2. **`crates/ide/src/`** — IDE API: `Analysis`, `AnalysisHost`. LSP-independent semantic API. All methods go through `with_db()` (cancellation boundary). `completions()` delegates to `ide-completion`.
3. **`crates/ide-completion/src/`** — Completion logic: `CompletionContext`, `CompletionAnalysis`, providers. Owns cursor classification, scope derivation, item rendering. Takes `&RootDatabase` directly.
4. **`crates/hir/src/`** — Semantic HIR: `Semantics`, `SemanticsScope`, `SourceAnalyzer`, types, scopes. Bridges syntax positions to HIR definitions.
5. **`crates/hir-def/`, `crates/hir-ty/`, `crates/hir-expand/`** — HIR internals: `ItemTree`, `Body`, `ExprScopes`, `InferenceResult`, macro expansion. Salsa-tracked queries.
6. **`crates/syntax/src/`** — CST: rowan `SyntaxNode`/`GreenNode`, `ast::` typed wrappers, parser. No semantic knowledge.
7. **`crates/base-db/src/`** — Salsa database traits: `SourceDatabase`, `RootQueryDb`. Declares `parse` query.

### Key boundary: `ide` vs `ide-completion`
- `ide/src/lib.rs:229-240` — `Analysis` is the public API; all methods return `Cancellable<T>`
- `ide/src/lib.rs:755-760` — `Analysis::completions()` delegates to `ide_completion::completions()` — the boundary is a function call, not a trait
- `ide-completion/src/lib.rs:191-196` — `completions()` takes `&RootDatabase` directly (not `&Analysis`), bypassing the `Cancellable` wrapper
- **Implication:** `ide-completion` is not cancellation-aware; cancellation is handled at the `ide` layer via `with_db()`

### Key boundary: `hir` vs `syntax`
- `hir/src/semantics.rs:161-170` — `Semantics<'db, DB>` is the bridge: holds `&'db DB` + caches for source-to-def mapping and macro calls
- `hir/src/semantics.rs:2116-2150` — `analyze_impl()`: finds the HIR container for a syntax node, creates `SourceAnalyzer`
- `hir/src/source_analyzer.rs:71-80` — `SourceAnalyzer` wraps `resolver`, `body_or_sig`, `file_id` — never holds syntax nodes directly
- **Direction:** Syntax → HIR is one-way via `Semantics`. HIR never holds syntax nodes; it uses `InFile<SyntaxNodePtr>` for source maps back.

### Key boundary: `Semantics` vs `SemanticsScope`
- `Semantics` is the general-purpose bridge (created per-request, cheap)
- `SemanticsScope` is a scoped handle returned by `scope_at_offset()` — bundles `db + file_id + resolver` for a specific position
- `SemanticsScope::process_all_names()` iterates all names visible at that scope — used by completion to populate `locals`
- `SemanticsScope` does **not** hold a reference to the syntax tree — it's purely semantic

### What this means for a Ballerina LS
- The `ide` layer's `with_db()` cancellation boundary is idiomatic Rust (Salsa unwinding) and doesn't translate directly to Go
- The `Semantics`/`SemanticsScope` pattern of a lightweight per-request handle that bridges syntax→semantics is protocol-level and generalizes
- The `CompletionContext` pattern of constructing a rich context struct from both syntax (token/trivia) and semantics (scope/types) is generalizable
- The `ide-completion` crate being a separate crate with its own `completions()` entry point (not a trait method) is a modularity choice, not protocol-level
