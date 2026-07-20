# Java LS invocation and type-directed completion

Invocation completion (function/method/remote-method calls), type-directed
completion (new-expression, mapping/list constructors, named args, initializers),
snippet and sort/filter behavior, and recovery. Java LS root:
`ballerina-lang/language-server/`.

## Invocation completion architecture

### Base: `InvocationNodeContextProvider<T>`

- `InvocationNodeContextProvider` at `providers/context/InvocationNodeContextProvider.java:40` — base for all invocation providers (FunctionCall, ExplicitNew, ImplicitNew, RemoteMethodCall, RightArrowAction). Provides:
  - `getNamedArgCompletionItems()` (line 100): builds `NamedArgCompletionItem` list from `FunctionSymbol` params — REQUIRED and DEFAULTABLE params get `Either.forLeft(ParameterSymbol)`, INCLUDED_RECORD params get `Either.forRight(RecordFieldSymbol)` per field. Skips already-present named args via `NameUtil.getDefinedArgumentNames()`.
  - `isValidNamedArgContext()` (line 170): returns false if any POSITIONAL_ARG or REST_ARG appears before the cursor — named args only suggested when cursor is before any positional/rest arg.
  - `isNotInNamedArgOnlyContext()` (line 185): returns false if any NAMED_ARG ends before cursor — suppresses expression completions when cursor is after a named arg.
  - `sort()` (line 60): dispatches by item type — NAMED_ARG → `sortNamedArgCompletionItem()`, else if parameterSymbol present → `sortDefaultCompletionItem()` (assignability-based), else → `sortParameterlessCompletionItem()` (default rank).
  - `sortNamedArgCompletionItem()` (line 85): 3-tier for included-record fields: required=1.1, defaultable=1.2, optional=1.3; for regular params: 1 + toRank.
  - `sortDefaultCompletionItem()` (line 120): uses `genSortTextByAssignability()` against the parameter's expected type.
  - `getParameterTypeSymbol()` (line 75): calls `semanticModel.expectedType()` at cursor — the parameter type the cursor position is expected to fill.

### Function call: `FunctionCallExpressionNodeContext`

- `FunctionCallExpressionNodeContext` at `providers/context/FunctionCallExpressionNodeContext.java:40` — attached to `FunctionCallExpressionNode`.
- `onPreValidation()` (line 70): cursor must be between `(` and `)` (openParen start < cursor < closeParen end).
- `getCompletions()` (line 45): three branches:
  1. **QNameRef** (`module:<cursor>`): `QNameRefCompletionUtil.getExpressionContextEntries()` → `getCompletionItemList()`.
  2. **Default** (bare cursor): if `isNotInNamedArgOnlyContext()` → `actionKWCompletions()` + `expressionCompletions()`. Then always: `getNamedArgExpressionCompletionItems()`.
  3. **Named args**: resolves `semanticModel.symbol(node)` → must be FUNCTION/METHOD/RESOURCE_METHOD → `getNamedArgCompletionItems()`.
- `getNamedArgExpressionCompletionItems()` (line 80): resolves the function symbol from the call node via `semanticModel.symbol(node)`, then delegates to `getNamedArgCompletionItems()`.

### Method call: `MethodCallExpressionNodeContext`

- `MethodCallExpressionNodeContext` at `providers/context/MethodCallExpressionNodeContext.java:40` — extends `FieldAccessContext<MethodCallExpressionNode>`.
- `onPreValidation()` (line 70): cursor must be within the method name range OR after the dot token (but before node end). Supports: `abc.def.testMethod(<cursor>)`, `abc.def.test<cursor>Method()`, `s<cursor>abc.def.testMethod()`, `self.<cursor>Method()`. Excludes `x.method() <cursor>`.
- `getCompletions()` (line 50): two branches:
  1. **Within parameter context** (cursor between `(` and `)`): QNameRef → `getExpressionContextEntries()`; else `actionKWCompletions()` + `expressionCompletions()`.
  2. **At method name** (cursor at `expr.<cursor>`): delegates to `FieldAccessContext.getEntries()` → `FieldAccessCompletionResolver` resolves expression type → gets visible entries (methods, fields, type guards, foreach snippets).
- `sort()` (line 100): within parameter context → OBJECT_FIELD/RECORD_FIELD rank 1, METHOD/FUNCTION rank 2+toRank; at method name → METHOD/FUNCTION rank 1, OBJECT_FIELD/RECORD_FIELD rank 2, others rank 2+toRank. All items get `sortByAssignability()` applied.

### Remote method call: `RemoteMethodCallActionNodeContext`

- `RemoteMethodCallActionNodeContext` at `providers/context/RemoteMethodCallActionNodeContext.java:40` — extends `RightArrowActionNodeContext<RemoteMethodCallActionNode>`.
- `getCompletions()` (line 50): three phases:
  1. **Before `->`** (`onSuggestClients`, cursor <= rightArrow start): `expressionCompletions()` — suggests client objects.
  2. **After `->`, before `(`** (`onSuggestClientActions`, cursor between rightArrow end and openParen start): resolves expression type via `semanticModel.expectedType()` at expression end line; if client → `getClientActions()` (REMOTE + RESOURCE qualified methods); else → workers + `function` keyword.
  3. **Within `(...)`** (`isInMethodCallParameterContext`): QNameRef → `getExpressionContextEntries()`; else `actionKWCompletions()` + `expressionCompletions()` + named args.
- `sort()` (line 120): within params → delegates to `super.sort()` (InvocationNodeContextProvider); at expression → clients rank 1, others rank 2; at method name → `sortByAssignability()`.
- `getClientActions()` at `RightArrowActionNodeContext.java:80`: filters `ObjectTypeSymbol.methods()` to REMOTE or RESOURCE qualified methods.

### Field access: `FieldAccessContext<T>`

- `FieldAccessContext` at `providers/context/FieldAccessContext.java:40` — base for `MethodCallExpressionNodeContext` and `FieldAccessExpressionNodeContext`.
- `getEntries()` (line 60): resolves expression type via `FieldAccessCompletionResolver`; if XML → xml attribute access completions; else → `resolver.getVisibleEntries(expr)` + type guard + foreach snippets. Also adds `[""]` member access completion item for MAP/RECORD-with-rest.
- `FieldAccessCompletionResolver` at `util/FieldAccessCompletionResolver.java` — resolves expression type and visible entries (methods, fields) for a field access expression.

## Type-directed completion (new expressions)

### Explicit `new`: `ExplicitNewExpressionNodeContext`

- `ExplicitNewExpressionNodeContext` at `providers/context/ExplicitNewExpressionNodeContext.java:40` — extends `InvocationNodeContextProvider<ExplicitNewExpressionNode>`.
- `getCompletions()` (line 50): three branches:
  1. **Within args** (`withinArgs`, cursor between `(` and `)`): QNameRef → `getExpressionContextEntries()`; else `expressionCompletions()` + named args from class init method.
  2. **QNameRef** (`new module:<cursor>`): filters module symbols by `getSymbolFilterPredicate()` → CLASS or STREAM (or listener if parent is SERVICE/LISTENER_DECLARATION).
  3. **Default** (`new <cursor>`): visible symbols filtered by predicate + module completions.
- `getSymbolFilterPredicate()` (line 100): if parent is SERVICE/LISTENER_DECLARATION → listener classes only; else → CLASS or STREAM.
- `getClassSymbol()` (line 90): uses `context.getContextType()` to find the expected class type for constructor completions.
- `getNamedArgExpressionCompletionItems()` (line 140): resolves class symbol from `node.typeDescriptor()` → gets `initMethod()` → `getNamedArgCompletionItems()`.

### Implicit `new`: `ImplicitNewExpressionNodeContext`

- `ImplicitNewExpressionNodeContext` at `providers/context/ImplicitNewExpressionNodeContext.java:40` — extends `InvocationNodeContextProvider<ImplicitNewExpressionNode>`.
- `getCompletions()` (line 50): two branches:
  1. **Within args** (`withinArgs`, cursor between `(` and `)`): QNameRef → `getExpressionContextEntries()`; else `expressionCompletions()` + named args from class init method (resolved via `expectedType()` at parent start line).
  2. **Default** (`new <cursor>`): visible symbols filtered by predicate (CLASS or STREAM type definition) + module completions.
- `getSymbolFilterPredicate()` (line 90): same as explicit — listener classes for SERVICE/LISTENER_DECLARATION parent, else CLASS or STREAM.
- `getNamedArgExpressionCompletionItems()` (line 140): resolves class via `expectedType()` at parent start line → `initMethod()` → `getNamedArgCompletionItems()`.

### Initializer completions: `NodeWithRHSInitializerProvider<T>`

- `NodeWithRHSInitializerProvider` at `providers/context/NodeWithRHSInitializerProvider.java:40` — abstract base for variable/field declarations with `= <cursor>`.
- `initializerContextCompletions()` (line 70): QNameRef → module content (VARIABLE/FUNCTION/TYPE_DEFINITION/CLASS); after qualifiers → `getCompletionItemsOnQualifiers()`; default → `actionKWCompletions()` + `expressionCompletions()` + `getNewExprCompletionItems()`.
- `getNewExprCompletionItems()` (line 120): uses `context.getContextType()` → if UNION → finds first CLASS/STREAM member; if STREAM → implicit `new` snippet; if CLASS → implicit `new` snippet.
- `sort()` (line 50): uses `genSortTextByAssignability()` against `context.getContextType()`.

### Named argument: `NamedArgumentNodeContext`

- `NamedArgumentNodeContext` at `providers/context/NamedArgumentNodeContext.java:40` — attached to `NamedArgumentNode`.
- `onPreValidation()` (line 70): cursor must be after `=` token and within expression range.
- `getCompletions()` (line 45): QNameRef → module content (VARIABLE/FUNCTION/TYPE_DEFINITION/CLASS); else → `expressionCompletions()` + `getNewExprCompletionItems()` (implicit `new` for CLASS context type).
- `sort()` (line 80): uses `genSortTextByAssignability()` against `expectedType()` at node end line.

## Type-directed completion (mapping/list constructors)

### Mapping constructor: `MappingConstructorExpressionNodeContext`

- `MappingConstructorExpressionNodeContext` at `providers/context/MappingConstructorExpressionNodeContext.java:40` — extends `MappingContextProvider<MappingConstructorExpressionNode>`.
- `onPreValidation()` (line 80): cursor must be between `{` and `}`.
- `getCompletions()` (line 50): three scopes via `CommonUtil.getMappingContextEvalNode()`:
  1. **VALUE_EXPR** (after `:` in field): QNameRef → `getExpressionsCompletionsForQNameRef()`; else `expressionCompletions()`.
  2. **COMPUTED_FIELD_NAME** (within `[...]`): QNameRef → `getExpressionsCompletionsForQNameRef()`; else `getComputedNameCompletions()` (VARIABLE + FUNCTION symbols + modules).
  3. **FIELD_NAME** (default): `getFieldCompletionItems()` — record fields from expected type + spread fields + variable completions for matching fields.
- `getFieldCompletionItems()` at `MappingContextProvider.java:150`: resolves record type descriptors via `getRecordTypeDescs()` (uses `expectedType()` at node end line). For each record type: gets valid fields (excluding existing), adds `fillAllStructFields` item, spread field items, variable completions for matching fields. If no record type found → suggests variables as field names + spread fields for MAP type.
- `sort()` (line 90): FIELD_NAME scope → spread items for MAP type with matching type param get rank 1; all items get `genSortTextByAssignability()`. VALUE_EXPR/COMPUTED_FIELD_NAME → delegates to `MappingContextProvider.sort()` (assignability-based).

### `MappingContextProvider<T>` (base)

- `MappingContextProvider` at `providers/context/MappingContextProvider.java:40` — abstract base for mapping constructor contexts.
- `getRecordTypeDescs()` (line 100): calls `expectedType()` at node end line → `RecordUtil.getRecordTypeSymbols()`.
- `getVariableCompletionsForFields()` (line 115): filters visible symbols to those matching record field names AND types.
- `getSpreadFieldCompletionItemsForMap()` (line 210): for MAP expected type — visible MAP variables whose type param is assignable to expected type.
- `getSpreadFieldCompletionItemsForRecordFields()` (line 240): visible symbols whose type is a RECORD whose all fields are members of validFields.
- `isSpreadable()` (line 260): checks if symbol's type is RECORD and all its fields are members of the valid fields set (by name + subtypeOf).
- `sort()` (line 310): uses `genSortTextByAssignability()` against `context.getContextType()`.

### List constructor: `ListConstructorExpressionNodeContext`

- `ListConstructorExpressionNodeContext` at `providers/context/ListConstructorExpressionNodeContext.java:40` — attached to `ListConstructorExpressionNode`.
- `onPreValidation()` (line 100): cursor must be within node range.
- `getCompletions()` (line 50): QNameRef → `getExpressionContextEntries()`; else → `expressionCompletions()` + `spreadOperatorCompletions()` (unless cursor is on SPREAD_MEMBER).
- `spreadOperatorCompletions()` (line 70): uses `expectedType()` to get array element type → filters visible symbols to ARRAY variables/functions whose member type is assignable to expected element type.
- `sort()` (line 110): 3-tier: spread items rank 1.1+lastRank; non-type items rank 1 + assignability + toRank; type items rank 2 + toRank.

## Snippet behavior

### Snippet catalog

- `Snippet` enum at `util/Snippet.java` — ~100 entries: KW_* (keywords), DEF_* (definitions), CLAUSE_* (clauses), EXPR_* (expressions), STMT_* (statements). Each wraps a `SnippetBlock` (label, detail, snippet text, kind, optional auto-imports).
- `SnippetBlock.Kind` — `KEYWORD`, `TYPE`, `SNIPPET`, `MODULE`, `EXPRESSION`, `STATEMENT`.
- `SnippetGenerator` at `util/SnippetGenerator.java` — generates snippet text for each enum entry.

### Snippet items in expression context

- `expressionCompletions()` at `AbstractCompletionProvider.java:512` — 24 snippet items: KW_SERVICE, KW_NEW, KW_ISOLATED, KW_TRANSACTIONAL, KW_FUNCTION, KW_LET, KW_TYPEOF, KW_TRAP, KW_CLIENT, KW_TRUE, KW_FALSE, KW_NIL, KW_CHECK, KW_CHECK_PANIC, KW_IS, EXPR_ERROR_CONSTRUCTOR, EXPR_OBJECT_CONSTRUCTOR, EXPR_BASE16_LITERAL, EXPR_BASE64_LITERAL, KW_FROM, DEF_REG_EXP, DEF_STRING, DEF_XML, DEF_NATURAL_EXPR.
- `actionKWCompletions()` at `AbstractCompletionProvider.java:499` — 4 items: KW_START, KW_WAIT, KW_FLUSH, CLAUSE_FROM.

### Function/method completion items (snippet format)

- `FunctionCompletionItemBuilder.build()` at `builder/FunctionCompletionItemBuilder.java:60` — builds function completion items with `InsertTextFormat.Snippet`. Insert text = `funcName(${1})` with snippet placeholders. Label = `funcName(type param, ...)`. Sets `command` to `editor.action.triggerParameterHints` when function has arguments.
- `buildFunctionPointer()` (line 80): for function pointer context (when context type is FUNCTION) — insertText = funcName (no parens), kind = Variable.
- `build(ClassSymbol, InitializerBuildMode)` (line 95): for class init — EXPLICIT mode uses qualified name, IMPLICIT mode uses `new`. Insert text = `new(${1})` or `module:ClassName(${1})`.
- `buildMethod()` (line 115): for self methods — insertText = `self.methodName(${1})`, label = `self.methodName(...)`.

### Anonymous function snippet

- `getAnonFunctionDefSnippet()` at `AbstractCompletionProvider.java:808` — when context type is FUNCTION → generates anonymous function snippet with params from `FunctionTypeSymbol`. Uses `FunctionGenerator.generateFunction()`.

## Sort/filter behavior

### Default sorting: `SortingUtil.toRank()`

- `toRank()` at `util/SortingUtil.java:230` — ranks by `CompletionItemKind`: Constant=1(onQName)/1, Variable=3/2, Function=1/3, Method=4, Constructor=5, ObjectField=6, RecordField=7, EnumMember=8, Enum=9, Class=10, Interface=11, Event=12, Struct=13, TypeParameter=14, Module=15, Snippet=16, Keyword=17, default=18. `main(` gets rank 25.
- `genSortText(int rank)` at `util/SortingUtil.java:190` — encodes rank as ASCII string: each 25-rank block uses `Z` prefix, remainder maps to `A`-`Y` suffix.

### Assignability-based sorting

- `genSortTextByAssignability()` at `util/SortingUtil.java:160` — 3-tier: directly assignable (rank 1 prefix), function-type-match (rank 2-4), other (rank 4+). Uses `isCompletionItemAssignable()` (subtypeOf check) and `isCompletionItemAssignableWithCheck()` (union-with-error check).
- `sortByAssignability()` at `AbstractCompletionProvider.java:861` — 3-tier: directly assignable=1, check-assignable=2, other=3, then appends item's rank.

### Type-desc context sorting

- `genSortTextForTypeDescContext()` at `util/SortingUtil.java:100` — same-module types=1, other-module types=2, modules=3, constants=4, enums=5, enum members=5, basic types=7, type snippets=8.

### Module sorting

- `genSortTextForModule()` at `util/SortingUtil.java:60` — current-project modules=1, imported modules=2, langlib modules=3, ballerina modules=4, langlib labels=5, standard lib labels=6, other=7.

### Configurable qualifier sorting

- `sortCompletionsAfterConfigurableQualifier()` at `util/SortingUtil.java:280` — anydata subtypes rank 1, type params/structs rank 2, modules rank 3, other rank 4.

### Block node sorting

- `BlockNodeContextProvider.sort()` at `providers/context/BlockNodeContextProvider.java:320` — `return` snippet gets rank 1 when within function/worker with return type.

### Fail statement sorting

- `FailStatementNodeContext.sort()` at `providers/context/FailStatementNodeContext.java:70` — rank 1 = ERROR/union-of-errors/function-returning-error; rank 2 = non-langlib modules; rank 3 = everything else.

## Recovery behavior

### Missing tokens

- `ExplicitNewExpressionNodeContext.onPreValidation()` — no special missing-token handling; relies on `withinArgs()` checking openParen/closeParen.
- `ImplicitNewExpressionNodeContext.withinArgs()` — returns false if `parenthesizedArgList` is empty (no parens → not within args).
- `FunctionCallExpressionNodeContext.onPreValidation()` — checks openParen/closeParen; if closeParen is missing, cursor must be after openParen.
- `MethodCallExpressionNodeContext.onPreValidation()` — checks `!dotToken.isMissing()` before allowing cursor after dot.
- `MappingConstructorExpressionNodeContext.onPreValidation()` — checks `!node.openBrace().isMissing() && !node.closeBrace().isMissing()`.

### Semantic model unavailability

- All providers check `context.currentSemanticModel().isPresent()` and `context.currentDocument().isPresent()` before calling `expectedType()` or `symbol()`. If absent → return empty list or fall back to non-semantic completions.
- `FunctionCallExpressionNodeContext.getNamedArgExpressionCompletionItems()` — returns empty if semanticModel is absent.
- `ExplicitNewExpressionNodeContext.getNamedArgExpressionCompletionItems()` — returns empty if semanticModel is absent.
- `ImplicitNewExpressionNodeContext.getNamedArgExpressionCompletionItems()` — returns empty if semanticModel or document is absent.

### Symbol resolution failure

- `FunctionCallExpressionNodeContext.getNamedArgExpressionCompletionItems()` — returns empty if `semanticModel.symbol(node)` is absent or not FUNCTION/METHOD/RESOURCE_METHOD.
- `ExplicitNewExpressionNodeContext.getNamedArgExpressionCompletionItems()` — returns empty if `symbol(node.typeDescriptor())` is absent or not TYPE/CLASS.
- `ImplicitNewExpressionNodeContext.getNamedArgExpressionCompletionItems()` — returns empty if `expectedType()` is absent or not CLASS.

### Error types in sorting

- `FailStatementNodeContext.isCompletionItemSubTypeOfError()` at `providers/context/FailStatementNodeContext.java:100` — checks raw type: ERROR, UNION where all members are ERROR, or FUNCTION whose return type is ERROR/union-of-errors.
- `isCompletionItemAssignableWithCheck()` at `util/SortingUtil.java:185` — checks if union type has error member AND non-error member assignable to context type.

### Compilation error handling

- `ExpectedTypeFinder` at `ExpectedTypeFinder.java` — `FromClauseNode` returns empty if binding pattern type is `COMPILATION_ERROR`. `getExpectedType()` returns empty for `TypeKind.OTHER`. `IllegalStateException` caught by caller, breaks walk and returns whatever found so far.

## Gaps and contradictions

1. **No `completionItem/resolve` handler** — items are returned fully populated inline. The Go rewrite could choose to use lazy resolution for performance.
2. **`getContextType()` is lazy and cached** — first call invokes `semanticModel.expectedType()`, subsequent calls return cached value. The Go rewrite must replicate this caching.
3. **`expectedType()` uses already-resolved semantic model** — does NOT trigger new compilation. The Go rewrite must ensure the same: read-only access to pre-computed data.
4. **Named arg completion only for REQUIRED and DEFAULTABLE params** — INCLUDED_RECORD params are expanded to their individual fields. REST params are not supported for named arg completion.
5. **Spread operator completions in list/mapping constructors** — only suggested when no positional/rest args exist before cursor (list) or when fields are empty (mapping). This is a deliberate UX choice, not a limitation.
6. **`isNotInNamedArgOnlyContext()` suppresses expression completions** — when cursor is after a named arg, only named arg completions are suggested (no expression completions). This prevents noise when the user is filling named args.
7. **`isValidNamedArgContext()` suppresses named args when positional/rest args precede cursor** — named args only suggested when cursor is before any positional/rest arg. This enforces Ballerina's language rule that named args must come after positional args.
8. **`main(` gets rank 25** — deliberately deprioritized to avoid cluttering completions.
9. **`error` symbol is excluded from expression completions** — covered by `lang.error` langlib instead.
10. **`isolated` variables excluded outside lock statements** — `AbstractCompletionProvider.getCompletionItemList()` at line 200: skips isolated-qualified variables when not within a lock statement.
