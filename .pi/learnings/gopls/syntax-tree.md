# Syntax-tree architecture

gopls uses a single `*ast.File` as its syntax tree with no separate CST layer or red/green split.

## Single syntax tree, keyed semantic side-tables

- `go/parser.ParseFile` (stdlib) returns a single `*ast.File` with no intermediate lossless CST.
- Parsing is on-demand, full re-parse per file (see `parsego.Parse()`, `/Users/wso2/projects/analysis/tools/gopls/internal/cache/parsego/parse.go:50`).
- parsego.File wraps `*ast.File` with position and source info (`parsego.File.File`, `parsego.File.Tok`, `parsego.File.Src`, `file.go:22-35`).
- Semantic information lives in `types.Info` (keyed maps: Defs, Uses, Types, Implicits) and `types.Package`, not a second tree (`cache.Package/syntaxPackage`, `/Users/wso2/projects/analysis/tools/gopls/internal/cache/package.go:56-57`).

## Losslessness via source + positions, not CST

- Lossy AST is sufficient because:
  1. `ast.File.Comments` holds comment groups (`ast.File` struct in stdlib).
  2. All ast.Node subclasses have `Pos()`/`End()` returning token.Pos.
  3. `token.File` maps token.Pos ↔ byte offset.
  4. Original source bytes are cached: `parsego.File.Src` (`file.go:33-35`).
  5. Position-based text recovery: `parsego.File.PosText()`, `NodeText()`, `NodeOffsets()` (`file.go:104-127`).

## Cursor lookups and LSP queries

- Hover/completion use `pgf.Cursor().FindByPos(start, end)` for AST node discovery (inspector.Cursor over `*ast.File`).
- All LSP handlers work directly on go/ast nodes: `hover.go:192-194` gets ast.Ident by position, then extracts type/scope info from types.Info (`hover.go:251-259`).
- No facade tree: no lazy parent/sibling caches, no immutable subtree structure at the syntax layer.

## Immutability is at snapshot/cache level, not syntax-tree

- `persistent.Map` and `immutable.Map` manage file/package caches across snapshots (`snapshot.go:40-44`).
- Lazy `sync.Once` caches wrap inspector cursors and ast.Object resolution (`parsego.File.cursor`, `file.go:37-38, 63-70`), not the tree itself.
- Snapshots are cloned for incremental updates; the underlying `*ast.File` is immutable but not a "green tree" — it's a plain AST.

## Key takeaway

Go's toolchain never needed a separate CST because position + source bytes + side-table semantics cover all LSP needs. This is fundamentally different from a two-tier syntax/AST split (it's one-tier syntax, plus orthogonal maps for semantics).
