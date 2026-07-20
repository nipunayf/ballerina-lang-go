# Definition and references

- `lsp/definition.go:14-30` — `symbolAtPosition()` walks AST for `ast.NodeWithSymbol`, resolves via `cx.SymbolLocation()`, returns `protocol.Location`
- `lsp/definition.go:definitionLocationAndSymbol` — type-switches on 11 AST node types (SimpleVarRef, LocalVarRef, ConstRef, Invocation, UserDefinedType, Function, ResourceMethod, SimpleVariable, Constant, TypeDefinition, ClassDefinition)
- `lsp/definition.go:safeSymbol` — `defer recover()` around `node.Symbol()` call
- `lsp/definition.go:nodeNameLocation` — uses identifier node position, falls back to parent node position
- `lsp/definition.go:definition` — `defer recover()` logs panic and returns nil
- `lsp/references.go:14-60` — `symbolAtPosition()` + parallel AST walk across candidate modules, dedup by symbol ref
- `lsp/references.go:referenceCandidateModules` — defining module + all modules that import it
- `lsp/references.go:runTopoPrefixFrontend` — runs frontend up to max candidate module index in topo order
- `lsp/references.go:collectReferenceLocations` — goroutine per candidate compilation unit
- `lsp/references.go:sameLocation` — skips definition location when `IncludeDeclaration=false`
- `lsp/references.go:references` — `defer recover()` logs panic and returns nil
