# Design recommendations: a semantic query surface

Accumulated design reasoning for exposing semantic queries to the LS (not yet
built). Grounded in the gaps recorded in `gaps.md`.

1. **The snapshot is the right boundary** — `StableSnapshot` already provides a frozen `*projects.Package` with a known `CompilationKey`. A semantic query surface should hang off `StableSnapshot` (or a new `SemanticSnapshot` wrapper), scoped per `DocumentURI`.

2. **Must not expose compiler internals** — never leak `*context.CompilerContext`, `*ast.BLangPackage`, `*model.SymbolSpace`, or `*projects.moduleContext`. Offer methods like `SymbolsAtPosition`, `ScopeAtPosition`, `TypeAtPosition`, `CompletionsAtPosition`, `HoverAtPosition`, `GoToDefinition`, `References`, `DocumentSymbols`, `Diagnostics`.

3. **The query surface needs the AST, which is private** — options: (a) implement inside `projects/` where `moduleContext` is accessible; (b) walk the public red-node syntax tree (`Document.SyntaxTree()`) which has a different node shape; (c) add a public `PackageCompilation.SemanticQuery(moduleID)` returning a query object implemented inside `projects/`. **(c) is cleanest.**

4. **Needs the CompilerContext** — symbol queries (`GetSymbol`, `SymbolType`, …) require `*context.CompilerContext`; the query object should hold `moduleContext.compilerCtx`.

5. **Needs the DiagnosticEnv** — byte offset → line/col requires `*diagnostics.DiagnosticEnv`, already public via `PackageCompilation.DiagnosticEnv()`.

6. **Must be thread-safe** — the snapshot is read on the LSP goroutine while the compile engine writes new snapshots. Make the query object immutable (capture everything at construction) or use read locks.

7. **Position conversion at the boundary** — compiler uses byte offsets (`diagnostics.Location`); LSP uses UTF-16 line/character. Accept UTF-16 positions and convert internally via `text.TextDocument` (which has UTF-16 conversion methods).

8. **Red-node tree for position queries** — red nodes know position and have parent references and are rebuilt per keystroke; the AST has neither parents nor a position query. `NodeAtPosition(pos)` should walk the red-node tree.

9. **Symbol resolution from a red node is the hard part** — (a) find red node at position; (b) map to the corresponding AST node — **no API exists for this step**; (c) read `Symbol()` if the AST node implements `NodeWithSymbol`; (d) query `CompilerContext` for type/location/kind.

10. **`ast.Walk` is the right AST traversal tool** — visitor over `*ast.BLangPackage` for finding nodes by position/type/criteria.

## Completion-specific design notes

### What the compiler exposes (available from `ls/`)

- **Red-node syntax tree** — `Document.SyntaxTree()` returns `*tree.SyntaxTree` with `RootNode.(*tree.ModulePart)`, position-aware nodes with parent references. `explore-codebase/projects/document.go:50-55`
- **Text content** — `Document.TextDocument()` returns `text.TextDocument` with full text. `explore-codebase/projects/document.go:55-60`
- **Package/module/document hierarchy** — `Project.CurrentPackage()`, `Package.Module(id)`, `Module.Document(id)`. `explore-codebase/projects/package.go:50-80`, `explore-codebase/projects/module.go:50-80`
- **DiagnosticEnv** — `PackageCompilation.DiagnosticEnv()` for offset→line/col. `explore-codebase/projects/package_compilation.go:215-220`
- **StableSnapshot** — provides `Package()`, `Project()`, `Diagnostics()`. `ls/ls/core/compile/snapshot.go:30-60`

### What the compiler does NOT expose (must be built)

- **No cursor-context classification** — must walk red-node tree + analyze tokens to determine if cursor is in import/expression/type-context/statement/field-name position.
- **No visible-symbols-at-position query** — scope chain is private inside `moduleContext`; must be built by walking the red-node tree's parent chain and mapping to AST symbols.
- **No qualifier-resolution API from `ls/`** — `ModuleScope.GetPrefixedSymbol(prefix, name)` exists but is only accessible inside the compiler.
- **No documentation-string query** — docs live on AST `DocumentableNode`; no public API to retrieve docs for a symbol.
- **No type-display-string API** — `semtypes.SemType` has no `String()`; `sem_type_printer_release.go` exists but is internal.
- **No member-completion API** — `semtypes.MappingMemberTypeInnerVal`, `ObjectMemberType`, `ListMemberType` exist but require a `semtypes.Context` (thread-local) and the type value.
- **No import-completion API** — `moduleContext.importedSymbols` is private.
- **No keyword-completion context** — no API to determine which keywords are valid at a given position.

### What the semantic query surface (ticket 14) must provide for completion

1. **Cursor-context classification** — given a position, classify as: import-context, expression-context, type-context, statement-context, module-member-context, field-access-context, etc.
2. **Visible symbols** — given a position and context, return all symbols visible at that point (local scope + enclosing scopes + module scope + imports).
3. **Qualifier resolution** — given a prefix and name, resolve the qualified symbol.
4. **Type/signature display** — given a symbol reference, produce a display string for its type and/or signature.
5. **Documentation** — given a symbol reference, retrieve its documentation string.
6. **Member completion** — given a type, enumerate its members (fields, methods).

### Implementation strategy (from semantic-query-decision.md)

- The semantic projection is built inside `ls/core/compile` and published with each `StableSnapshot`.
- `ls/core/query` remains the only feature-facing facade, composing private primitives.
- The query object holds a snapshot-local opaque symbol key; `model.SymbolRef`, compiler symbols/types, AST nodes, projects, compiler contexts, and parser nodes never cross the facade.
- Position conversion (byte→UTF-16) happens in `ls/server`.
- The initial projection supplies: cursor target/symbol, declaration location, type/signature display, documentation, qualifiers, and visible symbols.

## Expected contextual types — design notes (ticket 20)

### What the compiler exposes

The compiler's `resolveActionOrExpression(t, chain, expr, expectedType)` threads `expectedType` as a local parameter through the type resolver. Each syntactic context derives it differently:

| Context | Expected type source | `type_resolver.go` line |
|---|---|---|
| Assignment RHS | LHS resolved type | `resolveAssignment` line 1527: `resolveActionOrExpression(t, chain, s.GetExpression(), lhsTy)` |
| Variable init (typed) | Type annotation | `resolveVariableDefStmt` line 2889: `expectedType := variable.GetDeterminedType()` (set from type annotation) |
| Variable init (`var`) | None (inferred from init) | `resolveVariableDefStmt` line 2889: zero `SemType` when no type annotation |
| Return expr | Enclosing function return type | `resolveStatementInner` line 1603: `t.expectedReturnType()` |
| Function arg (positional) | Parameter type from signature | `resolveFunctionCallArgs` line 6062: `semtypes.ListMemberTypeInnerVal(cx, paramListTy, key)` |
| Function arg (included record) | Record field type | `resolveIncludedRecordSlot` line 6393 |
| Mapping field value | Field type from mapping type | `resolveMappingConstructorWithExpectedType` line 3771: `mat.FieldInnerVal(keyName)` |
| List member | Member type from list type | `resolveListConstructorWithExpectedType` line 4733: `lat.MemberAtInnerVal(memberIndex)` |
| `new` arg | `init` method param type | `resolveObjectNewExpr` line 3529: `semtypes.ListMemberTypeInnerVal(cx, paramListTy, key)` |
| `new` (implicit) | Intersect expected with OBJECT∪STREAM | `resolveNewExpr` line 3495: `semtypes.Intersect(expectedType, semtypes.Union(semtypes.OBJECT, semtypes.STREAM))` |
| Client resource path | None (resolved bottom-up) | `resolveResourceAccessPathType` line 5984 |
| `if`/`while` condition | `semtypes.BOOLEAN` | `resolveStatementInner` line 1561 |
| `panic` arg | `semtypes.ERROR` | `resolveStatementInner` line 1653 |
| `check` inner | `semtypes.Union(expectedType, semtypes.ERROR)` | `resolveCheckedExpr` line 3047 |

### Expected type is transient, not retained

- `expectedType` is a **local parameter** in `resolveActionOrExpression` — it is NOT stored on the AST node.
- `setExpectedType(e, ty)` calls `e.SetDeterminedType(ty)` which stores the *resolved* type of the expression, not the *expected* type. `semantics/semantic_analyzer.go:2113`.
- After compilation, the expected type must be **re-derived** by walking the AST upward from the cursor position to find the enclosing context.

### Design implications for ticket 20

1. **Build inside `projects/`** where `moduleContext` (and thus the AST) is accessible. The query object holds `moduleContext.compilerCtx` and `moduleContext.bLangPkg`.
2. **Walk the AST** to find the expression at cursor position and its enclosing context (assignment, variable declaration, function call, etc.).
3. **Re-derive the expected type** from the enclosing context's type information (variable type annotation, function parameter type, assignment LHS type, return type, etc.).
4. **Return an opaque DTO** — a `semtypes.SemType` serialized to a display string, or a new `ExpectedTypeDTO{TypeLabel string, Kind string}` that leaks no compiler objects.
5. **Publish with each `StableSnapshot`** as part of the completion index or a new expected-type index. The index is built after full compilation while `moduleContext` still holds its AST.
6. **No request compilation** — the expected type is derived from the already-compiled AST, not by re-parsing or re-compiling.
7. **Thread safety** — the query object is immutable (captures everything at construction from the frozen snapshot).

### What the DTO must NOT leak

- No `*context.CompilerContext`
- No `*ast.BLangPackage` or any AST node
- No `*model.SymbolSpace` or `model.SymbolRef`
- No `*projects.moduleContext`
- No `semtypes.SemType` (opaque compiler type)

### What the DTO SHOULD contain

- `TypeLabel string` — a display-friendly string like "int", "map<string>", "Person"
- `Kind string` — the context kind: "assignment", "variable", "return", "argument", "field", "member", "new", "condition", "panic"
- `IsEmpty bool` — whether no expected type could be determined

### Current state of the completion index

The existing `CompletionIndex` (`projects/completion_index.go`) already demonstrates the pattern: it copies only label/kind/detail facts from the compiled AST, leaks no compiler objects, and is published with each `StableSnapshot`. An `ExpectedTypeIndex` would follow the same pattern.
