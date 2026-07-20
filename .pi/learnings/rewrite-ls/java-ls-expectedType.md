# `expectedType` / `getContextType()` in Java LS completion

`BallerinaCompletionContext.getContextType()` at `BallerinaCompletionContextImpl.java:100` — lazy-loads via `semanticModel.expectedType(document, LinePosition.from(cursor))`; cached after first call. Returns `Optional<TypeSymbol>` representing the type expected at the cursor position (e.g., the LHS type of an assignment, the return type of a function, the parameter type of a call).

## Evidence table: every provider that calls `getContextType()`

| Provider | File:Line | Cursor/Node Position Queried | Facts Consumed | Fallback if Unavailable |
|---|---|---|---|---|
| `AbstractCompletionProvider.populateBallerinaFunctionCompletionItems()` | `AbstractCompletionProvider.java:481` | Any function symbol context | If context type is FUNCTION and symbol is not RESOURCE_METHOD → build function pointer item instead of regular function item | Regular function completion item |
| `AbstractCompletionProvider.getAnonFunctionDefSnippet()` | `AbstractCompletionProvider.java:808` | Expression context | If context type is FUNCTION → generate anonymous function snippet with params from `FunctionTypeSymbol` | `Optional.empty()` (no snippet) |
| `AbstractCompletionProvider.sortByAssignability()` | `AbstractCompletionProvider.java:861-862` | Any completion list | Rank 1 = directly assignable (`subtypeOf`), Rank 2 = assignable-with-check (union with error member), Rank 3 = other | Falls back to rank-only sort text |
| `CheckExpressionNodeContext.sort()` | `CheckExpressionNodeContext.java:88` | Check expression RHS | Clients first (rank 1), `init` methods first, `new` keyword second (rank 2), assignable-with-check items third (rank 3), other fourth (rank 4) | Default rank 4 |
| `ConditionalExpressionNodeContext.sort()` | `ConditionalExpressionNodeContext.java:89` | Ternary expression result position | `genSortTextByAssignability()` for all items | `super.sort()` (default) |
| `DefaultableParameterNodeContext.sort()` | `DefaultableParameterNodeContext.java:69` | Default parameter expression | `genSortTextByAssignability()` for all items | `super.sort()` (default) |
| `ExpressionFunctionBodyNodeContext.sort()` | `ExpressionFunctionBodyNodeContext.java:77` | `=>` expression body | `genSortTextByAssignability()` for all items | `super.sort()` (default) |
| `ExplicitNewExpressionNodeContext.getClassSymbol()` | `ExplicitNewExpressionNodeContext.java:107` | `new` expression | Gets raw type from context type; if it's a CLASS → return `ClassSymbol` for constructor completions | `Optional.empty()` (no class-specific completions) |
| `FromClauseNodeContext.sort()` | `FromClauseNodeContext.java:126` | Query `from` clause | After `in` keyword: rank 1 = assignable to expected type, rank 2 = iterable types (STRING/ARRAY/MAP/TABLE/STREAM/XML), rank 3 = other. On typed binding pattern: `genSortTextByAssignability()` | Default rank 3 for expression context; `genSortTextForTypeDescContext()` for type context |
| `KeySpecifierNodeContext` | `KeySpecifierNodeContext.java:87` | Table key specifier | When no explicit key constraint: uses context type to get `rowTypeSymbol` for record field filtering | Falls back to explicit type parameter from semantic model |
| `LetVariableDeclarationNodeContext.sort()` | `LetVariableDeclarationNodeContext.java:131` | Let variable expression | `genSortTextByAssignability()` for all items | `super.sort()` (default) |
| `ListConstructorExpressionNodeContext.sort()` | `ListConstructorExpressionNodeContext.java:138` | List literal `[...]` | Spread items get rank 1; non-type items get `genSortTextByAssignability()` (rank 1 prefix + assignability + toRank); type items get rank 2 | Default rank for spread items (function=4, variable=3) |
| `ListenerDeclarationNodeContext.sort()` | `ListenerDeclarationNodeContext.java:122` | Listener initializer | Rank 1 = variables assignable to context type, Rank 2 = listener-qualified variables, Rank 3 = `init` methods, Rank 4 = functions returning assignable type, Rank 5 = `new` keyword | `toRank()` with offset 5 |
| `MappingConstructorExpressionNodeContext.sort()` | `MappingConstructorExpressionNodeContext.java:105` | Record/map literal `{...}` | Spread items for MAP type with matching type param get rank 1; all items get `genSortTextByAssignability()` | `super.sort()` (default) |
| `MappingContextProvider.sort()` | `MappingContextProvider.java:370` | Mapping context (record fields) | `genSortTextByAssignability()` for all items | `super.sort()` (default) |
| `NodeWithRHSInitializerProvider.getNewExprCompletionItems()` | `NodeWithRHSInitializerProvider.java:141` | Variable/field declaration with `=` | Gets raw type from context type; if UNION → finds first CLASS/STREAM member; if STREAM → implicit `new` for stream; if CLASS → implicit `new` for class | Empty list |
| `ObjectFieldNodeContext` (2 calls) | `ObjectFieldNodeContext.java:93,114` | Object field initializer | Line 93: if raw context type is CLASS → add implicit `new` completion item. Line 114: `genSortTextByAssignability()` for sorting | No implicit new item; `super.sort()` for sorting |
| `ReturnStatementNodeContext.sort()` | `ReturnStatementNodeContext.java:67` | Return expression | `genSortTextByAssignability()` for all items | `super.sort()` (default) |
| `StartActionNodeContext.sort()` | `StartActionNodeContext.java:105` | `start` expression | If context type is FUTURE → unwraps type parameter; functions get rank 1 + assignability; other items get rank 2 + assignability | `super.sort()` (default) |
| `WaitActionNodeContext.sort()` | `WaitActionNodeContext.java:127` | `wait` expression | Workers rank 1; FUTURE variables matching FUTURE context type with matching type params rank 1 else 2; other items via `toRank()` | `toRank()` with offset 2 |
| `WaitFieldsListNodeContext.sort()` | `WaitFieldsListNodeContext.java:87` | `wait {f1, f2}` fields | `genSortTextByAssignability()` for all items | `super.sort()` (default) |

## Indirect callers of `sortByAssignability()` (which internally calls `getContextType()`)

| Provider | File:Line | Context |
|---|---|---|
| `RemoteMethodCallActionNodeContext.sort()` | `RemoteMethodCallActionNodeContext.java:156` | Remote method call `client->method()` — after client suggestion phase |
| `AsyncSendActionNodeContext.sort()` | `AsyncSendActionNodeContext.java:51` | Async send `e -> w` — each item gets `sortByAssignability()` |
| `AbstractFieldAccessExpressionNodeContext.sort()` | `AbstractFieldAccessExpressionNodeContext.java:71` | Field access `expr.f` — after field/type filtering |
| `MethodCallExpressionNodeContext.sort()` | `MethodCallExpressionNodeContext.java:134` | Method call `expr.f()` — after method filtering |

## Compiler implementation: `SemanticModel.expectedType()` algorithm

### Entry point

`BallerinaSemanticModel.expectedType()` at `BallerinaSemanticModel.java:429-448`:
1. Gets `BLangCompilationUnit` from the document via `getCompilationUnit(sourceDocument)` — this is the **already-resolved** semantic model from the current compilation, not a fresh compilation request
2. Finds the innermost syntax tree node at the cursor via `findInnerMostNode(linePosition, syntaxTree)` at `BallerinaSemanticModel.java:714-718` — converts `LinePosition` to a zero-width `TextRange` and calls `ModulePartNode.findNode(TextRange, true)` (the `true` flag means "find the innermost node")
3. Creates an `ExpectedTypeFinder` instance with the semantic model, BLang compilation unit, compiler context, line position, and source document
4. Walks **up** the syntax tree from the innermost node: calls `node.apply(expectedTypeFinder)`, and if the result is `null` or `empty`, moves to `node.parent()`. Stops when a non-empty result is found or the root is reached.

### `ExpectedTypeFinder` algorithm

`ExpectedTypeFinder` at `ExpectedTypeFinder.java:1-1272` — extends `NodeTransformer<Optional<TypeSymbol>>`. Each `transform()` method handles a specific syntax node kind.

**Core mechanism: syntax tree → BLang AST → expected type**

For each syntax node kind, the finder:
1. Maps the syntax tree node to a `BLangNode` (the compiler's internal AST) via `NodeFinder.lookup(bLangCompilationUnit, node.lineRange())` — this uses line-range matching to find the corresponding BLang AST node
2. Extracts the `expectedType` field from the BLang node (the type the compiler inferred as the expected type for that position during semantic analysis)
3. Converts the `BType` to an API `TypeSymbol` via `typesFactory.getTypeDescriptor(bType)`

**Key: the `expectedType` field on BLang nodes** — this is set by the compiler's type checker during semantic analysis. It represents what type the compiler expects an expression at that position to conform to. The `ExpectedTypeFinder` simply reads this pre-computed field rather than re-computing anything.

### Per-node-type behavior

| Syntax Node Kind | `transform()` Line | Algorithm |
|---|---|---|
| `SimpleNameReferenceNode` | 80-100 | Maps to BLang node via `NodeFinder`. If null, falls back to `findSymbolByName()`. If parent is POSITIONAL_ARG or NAMED_ARG, delegates to parent's transform. Otherwise calls `getExpectedType(bLangNode)` which reads `.expectedType` from the BLang node. |
| `AnnotationNode` | 102-130 | Resolves annotation symbol via QNameRef or SimpleNameRef, returns `AnnotationSymbol.typeDescriptor()` |
| `SpecificFieldNode` | 132-155 | Maps to `BLangRecordVarNameField`, reads `symbol.getType()` or `impConversionExpr` |
| `ObjectFieldNode` | 157-161 | Maps to BLang node, returns `getExpectedType(bLangNode)` |
| `BasicLiteralNode` | 163-167 | Returns `getTypeFromBType(bLangNode.getBType())` — the literal's own type, not expected type |
| `AssignmentStatementNode` | 169-173 | Delegates to `visit(node.varRef())` — the LHS variable reference |
| `FieldAccessExpressionNode` | 175-179 | Maps to BLang node, returns `getExpectedType(bLangNode)` |
| `LetVariableDeclarationNode` | 181-185 | Delegates to `visit(node.typedBindingPattern().bindingPattern())` — the binding pattern's type |
| `ListenerDeclarationNode` | 187-191 | Returns type descriptor of the listener declaration if present |
| `ModuleVariableDeclarationNode` | 193-197 | Delegates to `visit(node.typedBindingPattern().bindingPattern())` |
| `VariableDeclarationNode` | 199-210 | If type descriptor is `var` → returns `any\|error` union. Otherwise delegates to binding pattern. |
| `BinaryExpressionNode` | 212-230 | For `*` or `/` operators → empty. Otherwise reads `rhs.expectedType` or `lhs.expectedType` from BLang binary expression |
| `FunctionCallExpressionNode` | 232-242 | Maps to `BLangInvocation`. If has required args and invokable symbol → `transformFunctionOrMethod()`. Otherwise → `getExpectedTypeFromFunction()` |
| `MethodCallExpressionNode` | 244-258 | Maps to `BLangInvocation`. Resolves langlib method via `LangLibrary.getLangLibMethod()`. Delegates to `transformFunctionOrMethod()` |
| `QualifiedNameReferenceNode` | 260-278 | Returns type of `BLangSimpleVarRef.symbol` or `BLangUserDefinedType.symbol` |
| `ExplicitAnonymousFunctionExpressionNode` | 280-290 | Returns return type of the function |
| `ImplicitAnonymousFunctionExpressionNode` | 292-310 | If cursor is after `=>` → returns function's return type. Otherwise empty. |
| `PositionalArgumentNode` | 312-328 | If parent is METHOD_CALL or FUNCTION_CALL → delegates to parent. Otherwise maps to `BLangRecordLiteral` and returns `getExpectedType()` |
| `FunctionDefinitionNode` | 330-344 | Returns the function's return type descriptor |
| `MatchClauseNode` | 346-350 | Delegates to parent (MatchStatementNode) |
| `MatchStatementNode` | 352-356 | Returns `typeOf(node.condition())` — the type of the match expression |
| `MappingMatchPatternNode` | 358-362 | Delegates to parent (MatchClauseNode) |
| `ExplicitNewExpressionNode` | 364-410 | If cursor within parentheses → iterates `argsExpr` to find parameter at cursor index, returns param type. Otherwise returns `getExpectedType(bLangNode)` |
| `ImplicitNewExpressionNode` | 412-470 | Same pattern as ExplicitNewExpressionNode: within parens → param type by index; otherwise `getExpectedType()` |
| `IfElseStatementNode` | 472-484 | If cursor within condition → returns expected type of the condition expression |
| `WhileStatementNode` | 486-498 | If cursor within condition → returns expected type of the condition expression |
| `DefaultableParameterNode` | 500-504 | Returns `getExpectedType(bLangNode)` |
| `ExpressionFunctionBodyNode` | 506-510 | Returns `getExpectedType(bLangNode)` for the expression |
| `IndexedExpressionNode` | 516-520 | Returns `getExpectedType(bLangNode)` |
| `RemoteMethodCallActionNode` | 522-548 | If within parentheses → finds argument at cursor, returns param type. Otherwise `getExpectedType()` |
| `ListConstructorExpressionNode` | 550-572 | Gets expected type (must be ARRAY), unwraps `eType` (element type) if within brackets |
| `CaptureBindingPatternNode` | 574-596 | Returns type of the binding pattern variable |
| `MappingConstructorExpressionNode` | 598-602 | Delegates to `node.parent().apply(this)` — walks up to enclosing context |
| `WaitActionNode` | 604-620 | Iterates `exprList` to find expression at cursor, returns its expected type |
| `ErrorConstructorExpressionNode` | 622-640 | If within parentheses → returns `detailType` of the error. Otherwise returns the error's `expectedType` |
| `NamedArgumentNode` | 642-656 | If parent is METHOD_CALL or FUNCTION_CALL → delegates to parent. Otherwise `getExpectedType()` |
| `TableConstructorExpressionNode` | 658-700 | If within key specifier → returns constraint type. If within brackets → returns row type. Otherwise `getExpectedType()` |
| `RecordFieldWithDefaultValueNode` | 702-706 | Returns `getExpectedType(bLangNode)` |
| `SelectClauseNode` | 708-716 | Returns `expectedType` of the select clause expression |
| `FromClauseNode` | 722-770 | Complex: if cursor at `from`/`in` keyword → empty. If after `in` → unwraps iterable type (ARRAY→eType, STRING→STRING, TABLE→constraint, STREAM→constraint, XML→constraint, MAP→constraint). Otherwise → `typeOf(expression)` or builds union of iterables from binding pattern type |
| `ClientResourceAccessActionNode` | 772-830 | If within parentheses → finds argument at cursor, returns param type (skipping path params) |
| `transformSyntaxNode` (default) | 512-514 | Falls back to `this.visit(node.parent())` — walks up the tree |

### `getExpectedType(BLangNode)` private method

At `ExpectedTypeFinder.java:834-888` — the central dispatch that reads the `expectedType` field from various BLang node kinds:

| BLang Node Kind | Source of expected type |
|---|---|
| `RECORD_LITERAL_KEY_VALUE` | `field.getValue().expectedType` |
| `USER_DEFINED_TYPE` | `node.getBType()` |
| `VARIABLE` | `((BLangSimpleVariable) node).expr.expectedType` |
| `TABLE_CONSTRUCTOR_EXPR` | `((BLangTableConstructorExpr) node).expectedType` |
| `RECORD_LITERAL_EXPR` | `((BLangRecordLiteral) node).expectedType` |
| `SIMPLE_VARIABLE_REF` | `((BLangSimpleVarRef) node).expectedType` (or `impConversionExpr` for record literals) |
| `NAMED_ARGS_EXPR` | `((BLangNamedArgsExpression) node).expectedType` |
| `LIST_CONSTRUCTOR_EXPR` | `((BLangListConstructorExpr) node).expectedType` |
| `LITERAL` | `((BLangLiteral) node).expectedType` |
| `NUMERIC_LITERAL` | `((BLangNumericLiteral) node).expectedType` |
| `FIELD_BASED_ACCESS_EXPR` | `((BLangFieldBasedAccess) node).expectedType` |
| `TYPE_INIT_EXPR` | `((BLangTypeInit) node).expectedType` |
| `INDEX_BASED_ACCESS_EXPR` | `((BLangIndexBasedAccess) node).expectedType` |
| `INVOCATION` | `((BLangInvocation) node).expectedType` (falls back to `retType` if OTHER) |

### `getExpectedTypeFromFunction()` private method

At `ExpectedTypeFinder.java:1130-1174` — handles function/method call argument resolution:
1. If cursor not within parentheses → returns `getExpectedType(bLangNode)`
2. If no args → returns first parameter type via `getParamType(bLangInvocation, 0, emptyList)`
3. Iterates `argExprs`, counting arguments before cursor (tracking named args separately)
4. For langlib invocations: if first arg type is ARRAY → returns element type; otherwise returns param type at index
5. For regular invocations: if index out of bounds → falls back to `getExpectedType()` on the function arg node; otherwise returns param type or expected type of the expression at that index

### `transformFunctionOrMethod()` private method

At `ExpectedTypeFinder.java:1200-1272` — handles langlib function type parameter binding:
1. If no original invokable or no params → falls back to `getExpectedTypeFromFunction()`
2. Finds type parameter in the first param's type via `TypeParamFinder`
3. If no type param → falls back to `getExpectedTypeFromFunction()`
4. Clones and binds the langlib method with the actual parameter type via `langLibFunctionBinder.cloneAndBind()`
5. Returns the type of the parameter at the computed position index (accounting for `hasFirstArg` and `functionArgNodeAtCursor`)

### `getParamType()` private method

At `ExpectedTypeFinder.java:1030-1060` — resolves parameter type by index:
1. Gets `BInvokableSymbol` from the invocation
2. If index < params.size(): returns param type at index (skipping named args if present)
3. If rest param exists: returns rest param's member type (unwrapping ARRAY)
4. Otherwise: empty

### `buildUnionOfIterables()` private method

At `ExpectedTypeFinder.java:1062-1090` — builds a union of ARRAY, MAP, STREAM (and TABLE if RECORD, STRING if StringTypeSymbol, XML if XML) from a given type symbol. Used by `FromClauseNode` to represent all iterable forms of a type.

### Recovery and null behavior

- **Null BLang node**: `NodeFinder.lookup()` returns null when no BLang AST node matches the syntax tree node's line range. In this case, most `transform()` methods return `Optional.empty()`. Some have fallbacks: `SimpleNameReferenceNode` falls back to `findSymbolByName()`, `SpecificFieldNode` returns empty.
- **Missing tokens**: `QualifiedNameReferenceNode` returns empty if `identifier().isMissing()`. `BinaryExpressionNode` returns empty if operator is missing. `ImplicitAnonymousFunctionExpressionNode` returns empty if `rightDoubleArrow()` is missing.
- **IllegalStateException**: Caught by the caller (`BallerinaSemanticModel.expectedType()` at line 441) — breaks the walk and returns whatever result was found so far (or empty).
- **`TypeKind.OTHER`**: The `getExpectedType()` method returns `Optional.empty()` when `bType.getKind() == TypeKind.OTHER` — this is the compiler's sentinel for "no expected type was inferred".
- **`TypeKind.ANY`**: `SpecificFieldNode` has a special case for `impConversionExpr` with `TypeKind.ANY` — returns the type of the implicit conversion expression.
- **`COMPILATION_ERROR`**: `FromClauseNode` returns empty if the binding pattern's type is `COMPILATION_ERROR`.

### Returned type semantics

The returned `TypeSymbol` represents the type that the compiler's type checker inferred as the **expected type** for the expression at the given position. This is:
- For assignments: the type of the LHS variable
- For return statements: the return type of the enclosing function
- For function arguments: the parameter type of the function being called
- For variable declarations: the declared type (or `any|error` for `var`)
- For mapping constructors: the record/map type being constructed
- For list constructors: the array element type
- For `new` expressions: the class type being instantiated
- For `check` expressions: the type after unwrapping the error union
- For `start` expressions: the `future<T>` type parameter
- For `wait` expressions: the `future<T>` type parameter
- For query `from` clauses: the iterable's element type
- For match statements: the type of the match condition
- For if/while conditions: `boolean` (the expected type of condition expressions)

### Source of truth: current semantic model, not fresh compilation

The `expectedType()` method uses the **already-resolved semantic model** from the current project compilation (`BallerinaSemanticModel` wraps a `BLangCompilationUnit` that was produced during the compilation pipeline). It does NOT trigger a new compilation. The `expectedType` fields on BLang nodes were set by the type checker during the original compilation.

This means:
- The result reflects the state of the code at the last successful compilation
- If the code has errors that prevent type resolution, the `expectedType` field may be `null` or `TypeKind.OTHER`
- The method does not do any incremental or partial re-resolution — it's purely a read of pre-computed data

## Key behavioral notes

- `getContextType()` is lazy and cached: first call invokes `semanticModel.expectedType()`, subsequent calls return cached value (`BallerinaCompletionContextImpl.java:97-103`)
- The `expectedType()` API is provided by the Ballerina compiler's `SemanticModel` interface (not in this repo; it's a dependency from `io.ballerina.compiler.api`)
- Most providers use `genSortTextByAssignability()` which calls `isCompletionItemAssignable()` → `subtypeOf()` check against the context type
- `isCompletionItemAssignable()` at `SortingUtil.java:160` handles TYPEDESC (unwraps type parameter), rejects TypeParameter kind, and checks `subtypeOf(rawType)`
- `isCompletionItemAssignableWithCheck()` at `SortingUtil.java:185` checks if a union type has an error member and a non-error member assignable to context type
- The `sortByAssignability()` method at `AbstractCompletionProvider.java:861` produces a 3-tier sort: directly assignable (1), check-assignable (2), other (3), then appends the item's rank
