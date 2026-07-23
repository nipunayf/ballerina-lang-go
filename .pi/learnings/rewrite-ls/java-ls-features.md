# Java LS feature architecture

Keep entries summarized and pointer-dense — `path` + symbol, one line each.
Java LS root: `ballerina-lang/language-server/`.

## Completion

### Entry & dispatch

- Entry handler: `BallerinaTextDocumentService.completion()` at `BallerinaTextDocumentService.java:148` — wraps in `CompletableFutures.computeAsync`, builds `CompletionContext` via `ContextBuilder.buildCompletionContext()`, delegates to `LangExtensionDelegator.instance().completion()`
- Context construction: `ContextBuilder.buildCompletionContext()` at `ContextBuilder.java:111` — creates `CompletionContextImpl` via builder with `fileUri`, `workspaceManager`, `CompletionCapabilities`, `serverContext`, `cursorPosition`
- Extension dispatch: `LangExtensionDelegator.completion()` at `LangExtensionDelegator.java:80-100` — iterates `completionExtensions` (via `ServiceLoader<LanguageExtension>`), filters by URI scheme (`file:` or custom), calls `ext.validate()` then `ext.execute()`
- Two `CompletionExtension` implementations (SPI):
  - `BallerinaCompletionExtension` (`BallerinaCompletionExtension.java:41`) — validates `.bal`, wraps into `BallerinaCompletionContextImpl`, calls `CompletionUtil.getCompletionItems()`
  - `CompilerPluginCompletionExtension` (`CompilerPluginCompletionExtension.java:59`) — validates `.bal`, delegates to `PackageCompilation.getCompletionManager()` (compiler-plugin completions)
- Both extensions run sequentially; results are concatenated in `LangExtensionDelegator.completion()`

### Token/node resolution & trigger filtering

- `CompletionUtil.fillTokenInfoAtCursor()` at `CompletionUtil.java:130` — uses `PositionUtil.findTokenAtPosition()` to find the token at cursor, then `ModulePartNode.findNode(TextRange)` to find the enclosing `NonTerminalNode`; sets both on context (set-once guard in `BallerinaCompletionContextImpl`)
- `PositionUtil.findTokenAtPosition()` at `PositionUtil.java:50` — walks the syntax tree's token stream, finds the token whose text range contains the cursor position
- Trigger character filtering at `CompletionUtil.java:70-85` — skips completions when:
  - Trigger char is `>` but token is not `->` or `->>` (sync send)
  - Trigger char is `\` but node is not regex literal char/dot/escape
  - Trigger char is `?` but node is not regex flag expression
  - Cursor is within a comment (`isWithinComment()` at `CompletionUtil.java:170`)
- `isWithinComment()` at `CompletionUtil.java:170` — checks leading/trailing minutiae of the token at cursor for `COMMENT_MINUTIAE` spanning the cursor

**Go rewrite divergence**: The Go rewrite's `cursorInComment()` (`ls/core/query/completion.go:cursorInComment`) uses a text-based scanner (not CST trivia), tracking string/template literals and line/block comments. The Java LS uses CST minutiae (`COMMENT_MINUTIAE`). The Go approach is deliberately simpler and avoids CST trivia dependency — acceptable per the design decision that `//` inside `${}` interpolation is treated as part of the template.

### Provider routing (context classification)

### Provider routing (context classification)

- `CompletionUtil.route()` at `CompletionUtil.java:120` — walks the AST parent ladder from cursor node upward; for each node, looks up `ProviderFactory.instance().getProviders()` by `node.getClass()`; calls `provider.onPreValidation()`; if valid and not already in `resolverChain`, selects that provider
- `resolverChain` at `BallerinaCompletionContextImpl.java:50` — list of nodes already tried; prevents infinite loops when a provider declines via `onPreValidation()`
- `ProviderFactory` (`ProviderFactory.java:32`) — singleton; loads all `BallerinaCompletionProvider` via `ServiceLoader`; maps each provider's `getAttachmentPoints()` (node classes) to the instance; HIGH precedence overrides LOW for the same attachment point
- 118 context providers registered in SPI (`META-INF/services/...BallerinaCompletionProvider`), ~150 provider classes in `providers/context/` — one per syntax node type (ModulePartNodeContext, FunctionBodyBlockNodeContext, …)
- `BallerinaCompletionProvider<T>` SPI interface at `BallerinaCompletionProvider.java:33` — `getCompletions()`, `sort()`, `getAttachmentPoints()`, `getPrecedence()`, `onPreValidation()`
- `onPreValidation()` — default returns `true`; overridden by providers like `FunctionBodyBlockNodeContext` (checks cursor is between open/close braces) and `BlockNodeContextProvider` (checks qualifier context)

### Provider implementation pattern

- All providers extend `AbstractCompletionProvider<T extends Node>` at `AbstractCompletionProvider.java:125` — shared helpers:
  - `getCompletionItemList()` — dispatches by symbol kind (FUNCTION/METHOD → `populateBallerinaFunctionCompletionItems()`, VARIABLE → `VariableCompletionItemBuilder`, PARAMETER → `ParameterCompletionItemBuilder`, TYPE_DEFINITION/CLASS/ENUM → `TypeCompletionItemBuilder`, WORKER → `WorkerCompletionItemBuilder`, XMLNS → `XMLNSCompletionItemBuilder`, RECORD_FIELD → `RecordFieldCompletionItem`, OBJECT_FIELD → `ObjectFieldCompletionItem`, CONSTANT/ENUM_MEMBER → `ConstantCompletionItemBuilder`)
  - `getModuleCompletionItems()` — imported modules first (as `SymbolCompletionItem`), then distribution repo packages (as `StaticCompletionItem` with auto-import `TextEdit`), then predeclared langlibs
  - `expressionCompletions()` — keyword snippets + visible symbols (filtered by `getExpressionContextSymbolFilter()`) + basic types + anonymous function def snippet
  - `getTypeDescContextItems()` — `getTypeItems()` + `getModuleCompletionItems()`
  - `sort()` — delegates to `SortingUtil.toDefaultSorting()`
  - `sortByAssignability()` — uses `context.getContextType()` (semanticModel.expectedType()) to rank assignable items first
  - `getCompletionItemsOnQualifiers()` — handles `isolated`, `transactional`, `client`, `service` qualifier chains
  - `getAnonFunctionDefSnippet()` — generates anonymous function snippet when context type is FUNCTION
- `BlockNodeContextProvider<T>` at `BlockNodeContextProvider.java:40` — base for block contexts (function body, if/while/foreach blocks); provides `getStaticCompletionItems()` (keywords), `getStatementCompletionItems()` (if/while/foreach/match/return/panic/break/continue/transaction/retry/rollback/commit/fork), `getSymbolCompletions()` (visible variables + functions), `getTypeDescContextItems()`; overrides `sort()` to rank `return` first when within a function/worker with return type

### Item types & builders

- `LSCompletionItem` interface at `LSCompletionItem.java:27` — `getCompletionItem()`, `getType()`
- `CompletionItemType` enum — `OBJECT_FIELD`, `RECORD_FIELD`, `SNIPPET`, `STATIC`, `SYMBOL`, `TYPE`, `FUNCTION_POINTER`, `NAMED_ARG`, `SPREAD`
- Concrete implementations: `SymbolCompletionItem` (wraps a `Symbol`), `SnippetCompletionItem` (wraps a `SnippetBlock`), `StaticCompletionItem` (non-symbol items like unimported modules), `RecordFieldCompletionItem`, `ObjectFieldCompletionItem`, `FunctionPointerCompletionItem`
- Item builders at `completions/builder/` — 18 classes (Function, Variable, Type, Field, Constant, Worker, Parameter, NamedArg, ResourcePath, Foreach, ServiceTemplate, StreamTypeInit, TypeGuard, Spread, XMLNS, base + utils)
- **No `completionItem/resolve` handler** — items are returned fully populated inline

### Semantic model usage in completion


- `BallerinaCompletionContextImpl` at `BallerinaCompletionContextImpl.java:45` — caches `semanticModel`, `document`, `cursorPosition`, `completionParams`
- `context.getContextType()` at `BallerinaCompletionContextImpl.java:100` — lazy-loads via `semanticModel.expectedType(document, LinePosition.from(cursor))`; cached after first call
- `context.visibleSymbols(position)` at `AbstractDocumentServiceContext.java:100` — calls `workspaceManager.semanticModel(filePath)` → `semanticModel.visibleSymbols(document, linePos, DiagnosticState.VALID, DiagnosticState.REDECLARED)`
- `context.currentDocImportsMap()` at `AbstractDocumentServiceContext.java:130` — iterates `ModulePartNode.imports()`, calls `semanticModel.symbol(importDeclaration)` for each, caches as `Map<ImportDeclarationNode, ModuleSymbol>`
- `context.enclosedModuleMember()` at `BallerinaCompletionContextImpl.java:110` — lazy-loads via `BallerinaContextUtils.getEnclosedModuleMember(syntaxTree, cursorPosInTree)` — walks `ModulePartNode.members()` to find the member containing the cursor
- `context.currentSemanticModel()` at `AbstractDocumentServiceContext.java:160` — calls `workspaceManager.semanticModel(filePath)` (with or without cancelChecker), caches result
- `context.currentDocument()` at `AbstractDocumentServiceContext.java:145` — calls `workspaceManager.document(filePath)`, caches result
- `context.currentSyntaxTree()` at `AbstractDocumentServiceContext.java:175` — calls `workspaceManager.syntaxTree(filePath)` (no caching)
- `context.currentModule()` at `AbstractDocumentServiceContext.java:155` — calls `workspaceManager.module(filePath)`, caches result

### Sorting & ranking

- `SortingUtil` at `SortingUtil.java:69` — all static methods
- `toDefaultSorting()` — calls `toRank()` for each item, sets `sortText` via `genSortText(rank)`
- `toRank()` at `SortingUtil.java:230` — ranks by `CompletionItemKind`: Constant=1(onQName)/1, Variable=3/2, Function=1/3, Method=4, Constructor=5, ObjectField=6, RecordField=7, EnumMember=8, Enum=9, Class=10, Interface=11, Event=12, Struct=13, TypeParameter=14, Module=15, Snippet=16, Keyword=17, default=18; `main(` gets rank 25
- `genSortText(int rank)` at `SortingUtil.java:190` — encodes rank as ASCII string: each 25-rank block uses `Z` prefix, remainder maps to `A`-`Y` suffix (e.g., rank 1 = `A`, rank 26 = `ZA`, rank 27 = `ZB`)
- `genSortTextForTypeDescContext()` — ranks: same-module types=1, other-module types=2, modules=3, constants=4, enums=5, enum members=5, basic types=7, type snippets=8
- `genSortTextForModule()` — ranks: current-project modules=1, imported modules=2, langlib modules=3, ballerina modules=4, langlib labels=5, standard lib labels=6, other=7
- `genSortTextByAssignability()` — ranks: directly assignable=1, assignable-with-check=2, function-type-match=2-4, other=4+
- `sortCompletionsAfterConfigurableQualifier()` — special sorting for `configurable` context: anydata subtypes first
- `BlockNodeContextProvider.sort()` — overrides to rank `return` snippet first when within a function/worker with return type

### Statement context completion (function body blocks)

- `FunctionBodyBlockNodeContext` at `FunctionBodyBlockNodeContext.java:40` — attached to `FunctionBodyBlockNode`; extends `BlockNodeContextProvider<FunctionBodyBlockNode>`
- `onPreValidation()` at `FunctionBodyBlockNodeContext.java:65` — cursor must be between `{` and `}` (openBrace not missing, closeBrace offset >= cursor, openBrace endOffset <= cursor)
- `getCompletions()` at `FunctionBodyBlockNodeContext.java:45` — calls `super.getCompletions()` (BlockNodeContextProvider), then adds `DEF_WORKER` snippet if not on QNameRef and qualifiers are empty or end with TRANSACTIONAL
- `BlockNodeContextProvider.getCompletions()` at `BlockNodeContextProvider.java:55` — three branches:
  1. **QNameRef** (`module:<cursor>`): `QNameRefCompletionUtil.getModuleContent()` filtered to exclude SERVICE_DECLARATION → `getCompletionItemList()`
  2. **After qualifiers** (`isolated <cursor>`): `getCompletionItemsOnQualifiers()` — for ISOLATED adds `KW_FUNCTION` + `DEF_OBJECT_TYPE_DESC_SNIPPET`
  3. **Default** (bare cursor): `getStaticCompletionItems()` + `getStatementCompletionItems()` + `getTypeDescContextItems()` + `getSymbolCompletions()`

#### Static completion items (keywords/prefixes)
- `getStaticCompletionItems()` at `BlockNodeContextProvider.java:100` — 22 items: `STMT_NAMESPACE_DECLARATION` (xmlns snippet), `KW_XMLNS`, `KW_VAR`, `KW_WAIT`, `KW_START`, `KW_FLUSH`, `KW_NEW`, `KW_ISOLATED`, `KW_TRANSACTIONAL`, `KW_LET`, `KW_TYPEOF`, `KW_TRAP`, `KW_CLIENT`, `KW_CHECK_PANIC`, `KW_CHECK`, `KW_FINAL`, `KW_FAIL`, `EXPR_ERROR_CONSTRUCTOR`, `EXPR_OBJECT_CONSTRUCTOR`, `EXPR_BASE16_LITERAL`, `EXPR_BASE64_LITERAL`, `KW_FROM`

#### Statement completion items (control flow/worker snippets)
- `getStatementCompletionItems()` at `BlockNodeContextProvider.java:130` — conditional on context:
  - **Always**: `STMT_IF`, `STMT_WHILE`, `STMT_DO`, `STMT_LOCK`, `STMT_FOREACH`, `STMT_FOREACH_RANGE_EXP`, `STMT_TRANSACTION`, `STMT_RETRY`, `STMT_RETRY_TRANSACTION`, `STMT_MATCH`, `STMT_PANIC`, `DEF_STREAM`
  - **Fork** (`onSuggestFork`): `STMT_FORK` — only if enclosing function is NOT `isolated`
  - **Within transaction** (`withinTransactionStatement`): `STMT_ROLLBACK`, `STMT_COMMIT`
  - **Return type**: `STMT_RETURN` (with expression) if return type is non-NIL; `STMT_RETURN_SC` (return;) if NIL or no return type — uses `ReturnTypeFinder` to walk parent ladder
  - **Node before cursor** (`nodeBeforeCursor`): if IF_ELSE_STATEMENT → `STMT_ELSE_IF` + `STMT_ELSE`; if DO/MATCH/FOREACH/WHILE/LOCK → `CLAUSE_ON_FAIL`
  - **Within loop** (`withinLoopConstructs`): `STMT_CONTINUE`, `STMT_BREAK` — walks parent ladder for WHILE_STATEMENT or FOREACH_STATEMENT

#### Symbol completions in statement context
- `getSymbolCompletions()` at `BlockNodeContextProvider.java:170` — `context.visibleSymbols()` filtered by `CommonUtil.getVariableFilterPredicate()` OR `symbol.kind() == FUNCTION`

#### Sorting in statement context
- `BlockNodeContextProvider.sort()` at `BlockNodeContextProvider.java:320` — if within function/worker with return type, `return` gets rank 1; otherwise delegates to `super.sort()` (SortingUtil.toDefaultSorting)
- `withinFunctionOrWorkerWithReturn()` at `BlockNodeContextProvider.java:290` — walks parent ladder for FUNCTION_DEFINITION or NAMED_WORKER_DECLARATION; checks if returnTypeDesc is present

#### Return statement context
- `ReturnStatementNodeContext` at `ReturnStatementNodeContext.java:40` — attached to `ReturnStatementNode`
- `onPreValidation()` at `ReturnStatementNodeContext.java:90` — cursor must be after `return` keyword
- `getCompletions()` at `ReturnStatementNodeContext.java:50` — if on QNameRef → `getExpressionContextEntries()`; else `actionKWCompletions()` + `expressionCompletions()`
- `sort()` at `ReturnStatementNodeContext.java:70` — uses `genSortTextByAssignability()` against `context.getContextType()` (the function's return type)

#### Fail statement context
- `FailStatementNodeContext` at `FailStatementNodeContext.java:40` — attached to `FailStatementNode`
- `getCompletions()` at `FailStatementNodeContext.java:55` — if QNameRef → `getExpressionContextEntries()`; else `expressionCompletions()`
- `sort()` at `FailStatementNodeContext.java:70` — rank 1 = symbols whose type is ERROR or union-of-errors or function returning error; rank 2 = non-langlib modules; rank 3 = everything else
- `isCompletionItemSubTypeOfError()` at `FailStatementNodeContext.java:100` — checks raw type: ERROR, UNION where all members are ERROR, or FUNCTION whose return type is ERROR/union-of-errors

#### Fixture counts (statement_context)
- 170 JSON configs, ~170 .bal sources under `resources/completion/statement_context/config/` and `source/`
- Groups: assignment_stmt (13), async_send_action (2), checking_call_stmt (2), continue_break_stmt (2), do_stmt (2), else_stmt (2), elseif_stmt (6), fail_stmt (2), flush_action (2), fork_stmt (3), function_call_stmt (2), if_stmt (12), local_var_decl_stmt (1), lock_stmt (3), match_stmt (24), onfail_clause (12), panic_stmt (3), receive_action (2), return_stmt (18), start_action (7), sync_send_action (2), transaction (3), typeguard_stmt (4), wait_action (19), while_stmt (8), worker_declaration (7), xmlns (6)
- Skip list (StatementContextTest): elseif_ctx_config3, match_stmt_ctx_config8-11, start_action_ctx_config4a

### Expression context completion

- No single `ExpressionNodeContext` class — expression completions are dispatched per syntax node kind via `CompletionUtil.route()`
- Providers for expression contexts: `CheckExpressionNodeContext`, `ConditionalExpressionNodeContext`, `TrapExpressionNodeContext`, `TypeCastExpressionNodeContext`, `FunctionCallExpressionNodeContext`, `ErrorConstructorExpressionNodeContext`, `ExplicitNewExpressionNodeContext`, `ImplicitNewExpressionNodeContext`, `IndexedExpressionNodeContext`, `ListConstructorExpressionNodeContext`, `MappingConstructorExpressionNodeContext`, `MappingContextProvider`, `LetVariableDeclarationNodeContext`, `ExpressionFunctionBodyNodeContext`, `WaitActionNodeContext`, `StartActionNodeContext`, `FromClauseNodeContext`, `CollectClauseNodeContext`, `JoinClauseNodeContext`, `IfElseStatementNodeContext`, `WhileStatementNodeContext`, `DefaultableParameterNodeContext`, `AnnotationAccessExpressionNodeContext`, `FieldAccessExpressionNodeContext`, `MethodCallExpressionNodeContext`, `RemoteMethodCallActionNodeContext`, `ClientResourceAccessActionNodeContext`, `AsyncSendActionNodeContext`, `SyncSendActionNodeContext`, `ReceiveActionNodeContext`, `FlushActionNodeContext`, `WaitFieldsListNodeContext`, `BinaryExpressionNodeContext`, `UnaryExpressionNodeContext`, `BracedExpressionNodeContext`, `QueryExpressionNodeContext`, `TableConstructorExpressionNodeContext`, `LetExpressionNodeContext`, `XmlTemplateExpressionNodeContext`, `StringTemplateExpressionNodeContext`, `RawTemplateExpressionNodeContext`

#### `expressionCompletions()` (shared core)
- `AbstractCompletionProvider.expressionCompletions()` at `AbstractCompletionProvider.java:512` — returns:
  1. **Module completions** (`getModuleCompletionItems()`): imported modules + distribution packages (with auto-import TextEdits) + predeclared langlibs
  2. **Keyword/snippet items** (18 items): `KW_SERVICE`, `KW_NEW`, `KW_ISOLATED`, `KW_TRANSACTIONAL`, `KW_FUNCTION`, `KW_LET`, `KW_TYPEOF`, `KW_TRAP`, `KW_CLIENT`, `KW_TRUE`, `KW_FALSE`, `KW_NIL`, `KW_CHECK`, `KW_CHECK_PANIC`, `KW_IS`, `EXPR_ERROR_CONSTRUCTOR`, `EXPR_OBJECT_CONSTRUCTOR`, `EXPR_BASE16_LITERAL`, `EXPR_BASE64_LITERAL`, `KW_FROM`, `DEF_REG_EXP`, `DEF_STRING`, `DEF_XML`, `DEF_NATURAL_EXPR`
  3. **Visible symbols** filtered by `getExpressionContextSymbolFilter()`: VARIABLE, FUNCTION, TYPE_DEFINITION, CLASS (excluding `error` symbol name) — sorted alphabetically
  4. **Basic types** (`getBasicAndOtherTypeCompletions`): readonly, handle, never, json, anydata, any, byte
  5. **Anonymous function snippet** (`getAnonFunctionDefSnippet`): if context type is FUNCTION, generates snippet with params from FunctionTypeSymbol
- `getExpressionContextSymbolFilter()` at `AbstractCompletionProvider.java:550` — `CommonUtil.getVariableFilterPredicate()` OR (FUNCTION/TYPE_DEFINITION/CLASS and name != "error")

#### `actionKWCompletions()` (action prefix keywords)
- `AbstractCompletionProvider.actionKWCompletions()` at `AbstractCompletionProvider.java:499` — 4 items: `KW_START`, `KW_WAIT`, `KW_FLUSH`, `CLAUSE_FROM`

#### Check expression context
- `CheckExpressionNodeContext` at `CheckExpressionNodeContext.java:40` — attached to `CheckExpressionNode`
- `getCompletions()` at `CheckExpressionNodeContext.java:50` — special case: if parent is ASSIGNMENT_STATEMENT/LOCAL_VAR_DECL/MODULE_VAR_DECL/OBJECT_FIELD/FROM_CLAUSE → delegates to `CompletionUtil.route(ctx, node.parent())` (re-routes to parent context). Otherwise: QNameRef → `getExpressionContextEntries()`; else `actionKWCompletions()` + `expressionCompletions()` + `STMT_COMMIT`
- `sort()` at `CheckExpressionNodeContext.java:80` — 4-tier: rank 1 = clients + init methods; rank 2 = `new` keyword; rank 3 = assignable-with-check items (union with error member); rank 4 = everything else

#### Conditional expression context
- `ConditionalExpressionNodeContext` at `ConditionalExpressionNodeContext.java:40` — attached to `ConditionalExpressionNode`
- `getCompletions()` at `ConditionalExpressionNodeContext.java:50` — three branches:
  1. **Middle expression QNameRef** (`true ? module1:`): `getExpressionContextEntries()` by module alias; if empty → fallback to `expressionCompletions()`
  2. **QNameRef** (`true ? mod1:<cursor>`): `getExpressionContextEntries()`
  3. **Default**: `expressionCompletions()`
- `sort()` at `ConditionalExpressionNodeContext.java:80` — uses `genSortTextByAssignability()` against `context.getContextType()`

#### Trap expression context
- `TrapExpressionNodeContext` at `TrapExpressionNodeContext.java:40` — attached to `TrapExpressionNode`
- `getCompletions()` at `TrapExpressionNodeContext.java:45` — QNameRef → `getExpressionContextEntries()`; else `actionKWCompletions()` + `expressionCompletions()`
- No custom sort — uses `super.sort()` (SortingUtil.toDefaultSorting)

#### Type cast expression context
- `TypeCastExpressionNodeContext` at `TypeCastExpressionNodeContext.java:40` — attached to `TypeCastExpressionNode`
- `onPreValidation()` at `TypeCastExpressionNodeContext.java:70` — cursor must be within `<>` (before `>` token end offset); otherwise parent context handles it
- `getCompletions()` at `TypeCastExpressionNodeContext.java:50` — QNameRef → `getTypesInModule()`; else `getTypeDescContextItems()`
- `sort()` at `TypeCastExpressionNodeContext.java:80` — uses `genSortTextForTypeDescContext()`

#### Fixture counts (expression_context)
- 332 JSON configs, ~332 .bal sources under `resources/completion/expression_context/config/` and `source/`
- Groups: annotation_access (13), anon_func_expr (16), check_expression (12), conditional_expr (12), error_constructor_expr (16), fail_expr (3), function_call_expression (22), lang_lib_func_expr (10), list_constructor (11), mapping_constructor_expr (14), mapping_expr (67+), object_constructor_expr (12+)
- Skip list (ExpressionContextTest): object_constructor_expr_config12a/6/11, conditional_expr_ctx_config12, method_call_expression_ctx_config9, mapping_constructor_expr_config7/8, mapping_expr_ctx_config37

### Snippet catalog

- `Snippet.java` (394 lines) at `completions/util/Snippet.java` — enum with ~100 entries: KW_* (keywords), DEF_* (definitions), CLAUSE_* (clauses), EXPR_* (expressions), STMT_* (statements)
- Each entry wraps a `SnippetBlock` (label, detail, snippet text, kind, optional auto-imports) generated by `SnippetGenerator`
- `SnippetBlock.Kind` — `KEYWORD`, `TYPE`, `SNIPPET`, `MODULE`, `EXPRESSION`, `STATEMENT`

### Module completion (used in type-desc and expression contexts)

- `AbstractCompletionProvider.getModuleCompletionItems()` at `AbstractCompletionProvider.java:330` — three sources:
  1. **Imported modules** (lines 348-370): iterates `ctx.currentDocImportsMap()`, skips predeclared langlibs, creates `SymbolCompletionItem` wrapping `ModuleSymbol` with label=prefix, insertText=escaped prefix, no additionalTextEdits
  2. **Distribution repo packages** (lines 372-397): `LSPackageLoader.getInstance().getAllVisiblePackages(ctx)`, skips already-imported and predeclared langlibs, creates `StaticCompletionItem(Kind.MODULE)` with label=org/pkg, insertText=last-component, filterText=last-component, and `additionalTextEdits` via `CommonUtil.getAutoImportTextEdits("", label, alias, ctx)` — inserts `import pkgName;\n` at end of imports block
  3. **Predeclared langlibs** (line 398): `getPredeclaredLangLibCompletions()` — visible `ModuleSymbol`s whose moduleName is in `CommonUtil.PRE_DECLARED_LANG_LIBS`; creates `SymbolCompletionItem` with `TypeCompletionItemBuilder` (treated as types, not modules)
- `CommonUtil.getAutoImportTextEdits()` at `CommonUtil.java:157` — finds last import in current doc, creates `TextEdit` inserting `import org/pkg;\n` at end of imports block
- `CommonUtil.getAutoImportTextEdits(orgName, pkgName, alias, ctx)` at `CommonUtil.java:182` — same but with optional `as alias` clause
- `CommonUtil.getImportPosition()` at `CommonUtil.java:197` — finds insertion line: after last import, or after first non-import member, or at line 0

### Import declaration completion

- `ImportDeclarationNodeContext` at `ImportDeclarationNodeContext.java:67` — attached to `ImportDeclarationNode`
- `onPreValidation()` at `ImportDeclarationNodeContext.java:380` — returns true if cursor is before semicolon (or semicolon is missing)
- `getCompletions()` at `ImportDeclarationNodeContext.java:80` — dispatches to 4 context scopes:
  - **`onPrefixContext()`** (line 100): cursor after `as` keyword → returns empty list (no completions for prefix name)
  - **`onSuggestCurrentProjectModules()`** (line 105): `import pkgname.` or `import pkg.m` with no orgName → `getCurrentProjectModules()` lists sibling modules of current package (excluding default module and current module)
  - **`onSuggestAsKeyword()`** (line 110): cursor after module name, no `as` keyword yet → suggests `as` keyword snippet
  - **`orgNameContextCompletions()`** (line 217): cursor at `import <cursor>` or `import b<cursor>` → lists all visible packages as org names (deduplicated), plus individual packages. For each org, also adds the org name itself (e.g., `ballerina/`). Skips `ballerinai` org. Langlibs get `;` appended to insertText. Predeclared langlibs get `'` escape on the langlib name (e.g., `lang.'int;`)
  - **`moduleNameContextCompletions()`** (line 300): cursor at `import org/<cursor>` or `import org/mod.xx<cursor>` → lists packages matching the org. For `ballerinax` org, uses `getCentralPackages()` with prefix filtering via `CompletionSearchProvider`. For other orgs, uses `getAllVisiblePackages()`. When module name has multiple parts (e.g., `ballerina/lang.`), adds `additionalTextEdits` to replace the partial module name
- `ImportOrgNameNodeContext` at `ImportOrgNameNodeContext.java:40` — attached to `ImportOrgNameNode`; handles `import org/<cursor>`; same logic as `moduleNameContextCompletions()` but standalone
- `ImportDeclarationContextUtil.getImportCompletion()` at `ImportDeclarationContextUtil.java:40` — creates `StaticCompletionItem(Kind.MODULE)` with label, insertText, kind=Module, detail=Module
- `ImportDeclarationContextUtil.getLangLibModuleNameInsertText()` at `ImportDeclarationContextUtil.java:28` — for predeclared langlibs, replaces `.` with `.'` (e.g., `lang.'int;`); appends `;`

### Module part (top-level declaration) completion

- `ModulePartNodeContext` at `ModulePartNodeContext.java:40` — attached to `ModulePartNode`
- `getCompletions()` at `ModulePartNodeContext.java:50` — three branches:
  1. **Service type desc context** (`onServiceTypeDescContext`): cursor after `service` keyword → object-type symbols (CLASS, TYPE_DEFINITION with OBJECT raw type) + module completions + `on` keyword + qualifier completions
  2. **After qualifiers** (`onSuggestionsAfterQualifiers`): cursor after `public`/`isolated`/`transactional`/`client`/`service`/`configurable` → qualifier-specific snippets (type desc items, function/class/record snippets, etc.)
  3. **Default** (`getModulePartContextItems`): cursor at module level → top-level items + type desc items + service templates
- `ModulePartNodeContextUtil.getTopLevelItems()` at `ModulePartNodeContextUtil.java:60` — returns keyword snippets (type, public, isolated, final, const, listener, client, var, enum, xmlns, class, transactional, configurable) + definition snippets (function, expression-bodied function, annotation, record, object, class, enum, closed record, error type, table, table with key, stream, service) + `import` keyword (only if cursor is before any non-import member) + `public main function` snippet (if no `main` function exists)
- `ModulePartNodeContextUtil.isInImportStatementsContext()` at `ModulePartNodeContextUtil.java:100` — returns true if cursor is before the first non-import member declaration
- `ModulePartNodeContextUtil.isMainFunctionUnavailable()` at `ModulePartNodeContextUtil.java:90` — checks if no visible function named `main` exists
- `ModulePartNodeContextUtil.onServiceTypeDescContext()` at `ModulePartNodeContextUtil.java:140` — checks if cursor is after `service` keyword (including in minutiae)
- `ModulePartNodeContextUtil.serviceTypeDescContextSymbols()` at `ModulePartNodeContextUtil.java:155` — filters visible symbols to CLASS or TYPE_DEFINITION with OBJECT raw type
- `ModulePartNodeContextUtil.serviceTypeDescPredicate()` at `ModulePartNodeContextUtil.java:165` — predicate for service type desc filtering
- `ModulePartNodeContext.getModulePartContextItems()` at `ModulePartNodeContext.java:130` — if cursor is on a qualified name reference (`module:Type`), returns types from that module; otherwise returns top-level items + type desc items + service templates
- `ModulePartNodeContext.getCompletionItemsOnQualifiers()` at `ModulePartNodeContext.java:100` — overrides base to add qualifier-specific snippets per qualifier kind (PUBLIC → all definition snippets; SERVICE/CLIENT → class; ISOLATED → class/type desc; TRANSACTIONAL → function; CONFIGURABLE → type desc)
- `ModulePartNodeContext.sort()` at `ModulePartNodeContext.java:155` — delegates to `ModulePartNodeContextUtil.sort()` for DEFAULT, or `SortingUtil.sortCompletionsAfterConfigurableQualifier()` for CONFIGURABLE_QUALIFIER
- `ModulePartNodeContextUtil.sort()` at `ModulePartNodeContextUtil.java:115` — multi-tier sort: service template (1+3), main function (1+1), service snippet (1+2), function snippet (1+5), closed record (1+6), record (1+7), other snippets (2), types (3+1), langlib types (3+2), service/function keywords (1+4), other keywords (4), langlib modules (5), other modules (6), everything else (7)

### Service templates

- `ServiceTemplateGenerator` at `ServiceTemplateGenerator.java` — loads `service_templates.json` from distribution; for each module with listener types, generates `service on new Module:Listener(...)` snippets with auto-import `additionalTextEdits`
- Called from `ModulePartNodeContext.getModulePartContextItems()` at `ModulePartNodeContext.java:140`

### Context hierarchy (completion-specific)

```
DocumentServiceContext (fileUri, filePath, workspace, languageServerContext)
  └── PositionedOperationContext (cursorPosition, cursorPositionInTree, visibleSymbols)
       └── CompletionContext (capabilities)
            └── BallerinaCompletionContext (tokenAtCursor, nodeAtCursor, resolverChain, contextType, completionParams)
                 └── BallerinaEnclosedPositionContext (enclosedModuleMember)
```

- `BallerinaEnclosedPositionContext` at `BallerinaEnclosedPositionContext.java:29` — adds `enclosedModuleMember()` for self-class/self-object symbol resolution

## Hover

- Entry: `BallerinaTextDocumentService.hover()` at `BallerinaTextDocumentService.java:80` — `CompletableFutures.computeAsync`, `ContextBuilder.buildHoverContext()`, delegates to `HoverUtil.getHover()`
- Context: `HoverContextImpl` — extends `PositionedOperationContextImpl`, adds `tokenAtCursor`/`nodeAtCursor` (set-once guard)
- Core: `HoverUtil.getHover()` at `HoverUtil.java:60` — `fillTokenInfoAtCursor()` → `getSymbolAtCursor()` (`semanticModel.symbol()`) → `HoverObjectResolver.getHoverObjectForSymbol()` or `getHoverObjectForExpression()`
- `HoverUtil.getSymbolAtCursor()` at `HoverUtil.java:130` — special-cases LIST, PARENTHESIZED_ARG_LIST, CLIENT_RESOURCE_ACCESS_ACTION (uses `semanticModel.symbol(cursor.parent())`)
- `HoverObjectResolver` — dispatches by symbol kind (FUNCTION, METHOD, RESOURCE_METHOD, TYPE_DEFINITION, CLASS, VARIABLE, PARAMETER, TYPE→definition); builds markdown with signature, docs, params, return type
- `HoverSymbolResolver` — walks parents for `QualifiedNameReferenceNode`; appends "[View API Docs](url)" for external-module symbols
- Cancellation: `context.checkCancelled()` before and after `semanticModel.symbol()`
- Edge cases: empty semanticModel/document → empty hover; empty symbolAtCursor → `MatchedExpressionNodeResolver` fallback for expression hover (`new ClassName(...)`)
- `HoverUtil.withValidAccessModifiers()` at `HoverUtil.java:170` — PRIVATE/PUBLIC/RESOURCE/REMOTE filtering of fields/methods in hover

## Definition

- Entry: `BallerinaTextDocumentService.definition()` at `BallerinaTextDocumentService.java:120` — `computeAsync`, `buildDefinitionContext()`, delegates to `DefinitionUtil.getDefinition()`
- Core: `DefinitionUtil.getDefinition()` at `DefinitionUtil.java:50` — `fillTokenInfoAtCursor()` → `semanticModel.symbol()` → `getLocation()`
- `DefinitionUtil.getLocation()` at `DefinitionUtil.java:80` — symbol → `PathUtil.getFilePathForSymbol()` → write-protected check → `PathUtil.getBalaUriForPath()` for bala files, else `absFilePath.toUri()`
- Special case: `SymbolUtil.isSelfClassSymbol()` at `DefinitionUtil.java:65` — `self` in class navigates to class definition via `CommonUtil.getRawType(...)`
- Returns `List<Location>` (single element or empty)

## References

- Entry: `BallerinaTextDocumentService.references()` at `BallerinaTextDocumentService.java:140` — `computeAsync`, `buildReferencesContext()`, delegates to `ReferencesUtil.getReferences()`
- Core: `ReferencesUtil.getReferences()` at `ReferencesUtil.java:50` — `getSymbolAtCursor()`, then iterates all modules in `project.currentPackage().moduleIds()` calling `semanticModel.references(symbol)` per module
- Doc references: `ReferencesUtil.findReferencesInDocumentation()` at `ReferencesUtil.java:80` — `DocumentationReferenceFinder` over markdown doc comments
- `ReferencesUtil.getSymbolAtCursor()` at `ReferencesUtil.java:100` — TYPE_PARAMETER/STREAM_TYPE_PARAMS edge case (search at col-1), RHS-of-symbol fallback (col-1 retry)
- Returns `Map<Module, List<Location>>` flattened to `List<Location>`

## Signature help

- Entry: `BallerinaTextDocumentService.signatureHelp()` at `BallerinaTextDocumentService.java:100` — `computeAsync`, `buildSignatureContext()`, delegates to `SignatureHelpUtil.getSignatureHelp()`
- Core: `SignatureHelpUtil.getSignatureHelp()` at `SignatureHelpUtil.java:70` — `fillTokenInfoAtCursor()`, walks parent ladder for invocation node (FUNCTION_CALL, METHOD_CALL, REMOTE_METHOD_CALL_ACTION, CLIENT_RESOURCE_ACCESS_ACTION, IMPLICIT/EXPLICIT_NEW_EXPRESSION)
- Active parameter: `getActiveParamIndex()` at `SignatureHelpUtil.java:140` — counts commas before cursor in the arg list
- `getFunctionSymbol()` at `SignatureHelpUtil.java:200` — dispatch by node kind: FUNCTION_CALL via QNameRef/visible symbols; METHOD_CALL via type-descriptor chain; NEW via `semanticModel.typeOf()` → raw type → class init; CLIENT_RESOURCE_ACCESS via `resourceMethodFilter()`
- `SignatureInfoModelBuilder` — default (compact) + expanded signatures, included-record params, parameter docs
- `resourceMethodFilter()` at `SignatureHelpUtil.java:300` — matches resource path segments (named/path-param/rest-param), type-checks computed segments

## Common patterns

### Context hierarchy

```
DocumentServiceContext (fileUri, filePath, workspace, languageServerContext)
  └── PositionedOperationContext (cursorPosition, cursorPositionInTree, visibleSymbols)
       ├── HoverContext (tokenAtCursor, nodeAtCursor)
       ├── CompletionContext (capabilities)
       │    └── BallerinaCompletionContext (tokenAtCursor, nodeAtCursor, resolverChain, contextType, completionParams)
       ├── SignatureContext (capabilities, nodeAtCursor)
       ├── ReferencesContext (empty marker)
       └── BallerinaDefinitionContext (cursorPosition)
```

- `AbstractDocumentServiceContext` — caches `currentSemanticModel`, `currentDocument`, `currentModule`, `currentDocImportsMap`; `visibleSymbols()` uses `DiagnosticState.VALID` + `REDECLARED`

### Semantic model access chain

All features: `context.currentSemanticModel()` → `workspaceManager.semanticModel(filePath)` → `waitAndGetPackageCompilation()` → `project.currentPackage().getCompilation()` → `getSemanticModel(moduleId)`. Cached per context instance.

### Cancellation pattern

- All async handlers wrap in `CompletableFutures.computeAsync((cancelChecker) -> {...})`
- `context.checkCancelled()` before/after expensive operations; `CancellationException` caught and silently ignored
- `BAL_JAVA_DEBUG` env var disables the cancellation checker (`AbstractDocumentServiceContext.java:60`)

### Error handling pattern

- `UserErrorException` → `clientLogger.notifyUser()`; `CancellationException` → ignored; `Throwable` → `clientLogger.logError()`; all return empty/default values

### URI scheme handling

- `PathUtil.convertUriSchemeFromBala()` — `bala://` → `file://` for the workspace manager
- `PathUtil.getBalaUriForPath()` — file paths → `bala://` for LSP responses
- `LangExtensionDelegator.handleURIScheme()` — checks `file:` or custom schemes
