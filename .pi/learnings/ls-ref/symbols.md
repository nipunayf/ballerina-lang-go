# Document and workspace symbols

- `lsp/symbols.go:documentSymbols` — walks `moduleTopLevelSymbols()` from `module.Package`, returns flat list sorted by position
- `lsp/symbols.go:workspaceSymbols` — fuzzy-match across all modules' exported symbols
- `lsp/symbols.go:fuzzyMatch` — case-insensitive subsequence match for workspace symbols
- `lsp/symbols.go:moduleTopLevelSymbols` — walks `pkg.TypeDefinitions`, `ClassDefinitions`, `GlobalVars`, `Constants`, `Functions`
- `lsp/symbols.go:lspSymbolKind` — maps `model.SymbolKind` to `protocol.SymbolKind`
- `lsp/symbols.go:appendSafeSymbol` — `defer recover()` around `node.Symbol()`
- `lsp/symbols.go:documentSymbols` — `defer recover()` logs panic and returns empty list
- `lsp/symbols.go:workspaceSymbols` — `defer recover()` logs panic and returns nil
- `lsp/symbols.go:workspaceSymbols` — runs frontend up to TopLevelTypeResolved for all modules, fuzzy-matches exported symbols
- `lsp/symbols.go:documentSymbols` — stable sort by position (line, then character)
- `lsp/symbols.go:workspaceSymbols` — stable sort by name, then URI, then line
