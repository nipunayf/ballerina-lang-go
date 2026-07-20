# Code actions

- `lsp/code_action.go:14-50` — missing-import quick-fix, unused-import cleanup, auto-import text edits
- `lsp/code_action.go:missingImportCandidates` — finds qualified refs (`alias:name`) not yet imported, matches against known modules
- `lsp/code_action.go:unusedImportCandidates` — matches `unused import prefix` diagnostics to import lines
- `lsp/code_action.go:importInsertionText` — inserts after last import or before first code line
- `lsp/code_action.go:importLineDeleteRange` — extends range to include newline after import line
- `lsp/code_action.go:diagnosticsForCodeAction` — if client provides no diagnostics, runs fresh diagnostics on reset snapshot
- `lsp/code_action.go:knownImportableModules` — langlib modules + project modules with resolved exports
- `lsp/code_action.go:knownImportableModuleAliases` — same but without public names (for completion)
- `lsp/code_action.go:qualifiedRefFinder` — AST walk collecting `alias:name` refs from SimpleVarRef, Invocation, UserDefinedType
- `lsp/code_action.go:sourceQualifiedRefs` — text scan for `:` patterns outside AST (for recovering parse)
- `lsp/code_action.go:codeActions` — `defer recover()` logs panic and returns nil
