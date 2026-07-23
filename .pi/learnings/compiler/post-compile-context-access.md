# Post-compilation context access: what survives, what doesn't

Ticket 36: Can LS completion safely read through compiler context after a
generation completes, to retrieve relevant bound AST/types/scopes/symbols on
demand with no precomputed index and no request-time compilation?

## Executive summary

**No — not through the public API.** The compiler's semantic artifacts (BLang
AST, CompilerContext, symbol scopes, resolved types) are all private to
`moduleContext` and `packageContext`. The public `Package` → `Module` →
`Document` chain exposes only the red-node syntax tree
(`Document.SyntaxTree()`), text content (`TextDocument()`), and metadata (IDs,
names, descriptors). There is no public `AST()`, `BIR()`, `CompilerContext()`,
or `SemanticModel()` method on `Package` or `PackageCompilation`.

The current design bridges this gap with **pre-desugar projections** — the
three completion indices (ExpectedTypeIndex, MemberCompletionIndex,
InvocationCompletionIndex) are built inside `analyzeAndDesugar` (Phase 2)
AFTER local-node resolution but BEFORE desugar, capturing resolver-derived
state that would be lost. These projections are the only way to surface
compiler-derived semantic information to the LS.

## What objects retain what after compilation

### 1. `*projects.Package` (public)

- `PackageID()`, `PackageName()`, `PackageOrg()`, `PackageVersion()`, `Descriptor()`, `Manifest()` — metadata only
- `ModuleIDs()`, `Module(id)`, `Modules()`, `DefaultModule()`, `ModuleByName(name)` — navigation
- `Compilation()` → `*PackageCompilation` — triggers compilation on first call (sync.Once)
- `Resolution()` → `*PackageResolution` — dependency graph
- **No `AST()` method** — the BLang AST is private in `moduleContext.bLangPkg`
- **No `BIR()` method** — the BIR is private in `moduleContext.birPkg`
- **No `CompilerContext()` method** — the compiler context is private in `moduleContext.compilerCtx`
- **No `SymbolScope()` method** — the package/module scope is private in `bLangPkg.Scope`

### 2. `*projects.Module` (public)

- `ModuleID()`, `ModuleName()`, `Descriptor()` — metadata
- `DocumentIDs()`, `TestDocumentIDs()`, `Document(id)` — navigation to documents
- `PackageInstance()` — back to containing package
- **No access to `moduleContext`** — the `moduleCtx` field is private
- **No access to compilation artifacts** — all semantic state is in the private `moduleContext`

### 3. `*projects.Document` (public)

- `DocumentID()`, `Name()` — metadata
- `SyntaxTree()` → `*tree.SyntaxTree` — **the red-node syntax tree** (public, accessible)
- `TextDocument()` → `text.TextDocument` — the source text (public, accessible)
- `Module()` → back to containing module
- **No access to `documentContext`** — the `documentCtx` field is private

### 4. `*projects.PackageCompilation` (public)

- `Resolution()` → `*PackageResolution` — dependency graph
- `DiagnosticResult()` → `DiagnosticResult` — diagnostics
- `DiagnosticEnv()` → `*diagnostics.DiagnosticEnv` — offset→line/col resolution
- `SemanticModel(moduleID)` → `any` — **TODO stub, returns nil** (`ls/projects/package_compilation.go:223-227`)
- `CompletionManager()` → `any` — **TODO stub, returns nil** (`ls/projects/package_compilation.go:243-247`)
- `CodeActionManager()` → `any` — **TODO stub, returns nil** (`ls/projects/package_compilation.go:235-239`)
- **No access to `compilerEnv`** — the `compilerEnv` field is private
- **No access to module contexts** — `rootPackageContext` is private; `moduleContextMap` is private in `packageContext`

### 5. `*context.CompilerContext` (private, inside `moduleContext`)

- `GetSymbol(ref)` → `model.Symbol` — resolves a SymbolRef to its Symbol
- `SymbolName(ref)`, `SymbolType(ref)`, `SymbolKind(ref)`, `SymbolIsPublic(ref)` — symbol queries
- `SymbolPackage(ref)` → `model.PackageIdentifier` — which package a symbol belongs to
- `GetTypeEnv()` → `semtypes.Env` — the type environment
- `ExpectedSlotRecords()` → `[]ExpectedSlotRecord` — resolver-captured expected-type facts
- `Diagnostics()` → `[]diagnostics.Diagnostic` — compilation diagnostics
- **All methods are public on `CompilerContext`**, but the `CompilerContext` itself is only reachable through `moduleContext.compilerCtx` which is **private** (`ls/projects/module_context.go:44`)

### 6. `*ast.BLangPackage` (private, inside `moduleContext`)

- The full BLang AST with resolved types, symbol refs, scopes
- `Scope` field — the `*model.PackageScope` with all module-level symbols
- Only accessible via `moduleContext.getBLangPackage()` which is **package-private** (lowercase)
- After desugar, the AST is **rewritten** — constants/globals may be emptied, field accesses rewritten

### 7. `*context.CompilerEnvironment` (shared, persistent)

- Lives on `Environment.compilerEnv` (`ls/projects/env.go:22`)
- `Environment.CompilerEnvironment()` is **public** (`ls/projects/env.go:47`)
- Holds `symbolSpaces` (all symbol spaces across all modules), `typeEnv`, `underlyingSymbol` (narrowing map)
- `symbolSpaces` is append-only during compilation; after compilation it's stable
- `GetSymbol(ref)` resolves any SymbolRef to its Symbol
- `FindSymbol(pkg, name)` is public but documented as "potentially very slow" (`ls/context/env.go:130-141`)
- **Thread-safe for reads**: `symbolSpacesMu` is RWMutex; `GetSymbol` takes RLock
- **Not thread-safe for writes during reads**: `AddSymbolToSameSpace` takes write lock

## What CAN be read post-compilation from the public surface

### From `Document.SyntaxTree()` (red-node tree)

- Module structure: imports, functions, services, listeners, types, constants, variables, annotations
- Positions: every red node has byte-offset spans
- Text content via `Document.TextDocument().String()`
- **No symbols, no types, no scopes** — the red-node tree is purely syntactic

### From `PackageCompilation.DiagnosticEnv()`

- Offset-to-line/column resolution for any file registered during compilation
- File name resolution from `diagnostics.Location`

### From `PackageCompilation.DiagnosticResult()`

- All compilation diagnostics with locations

### From `Package.Resolution()`

- Dependency graph (module-level and package-level)
- Package descriptors and module descriptors

## What CANNOT be read post-compilation from the public surface

### BLang AST (private)

- `moduleContext.bLangPkg` is private (`ls/projects/module_context.go:43`)
- `moduleContext.getBLangPackage()` is package-private (lowercase `getBLangPackage`)
- No public `Package.AST()` method exists
- The BLang AST is the only tree with resolved types (`GetDeterminedType()`), symbol refs (`RawSymbol`), and scopes (`Scope`)

### CompilerContext (private)

- `moduleContext.compilerCtx` is private (`ls/projects/module_context.go:44`)
- No public accessor on `PackageCompilation` or `Package`
- Contains: symbol resolution, type queries, expected slot records, diagnostics

### Symbol scopes (private)

- `bLangPkg.Scope` is a `*model.PackageScope` on the private BLang AST
- `moduleContext.importedSymbols` is private (`ls/projects/module_context.go:45`)
- No "what symbols are in scope at position X?" query from the public API
- No "what does this module import?" query from the public API

### BIR (private)

- `moduleContext.birPkg` is private (`ls/projects/module_context.go:46`)
- `moduleContext.getBIRPackage()` is package-private

### Environment.publicSymbols (private)

- `Environment.publicSymbols` is private (`ls/projects/env.go:28`)
- The map of compiled package symbol spaces is only accessible during the compile worker
- `Environment.CompilerEnvironment()` is public but doesn't expose `publicSymbols`

## The projection bridge: how the LS currently accesses compiler state

The LS builds **five projections** during compilation, all stored in the
`StableSnapshot` and accessible through the `CompletionLease`:

### Built during compilation (in `realCompilePackage`)

1. **`CompletionIndex`** — module-level facts from red-node tree walking
   (`projects.BuildCompletionIndex(pkg)`)
   - CAN be derived post-compilation from public surface
   - Currently built in `realCompilePackage` for consistency

2. **`ImportCatalog`** — import module/alias-export catalog
   (`projects.BuildImportCatalog(pkg)`)
   - PARTIALLY derivable post-compilation: stdlib and project module lists are
     derivable, but alias exports require `Environment.publicSymbols` (private)

### Built during Phase 2 (inside `analyzeAndDesugar`, pre-desugar)

3. **`ExpectedTypeIndex`** — resolver-captured expected-type facts
   (`buildModuleExpectedTypeIndex(moduleCtx)`)
   - Requires: `moduleCtx.compilerCtx.ExpectedSlotRecords()` (private),
     `moduleCtx.bLangPkg.Scope` (private), `semtypes.ContextFrom(compilerCtx.GetTypeEnv())`
   - **Cannot be derived post-compilation**

4. **`MemberCompletionIndex`** — member-access completion candidates
   (`buildModuleMemberCompletionIndex(moduleCtx)`)
   - Requires: walking `moduleCtx.bLangPkg` (private) with `ast.Walk`,
     reading `GetDeterminedType()` from field-access receivers
   - **Cannot be derived post-compilation**

5. **`InvocationCompletionIndex`** — invocation completion candidates
   (`buildModuleInvocationCompletionIndex(moduleCtx)`)
   - Requires: walking `moduleCtx.bLangPkg` (private) with `ast.Walk`,
     reading `inv.RawSymbol` (resolved during local-node resolution),
     `moduleCtx.compilerCtx.GetSymbol(ref)` (private)
   - **Cannot be derived post-compilation**

## Lifecycle and concurrency constraints

### Compilation lifecycle

1. `Package.Compilation()` triggers `sync.Once` compilation
2. Phase 1 (sequential): parse, AST build, import resolution, symbol resolution, top-level type resolution
3. Phase 2 (parallel per module): local node resolution, semantic analysis, CFG, desugar, BIR
4. After Phase 2: `moduleContext.bLangPkg` holds the **desugared** AST (rewritten)
5. The projections are built **between** local-node resolution and desugar (in `analyzeAndDesugar`)

### What survives after compilation

- `moduleContext.bLangPkg` — the desugared BLang AST (may be rewritten)
- `moduleContext.compilerCtx` — the CompilerContext with all diagnostics, symbol refs, type env
- `moduleContext.importedSymbols` — the import map
- `moduleContext.birPkg` — the generated BIR
- `compilerEnv.symbolSpaces` — all symbol spaces (append-only, stable after compilation)
- `compilerEnv.typeEnv` — the type environment (stable after freeze)

### Thread safety

- `CompilerContext.GetSymbol(ref)` — thread-safe (RLock on `symbolSpacesMu`)
- `CompilerEnvironment.GetSymbol(ref)` — thread-safe (RLock on `symbolSpacesMu`)
- `CompilerEnvironment.symbolSpace(index)` — thread-safe (RLock)
- `SymbolSpace.SymbolAt(index)` — thread-safe (RLock)
- `SymbolSpace.Symbols()` — thread-safe iterator (RLock held during iteration)
- `CompilerContext.ExpectedSlotRecords()` — NOT thread-safe (no lock; must be called after resolution completes)
- `CompilerContext.Diagnostics()` — thread-safe (mutex)
- `CompilerContext.stage` — NOT thread-safe (unprotected field, `ls/context/context.go:38-42`)
- `Environment.publicSymbols` — NOT thread-safe (unprotected map, `ls/projects/env.go:28`)
- `CompilerEnvironment.symbolSpaces` — RWMutex protected; desugar adds init functions concurrently

### Generation pinning

- The `StableSnapshot` holds a `*projects.Package` reference
- The `Package` holds a `*packageContext` which holds `moduleContextMap`
- Each `moduleContext` holds `compilerCtx`, `bLangPkg`, `importedSymbols`, `birPkg`
- These are all **heap-allocated, garbage-collected** — they live as long as the snapshot is retained
- The `CompilerEnvironment` is **shared** across all compilations of the same source root
- `Environment.Duplicate()` shares the same `CompilerEnvironment` (not deep-copied)
- The `CompletionLease` pins the snapshot via lease count, preventing eviction

## Design conclusion: a narrow semantic-handle is not viable from the public surface

A hypothetical "semantic handle" that lets the query layer read bound
AST/types/scopes/symbols on demand would require:

1. **A public `Package.AST()` method** — doesn't exist
2. **A public `Package.CompilerContext()` method** — doesn't exist
3. **A public `PackageCompilation.SemanticModel()`** — TODO stub, returns nil
4. **A public `Module.Scope()` method** — doesn't exist
5. **A public `Module.ImportedSymbols()` method** — doesn't exist

All of these would require either:
- Making `moduleContext` fields public (architectural change)
- Adding facade methods on `Package`/`Module`/`PackageCompilation` that delegate
  to the private `moduleContext` (feasible but not done)
- Exposing the `CompilerEnvironment` directly (already public via
  `Environment.CompilerEnvironment()`, but `publicSymbols` is still private)

### What a minimal viable semantic handle could look like

If the design were to add a narrow semantic handle, it would need:

```go
// On Package or PackageCompilation:
func (p *Package) CompilerContext(moduleID ModuleID) *context.CompilerContext
func (p *Package) BLangPackage(moduleID ModuleID) *ast.BLangPackage
```

These would delegate to the private `moduleContext` fields. The `CompilerContext`
already has public methods for symbol queries, type env access, and expected slot
records. The `BLangPackage` has the full resolved AST with scopes.

**However**, even with these, the query layer would need to:
- Navigate the BLang AST to find the node at cursor position (no `NodeAtPosition` API)
- Walk the scope chain to find visible symbols (scope chain is in the BLang AST)
- Resolve symbol refs to symbols (via `CompilerContext.GetSymbol(ref)`)
- Perform subtype checks (requires `semtypes.Context` created from `GetTypeEnv()`)
- Handle the desugared AST (post-desugar, the AST is rewritten)

The current projection-based design avoids all of these by pre-computing the
relevant facts during compilation and copying only protocol-free data (strings,
integers) into the projections. This is a deliberate architectural choice that
keeps the query layer free of compiler imports.

## Key files and line numbers

| File | Line | Symbol |
|------|------|--------|
| `ls/projects/module_context.go` | 40-47 | `moduleContext` struct (private fields) |
| `ls/projects/module_context.go` | 43 | `bLangPkg *ast.BLangPackage` (private) |
| `ls/projects/module_context.go` | 44 | `compilerCtx *context.CompilerContext` (private) |
| `ls/projects/module_context.go` | 45 | `importedSymbols` (private) |
| `ls/projects/module_context.go` | 46 | `birPkg *bir.BIRPackage` (private) |
| `ls/projects/module_context.go` | 280-283 | `getBLangPackage()` (package-private) |
| `ls/projects/module_context.go` | 287-289 | `getBIRPackage()` (package-private) |
| `ls/projects/package.go` | 28 | `packageCtx *packageContext` (private) |
| `ls/projects/package.go` | 228 | `Compilation()` → `*PackageCompilation` (public) |
| `ls/projects/package_compilation.go` | 223-227 | `SemanticModel()` TODO stub |
| `ls/projects/package_compilation.go` | 235-239 | `CodeActionManager()` TODO stub |
| `ls/projects/package_compilation.go` | 243-247 | `CompletionManager()` TODO stub |
| `ls/projects/package_compilation.go` | 30-37 | `PackageCompilation` struct (private fields) |
| `ls/projects/package_context.go` | 33 | `moduleContextMap` (private) |
| `ls/projects/env.go` | 22-28 | `Environment` struct with `publicSymbols` (private) |
| `ls/projects/env.go` | 47-49 | `CompilerEnvironment()` (public) |
| `ls/context/context.go` | 91-100 | `ExpectedSlotRecord` struct |
| `ls/context/context.go` | 175-178 | `EnableExpectedSlotCapture()` |
| `ls/context/context.go` | 195-198 | `ExpectedSlotRecords()` |
| `ls/context/env.go` | 130-141 | `FindSymbol()` (public, slow) |
| `ls/context/env.go` | 22-35 | `CompilerEnvironment` struct |
| `ls/ls/core/compile/compile.go` | 280-300 | `realCompilePackage` builds all projections |
| `ls/ls/core/compile/compile.go` | 340-370 | `CompletionLease` implementation |
| `ls/ls/core/compile/snapshot.go` | 200-240 | `SnapshotStore.lease` mechanics |
| `ls/projects/expected_type_index.go` | 120-170 | `buildModuleExpectedTypeIndex` |
| `ls/projects/member_completion_index.go` | 130-170 | `buildModuleMemberCompletionIndex` |
| `ls/projects/invocation_completion_index.go` | 200-260 | `buildModuleInvocationCompletionIndex` |
| `ls/projects/completion_index.go` | 80-110 | `BuildCompletionIndex` (post-compile derivable) |
| `ls/projects/import_catalog.go` | 56-70 | `BuildImportCatalog` (partially derivable) |
