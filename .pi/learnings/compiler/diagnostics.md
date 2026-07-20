# Diagnostics: capture, identity, position resolution

Keep entries summarized and pointer-dense — `path` + symbol, one line each.

## Capture & value semantics

- `DiagnosticResult` is a value type — `NewDiagnosticResult(diags)` clones the slice and categorizes by severity. `explore-codebase/projects/diagnostic_result.go:22-48`
- **`CompilerContext.Diagnostics()` returns the raw `[]diagnostics.Diagnostic` slice** — not a copy; callers must clone to retain across mutations. `explore-codebase/context/context.go:103`
- Diagnostics are ephemeral per `PackageCompilation` — they live on `CompilerContext.Diagnostics()` and are collected into `PackageCompilation.diagnosticResult` during `compileModulesInternal()`; no caching across compilations.

## Location model

- `Location` stores `fileIndex int` + `startOffset/endOffset int` — fileIndex is a 1-based index into DiagnosticEnv's file list. `explore-codebase/tools/diagnostics/location.go:17-21`
- `DiagnosticEnv` stores a `text.TextDocument` per registered file — used to resolve byte offsets to line/column. `explore-codebase/tools/diagnostics/diagnostic_env.go:37`
- `DiagnosticEnv.RegisterFile(fileName, doc)` — called during compilation; updates in-place when the same name is re-registered. `explore-codebase/projects/package_compilation.go:87-91`, `explore-codebase/tools/diagnostics/diagnostic_env.go:52-56`
- `DiagnosticEnv` is thread-safe — `mu sync.RWMutex` on file registrations. `explore-codebase/tools/diagnostics/diagnostic_env.go:35-39`

## DiagnosticEnv identity (the central pitfall)

- **`DiagnosticEnv` is a per-CompilerEnvironment singleton** — created once in `NewCompilerEnvironment()`, never replaced. `explore-codebase/context/env.go:37`
- `Environment.Duplicate()` shares the same `*context.CompilerEnvironment` pointer — no deep copy. `explore-codebase/projects/env.go:84`
- `PackageCompilation.DiagnosticEnv()` returns the shared singleton. `explore-codebase/projects/package_compilation.go:218`
- `compileModulesInternal()` registers all module source files into this shared env, before Phase 1. `explore-codebase/projects/package_compilation.go:87-91`
- **Consequence**: two concurrent compilations (e.g. stable + in-progress snapshots) share the same DiagnosticEnv; the second overwrites the first's text documents, corrupting offset→line/col resolution for the first's diagnostics.
- Historical note: `RegisterFile` panicking on same-name/different-TextDocument re-registration is what blocked the ADR-042 modifier chain pre-ticket-09; verify current behavior before relying on either the panic or the in-place update.

## Per-file grouping

- No compiler API returns diagnostics for a single document — `moduleContext.getDiagnostics()` returns all diagnostics for the module (see gaps.md).
- The LS groups per file itself: `extractByFile(pkg, comp, env)` groups by `env.FileName`, resolving documents from the captured immutable package. `ls/ls/core/compile/compile.go:300-340`
