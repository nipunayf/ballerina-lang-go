# AST (BLang) & syntax tree (red/green), parser recovery

Keep entries summarized and pointer-dense — `path` + symbol, one line each.

## AST node shapes (ast package)

- `ast.Node` interface — `GetPosition() diagnostics.Location`, `GetDeterminedType() semtypes.SemType` — `explore-codebase/ast/interfaces.go:30-35`
- `ast.BLangNode` interface — extends `Node` with `SetDeterminedType()`, `SetPosition()` — `explore-codebase/ast/ast.go:58-62`
- `setExpectedType(e, ty)` is an alias for `e.SetDeterminedType(ty)` — stores the *resolved* type, not the *expected* type. `semantics/semantic_analyzer.go:2113`
- `ast.NodeWithSymbol` interface — extends `Node` with `Symbol() model.SymbolRef` — `explore-codebase/ast/interfaces.go:35-40`
- `ast.BLangPackage` struct — `Imports`, `XmlnsList`, `Constants`, `GlobalVars`, `Services`, `Functions`, `TypeDefinitions`, `Annotations`, `InitFunction`, `TestablePkgs`, `ClassDefinitions`, `PackageID`, `Scope` — `explore-codebase/ast/ast.go:172-190`
- `ast.BLangCompilationUnit` struct — `TopLevelNodes`, `Name`, `Scope`, `packageID`, `sourceKind` — `explore-codebase/ast/ast.go:165-172`
- `ast.TypeData` struct — `TypeDescriptor TypeDescriptor`, `Type semtypes.SemType` — `explore-codebase/ast/interfaces.go:40-45`
- `ast.TopLevelNode` interface — extends `Node` with `isTopLevel()` — `explore-codebase/ast/interfaces.go:50-55`
- `ast.InvokableNode` interface — `GetName()`, `GetParameters()`, `GetReturnTypeDescriptor()`, `GetBody()`, `GetRestParam()` — `explore-codebase/ast/interfaces.go:150-170`
- `ast.FunctionNode` interface — extends `InvokableNode`, `TopLevelNode` — `explore-codebase/ast/interfaces.go:175-180`
- `ast.ClassDefinition` interface — `GetName()`, `GetMethods()`, `GetMethod(name)`, `GetInitFunction()` — `explore-codebase/ast/interfaces.go:185-195`
- `ast.VariableNode` interface — `GetInitialExpression()`, `GetIsDeclaredWithVar()` — `explore-codebase/ast/interfaces.go:130-140`
- `ast.SimpleVariableNode` interface — extends `VariableNode` with `GetName()`, `SetName()` — `explore-codebase/ast/interfaces.go:140-145`
- `ast.TypeDescriptor` interface — `Node`, `IsGrouped()` — `explore-codebase/ast/interfaces.go:210-215`
- `ast.RecordTypeNode` interface — `GetRestFieldType()`, `GetFields()` — `explore-codebase/ast/interfaces.go:225-230`
- `ast.ObjectType` interface — `Members()`, `Member(name)` — `explore-codebase/ast/interfaces.go:260-265`
- `ast.UnionTypeNode` / `IntersectionTypeNode` interfaces — `Lhs()`, `Rhs()` — `explore-codebase/ast/interfaces.go:270-285`
- `ast.ErrorTypeNode` interface — `TypeDescriptor` — `explore-codebase/ast/interfaces.go:290-295`
- `ast.BLangIdentifier` struct — `Value`, `OriginalValue`, `isLiteral` — `explore-codebase/ast/ast.go:100-105`
- `ast.BLangImportPackage` struct — `OrgName`, `PkgNameComps`, `Alias`, `CompUnit`, `Version` — `explore-codebase/ast/ast.go:110-120`
- `ast.DocumentableNode` interface — `GetMarkdownDocumentationAttachment()`, `SetMarkdownDocumentationAttachment()` — `explore-codebase/ast/interfaces.go:310-315`
- `ast.MarkdownDocumentationNode` interface — `GetDocumentation()`, `GetParameters()`, `GetReturnParameter()`, `GetDeprecationDocumentation()`, `GetReferences()` — `explore-codebase/ast/interfaces.go:320-340`
- `ast.InvocationNode` interface — `GetPackageAlias()`, `GetName()`, `GetArgumentExpressions()`, `GetRequiredArgs()`, `GetExpression()` — `explore-codebase/ast/interfaces.go:200-210`
- `ast.FieldBasedAccessNode` interface — `GetExpression()`, `GetFieldName()` — `explore-codebase/ast/interfaces.go:215-220`
- `ast.SimpleVariableReferenceNode` interface — `GetPackageAlias()`, `GetVariableName()` — `explore-codebase/ast/interfaces.go:225-230`

## Traversal

- `ast.Walk(v Visitor, node BLangNode)` — depth-first traversal over all BLang node types — `ast/walk.go:1-100`
- `ast.Visitor` interface — `Visit(node BLangNode) (w Visitor)`, `VisitTypeData(typeData *TypeData) (w Visitor)` — `ast/walk.go:30-35`
- The visitor is the right tool for finding AST nodes by position/type once you have `*ast.BLangPackage`.

## Positions: red nodes vs AST nodes

- **Red nodes (tree.Node) know position** — `Position() int` (byte offset), `TextRange()`, `LineRange()`, `Location()`. `parser/tree/node.go:50-80`
- **Red nodes have parent references** — `Parent() NonTerminalNode`, `Ancestor(filter)`, `Ancestors()`. `parser/tree/node.go:60-80`
- **Red nodes are rebuilt per keystroke** — comment: "We rebuild these per keystroke." `parser/tree/node.go:30-35`
- **Green nodes (STNode) know width but not position** — `Width() uint16`, `Kind()`, `IsMissing()`. `parser/tree/st_node.go:30-60`
- **AST (BLang) nodes have `GetPosition() diagnostics.Location`** — byte-offset based, file-indexed — but no parent references and no position-to-node query. `ast/interfaces.go:40-45`
- The red-node tree is the right tree for position queries; the AST carries symbols/types. No API maps between them (see gaps.md).

## NodeBuilder: red→BLang transformation

- `NodeBuilder` implements `tree.NodeTransformer[BLangNode]` — transforms red-node syntax tree into BLang AST. `ast/node_builder.go:136`
- `tree.NodeTransformer[T]` interface — `TransformSyntaxNode(node Node) T` dispatches by concrete type; ~200 `Transform*` methods. `parser/tree/node_transformer.go:19-24`
- `NewNodeBuilder(cx)` — strict mode (panics on errors). `ast/node_builder.go:110-113`
- `NewRecoveringNodeBuilder(cx)` — recovery mode (produces `BLangBadNode` types instead of panicking). `ast/node_builder.go:115-117`
- `NodeBuilderMode` — `NodeBuilderModeStrict` (0) / `NodeBuilderModeRecover` (1). `ast/node_builder.go:79-84`
- `NodeBuilder.recovering()` — returns `mode == NodeBuilderModeRecover`. `ast/node_builder.go:133-135`
- `GetCompilationUnit(cx, syntaxTree)` — public entry point: creates `NewNodeBuilder(cx)`, calls `TransformModulePart()`. `ast/ast.go:1542-1546`
- `NodeBuilder` fields: `PackageID`, `cx *context.CompilerContext`, `types typeTable`, `mode`, `reportedSyntaxDiagnostics map`. `ast/node_builder.go:86-98`

### What NodeBuilder requires from CompilerContext

- `cx.DiagnosticEnv()` — for creating `diagnostics.Location` objects via `n.de()`. `ast/node_builder.go:100-108`
- `cx.GetDefaultPackage()` — for `PackageID`. `ast/node_builder.go:122`
- `cx.GetNextAnonymousTypeKey(packageID)` — for anonymous type names. `ast/node_builder.go:380-383`
- `cx.GetNextAnonymousFunctionKey(packageID)` — for anonymous function names. `context/context.go:295-298`
- `cx.SyntaxError(message, pos)` — for reporting syntax errors during AST build. `context/context.go:230-232`
- `cx.NewPackageID(org, nameComps, version)` — for creating package IDs. `context/context.go:225-227`

### What CompilerContext requires from CompilerEnvironment

- `env.DiagnosticEnv()` — shared `*diagnostics.DiagnosticEnv`. `context/env.go:100-102`
- `env.GetDefaultPackage()` — via `packageInterner`. `context/env.go:215-217`
- `env.GetNextAnonymousTypeKey()` / `GetNextAnonymousFunctionKey()` — counter maps. `context/env.go:245-260`
- `env.NewSymbolSpace()`, `NewModuleScope()`, `NewFunctionScope()`, `NewBlockScope()` — scope creation. `context/env.go:108-140`
- `env.GetSymbol(ref)`, `SymbolName()`, `SymbolType()`, `SymbolLocation()`, `SymbolKind()` — symbol queries. `context/env.go:142-190`
- `env.GetTypeEnv()` — `semtypes.Env`. `context/env.go:235-238`
- `env.packageInterner` — `*model.PackageIDInterner` (shared, `DefaultPackageIDInterner`). `context/env.go:28`

### Position handling in NodeBuilder

- `getPosition(node)` — uses `node.TextRange()` (excludes leading trivia). `ast/node_builder.go:155-157`
- `getRecoveryPosition(node)` — uses `node.TextRangeWithMinutiae()` (includes leading trivia, for bad nodes). `ast/node_builder.go:159-161`
- `location(node, textRange)` — creates `diagnostics.NewLocation(de, fileName, start, end)`. `ast/node_builder.go:163-165`
- `getPositionRange(startNode, endNode)` — span from start to end node. `ast/node_builder.go:167-170`
- `getPositionWithoutMetadata(node)` — skips leading metadata child. `ast/node_builder.go:172-174`
- All positions are byte-offset based, file-indexed via `DiagnosticEnv`.

## Red-node statement types (parser/tree/node_gen.go)

- `tree.StatementNode = NonTerminalNode` — `explore-codebase/parser/tree/node_gen.go:346`
- `tree.AssignmentStatementNode` — `VarRef()`, `EqualsToken()`, `Expression()` — `node_gen.go:348-384`
- `tree.CompoundAssignmentStatementNode` — `VarRef()`, `Expression()` — `node_gen.go:384-432`
- `tree.VariableDeclarationNode` — `TypedBindingPattern()`, `EqualsToken()`, `Initializer()` — `node_gen.go:432-484`
- `tree.BlockStatementNode` — `OpenBraceToken()`, `Statements()`, `CloseBraceToken()` — `node_gen.go:484-512`
- `tree.BreakStatementNode` — `BreakToken()`, `SemicolonToken()` — `node_gen.go:512-536`
- `tree.FailStatementNode` — `FailToken()`, `Expression()`, `SemicolonToken()` — `node_gen.go:536-568`
- `tree.ExpressionStatementNode` — `Expression()` — `node_gen.go:568-588`
- `tree.ContinueStatementNode` — `ContinueToken()`, `SemicolonToken()` — `node_gen.go:588-612`
- `tree.IfElseStatementNode` — `IfKeyword()`, `Condition()`, `IfBody()`, `ElseBody()` — `node_gen.go:648-684`
- `tree.ElseBlockNode` — `ElseKeyword()`, `ElseBody()` — `node_gen.go:684-708`
- `tree.WhileStatementNode` — `WhileKeyword()`, `Condition()`, `WhileBody()`, `OnFailClause()` — `node_gen.go:708-748`
- `tree.PanicStatementNode` — `PanicKeyword()`, `Expression()`, `SemicolonToken()` — `node_gen.go:748-780`
- `tree.ReturnStatementNode` — `ReturnKeyword()`, `Expression()`, `SemicolonToken()` — `node_gen.go:780-812`
- `tree.ForEachStatementNode` — `ForeachKeyword()`, `TypedBindingPattern()`, `InKeyword()`, `Collection()`, `BlockStatement()` — `node_gen.go:916-968`
- `tree.LockStatementNode` — `LockKeyword()`, `BlockStatement()` — `node_gen.go:848-880`
- `tree.ForkStatementNode` — `ForkKeyword()`, `BlockStatement()` — `node_gen.go:880-916`
- `tree.LocalTypeDefinitionStatementNode` — `TypeKeyword()`, `TypeName()`, `TypeDescriptor()` — `node_gen.go:812-848`

## Red-node expression types (parser/tree/node_gen.go)

- `tree.ExpressionNode = NonTerminalNode` — `node_gen.go:968`
- `tree.BinaryExpressionNode` — `LhsExpr()`, `Operator()`, `RhsExpr()` — `node_gen.go:970-990`
- `tree.BracedExpressionNode` — `OpenParen()`, `Expression()`, `CloseParen()` — `node_gen.go:990-1018`
- `tree.CheckExpressionNode` — `CheckKeyword()`, `Expression()` — `node_gen.go:1018-1038`
- `tree.FieldAccessExpressionNode` — `Expression()`, `DotToken()`, `FieldName()` — `node_gen.go:1038-1070`
- `tree.FunctionCallExpressionNode` — `FunctionName()`, `OpenParen()`, `Arguments()`, `CloseParen()` — `node_gen.go:1070-1106`
- `tree.MethodCallExpressionNode` — `Expression()`, `DotToken()`, `MethodName()`, `OpenParen()`, `Arguments()`, `CloseParen()` — `node_gen.go:1106-1158`
- `tree.MappingConstructorExpressionNode` — `OpenBrace()`, `Fields()`, `CloseBrace()` — `node_gen.go:1158-1190`
- `tree.IndexedExpressionNode` — `Expression()`, `OpenBracket()`, `Index()`, `CloseBracket()` — `node_gen.go:1190-1226`
- `tree.TypeofExpressionNode` — `TypeofKeyword()`, `Expression()` — `node_gen.go:1226-1250`
- `tree.UnaryExpressionNode` — `Operator()`, `Expression()` — `node_gen.go:1250-1274`
- `tree.TypeTestExpressionNode` — `Expression()`, `IsKeyword()`, `TypeDescriptor()` — `node_gen.go:2264-2292`
- `tree.RemoteMethodCallActionNode` — `Expression()`, `RightArrowToken()`, `MethodName()`, `OpenParen()`, `Arguments()`, `CloseParen()` — `node_gen.go:2294-2346`
- `tree.ListConstructorExpressionNode` — `OpenBracket()`, `Expressions()`, `CloseBracket()` — `node_gen.go:1190-1226` (indexed)
- `tree.NewExpressionNode` — `NewKeyword()`, `TypeDescriptor()`, `OpenParen()`, `Arguments()`, `CloseParen()` — `node_gen.go:1700-1724` (inferred-typedesc)
- `tree.ObjectConstructorExpressionNode` — `ObjectKeyword()`, `Members()` — `node_gen.go:1764-1816`
- `tree.SpecificFieldNode` — `FieldName()`, `ColonToken()`, `ValueExpr()` — `node_gen.go:1566-1602`
- `tree.SpreadFieldNode` — `EllipsisToken()`, `ValueExpr()` — `node_gen.go:1602-1626`
- `tree.NamedArgumentNode` — `Name()`, `EqualsToken()`, `Expression()` — `node_gen.go:1628-1660`
- `tree.PositionalArgumentNode` — `Expression()` — `node_gen.go:1660-1676`
- `tree.RestArgumentNode` — `EllipsisToken()`, `Expression()` — `node_gen.go:1676-1700`

## Parser error recovery

- **Parser has full error recovery** — `BallerinaParserErrorHandler` with `Recover()`, `getResolution()`, `getCompletion()`, `getFailSafeSolution()`. `parser/error_handler.go:1-902`
- **Recovery produces `BLangBadNode` types** — `BLangBadTopLevelNode`, `BLangBadStmt`, `BLangBadExprOrAction`, `BLangBadTypeNode`, `BLangBadIdentifier`. `ast/bad_nodes.go:1-50`
- **Recovery has a completion-specific path** — `getCompletion()` calls `GetInsertSolution()`, own iteration limit (`COMPLETION_ITTER_LIMIT = 15`). `parser/error_handler.go:200-220`
- **Recovery produces missing tokens** — `tree.CreateMissingTokenWithDiagnosticsFromParserRules()`. `parser/error_handler.go:280-285`
- **Recovery is always on** — no flag to disable; the parser always produces a full syntax tree.
