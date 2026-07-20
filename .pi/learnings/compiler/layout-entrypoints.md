# Layout & entry points

## Layout (paths relative to `explore-codebase/`; `ls/` is a mirror)

- `context/` — CompilerContext (per-module), CompilerEnvironment (shared)
- `projects/` — project/package/module/document model, resolution, compilation
- `model/` — symbol types, scopes, symbol spaces, package IDs
- `semantics/` — symbol/type resolution, semantic analysis, CFG
- `ast/` — BLangPackage, BLangCompilationUnit, AST nodes
- `tools/diagnostics/` — Diagnostic, DiagnosticEnv, Location (byte-offset)
- `parser/` — parser, error recovery, red/green tree (`parser/tree/`)
- `semtypes/` — SemType algebra, member-type queries, Context/Env

## Entry points

- `projects.Load(fsys, path)` → `ProjectLoadResult` — `explore-codebase/projects/project_loader.go`
- `Package.Compilation()` → `*PackageCompilation` — `explore-codebase/projects/package_context.go:123`
- `Package.Resolution()` → `*PackageResolution` — `explore-codebase/projects/package_context.go:130`
- `PackageCompilation.DiagnosticResult()` / `.DiagnosticEnv()` — `explore-codebase/projects/package_compilation.go:218`
- `Package/Module/Document.Modify()` — immutable modification chain
- `context.NewCompilerContext(env)`; `context.NewCompilerEnvironment(typeEnv, statsEnabled)`

## Semantics resolvers

- `ResolveCompilationUnitImports(ctx, cus, implicitImports, publicSymbols, orgName)` — `explore-codebase/semantics/symbol_resolver.go:1011`
- `GetImplicitImports(ctx)` (lang.*) — `explore-codebase/semantics/symbol_resolver.go:1089`
- `ResolveSymbols(ctx, pkgID, cuImportsList)` → `(model.Scope, model.ExportedSymbolSpace)` — `explore-codebase/semantics/symbol_resolver.go:580`
- `ResolveTopLevelNodes` — `explore-codebase/semantics/type_resolver.go:574`; `ResolveLocalNodes` — `explore-codebase/semantics/type_resolver.go:625`
- `moduleResolver.resolveModuleLoadRequests(ctx, requests)` — `explore-codebase/projects/module_resolver.go:50`
