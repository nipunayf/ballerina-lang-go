# Completion cursor classification and AST fidelity

## Cursor position → byte offset

- `lsp/completion.go:1385-1430` — `byteOffsetFromPosition(content, position)` — walks content line-by-line, then character-by-character with UTF-16 surrogate-aware width. This is the single entry point for converting LSP `Position{Line, Character}` to a byte offset. **Production-ready pattern**: handles `\r\n` vs `\n`, UTF-16 surrogate pairs (counts as 2 code units), and bounds-checking.

## Identifier prefix extraction

- `lsp/completion.go:1014-1030` — `identifierPrefixAtOffset(content, offset)` — walks backward from offset collecting identifier runes (`_`, `'`, letters, digits). Returns the raw prefix string used for filtering completion items.
- `lsp/completion.go:1088-1092` — `isIdentifierRune(r rune)` — defines what counts as an identifier character. Simple but sufficient for Ballerina identifiers.

## AST node chain at offset (cursor context)

- `lsp/completion.go:886-920` — `nodeChainAtOffset(cu, offset)` — walks the AST with `ast.Walk`, collecting the ancestor chain of nodes that contain the cursor offset. Uses a `nodeChainAtOffsetFinder` visitor that:
  - Prunes subtrees that don't contain the offset (`locationContains` check at line 904)
  - Maintains a `stack` of visited nodes and a `chain` that is the deepest path to the cursor
  - `ast/walk.go:36` — `Walk(v Visitor, node BLangNode)` — standard visitor pattern; `Visit` returns a child visitor or `nil` to stop descent
- **Key design**: The chain is the shortest path from root to the deepest node containing the cursor. This is used by `completionContextFromNodeChain` to classify the completion context.

## Completion context classification

- `lsp/completion.go:552-560` — `completionContextFromNodeChain(offset, prefix, chain)` — two-phase classification:
  1. `signatureCompletionContextFromNodeChain` — checks for function/function-type signature contexts (parameter types, return type descriptors, return types)
  2. `completionContextAtChainNode` — walks chain from leaf to root, type-switches on each node

- `lsp/completion.go:562-584` — `signatureCompletionContextFromNodeChain` — walks chain from leaf to root looking for `BLangFunction`, `BLangResourceMethod`, `BLangFunctionType`, `BMethodDecl`. Delegates to `invokableCompletionContext` or `functionTypeCompletionContext`.

- `lsp/completion.go:586-650` — `completionContextAtChainNode` — type-switch on 12+ AST node types:
  - `BLangCompilationUnit` → `completionKindModuleVarDecl` (if next node is a top-level decl)
  - `BLangFunction`/`BLangResourceMethod` → `invokableCompletionContext`
  - `BLangFunctionType`/`BMethodDecl` → `functionTypeCompletionContext`
  - `BLangTypeDefinition` → `completionKindRecordTypeDesc` (if prefix starts with "record")
  - `BLangRecordType` → `completionKindType` (if cursor in field type) or `completionKindRecordField`
  - `BLangFieldBaseAccess` → `completionKindMemberAccess` (if cursor after `.`)
  - `BLangSimpleVariable` → `completionKindType` (if cursor in type node) or `completionKindExpression` (if in initializer)
  - `BLangExpressionStmt` → `statementBeginCompletionContext`
  - `BLangSimpleVarRef`/`BLangInvocation` → `completionKindImportedSymbol` (if qualified with `pkg:`)
  - `BLangBlockFunctionBody`/`BLangBlockStmt` → `statementBeginCompletionContext`

- **Prototype shortcut**: The type switch is exhaustive for the PoC's limited AST but will need extension for the target's richer AST. The chain-walk-from-leaf approach is correct but the classification logic is interleaved with the type switch — a production design should separate the "which node am I in" detection from the "what kind of completion should I offer" decision.

## Invokable context classification

- `lsp/completion.go:651-662` — `invokableCompletionContext(offset, prefix, fn)` — three checks in order:
  1. `isFunctionParameterTypeCompletionContext` — cursor in any parameter's type node
  2. `isFunctionReturnTypeDescCompletionContext` — cursor between `)` and body `{` when no explicit return type
  3. `isFunctionReturnTypeCompletionContext` — cursor in or after the return type node when explicit return type exists

- `lsp/completion.go:694-704` — `isFunctionParameterTypeCompletionContext` — iterates `fn.GetParameters()` and `fn.GetRestParam()`, checks if cursor is in the type node of any parameter.

- `lsp/completion.go:714-750` — `isFunctionReturnTypeDescCompletionContext` / `isFunctionReturnTypeCompletionContext` — uses `invokableParamListEndOffset` and `invokableBodyStartOffset` to determine if cursor is in the return-type region between params and body.

- `lsp/completion.go:741-774` — `invokableParamListEndOffset` / `invokableBodyStartOffset` — type-switches on `BLangFunction` vs `BLangResourceMethod` to get `ParamListPos` and body start. **Prototype shortcut**: hardcoded for two function types; the target will need a more general approach.

## Bad node detection (recovering AST)

- `lsp/completion.go:929-970` — `badCompletionKindAtOffset(cu, offset)` — walks AST looking for `BLangBadStmt`, `BLangBadExprOrAction`, `BLangBadIdentifier` nodes at the cursor offset. Used to detect when the user is typing incomplete syntax (e.g., a partial identifier after `import`).

- `lsp/completion.go:971-990` — `importedSymbolContextFromQualifiedName(alias, name, pos, offset)` — detects `pkg:PartialIdent` patterns by checking if the `name` identifier is a `BLangBadIdentifier` or has empty value. Returns `completionKindImportedSymbol` with the alias.

## Position utilities

- `lsp/completion.go:991-1010` — `locationHasOffsets`, `locationHasUsableOffsets`, `locationContains`, `locationSpan` — utility functions for working with `ast.Location` (byte-offset positions). `locationHasUsableOffsets` filters out zero-length positions that indicate unset locations.

## Assessment

**Production-ready patterns to replicate:**
1. `byteOffsetFromPosition` — UTF-16-aware position conversion (completion.go:1385)
2. `nodeChainAtOffset` — AST walk with offset-based pruning (completion.go:886)
3. `identifierPrefixAtOffset` — backward scan for identifier prefix (completion.go:1014)
4. `completionContextFromNodeChain` — two-phase classification (signature first, then node chain) (completion.go:552)

**Prototype shortcuts to avoid:**
1. The 12-case type switch in `completionContextAtChainNode` is brittle — new AST node types require adding cases. A production design should use a visitor pattern or a registry of "context classifiers" per node type.
2. `invokableParamListEndOffset`/`invokableBodyStartOffset` hardcode two function types — the target's richer function type hierarchy needs a more general approach.
3. No caching of the node chain — every completion request re-walks the AST from root.
4. The `completionKind` enum is flat (11 values) — a production design may need hierarchical context (e.g., "inside a function's return type that is a record type").
