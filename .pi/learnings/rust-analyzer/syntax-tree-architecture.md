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
