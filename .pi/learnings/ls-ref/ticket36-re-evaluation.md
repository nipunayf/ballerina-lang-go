# Ticket 36 Re-evaluation: ls-ref Completion Architecture vs Current Worktree

> **Status**: Verified against actual code in `ls-ref/lsp/` (PoC) and `ls/ls/` (current worktree).
> **Date**: 2026-07-23
> **Scope**: Completion prerequisites, snapshot construction, AST/type/scope retention, request-time frontend work, member/invocation/expected-type coverage.

---

## 1. PoC Completion Prerequisites (ls-ref)

The PoC in `ls-ref/lsp/completion.go` requires the following prerequisites to produce completion items:

### 1a. Snapshot Construction

**File**: `ls-ref/lsp/snapshot.go` (lines 1-200)

The PoC builds snapshots in two modes:
- **Single-file**: `newSingleFileSnapshot()` (line ~80) — creates a minimal snapshot with one module, one file, a fresh compiler environment.
- **Build project**: `newBuildSnapshot()` (line ~120) — scans the project directory for `.bal` files, reads `Ballerina.toml`, creates modules, reuses old module state when possible.

**Key prerequisite**: A `Snapshot` must exist with:
- `Files` map (URI → SourceFile with content)
- `Modules` map (name → Module with Files, CompilationUnits, Stage)
- `Env` (CompilerEnvironment with type env)
- `TopoOrder` (module topological order)

### 1b. AST Retention (Recovering Compilation Unit)

**File**: `ls-ref/lsp/completion.go` (lines ~100-110)

```go
func recoveringCompilationUnit(cx *context.CompilerContext, module *Module, source SourceFile) *ast.BLangCompilationUnit {
    syntaxTree, err := parser.GetSyntaxTree(cx, source.File, source.Content)
    if err != nil || syntaxTree == nil {
        return nil
    }
    builder := ast.NewRecoveringNodeBuilder(cx)
    builder.PackageID = module.PackageID
    compilationUnit := builder.TransformModulePart(syntaxTree.RootNode.(*tree.ModulePart)).(*ast.BLangCompilationUnit)
    compilationUnit.SetPackageID(module.PackageID)
    return compilationUnit
}
```

**Prerequisite**: A fresh parse + AST build from current source content. The PoC does **not** reuse cached compilation units for completion — it always re-parses with `recoveringCompilationUnit()` (line ~100). This is a **prototype shortcut**: it re-parses on every completion request instead of reading from the module's cached `CompilationUnits`.

### 1c. Type/Scope Retention (Frontend Stages)

**File**: `ls-ref/lsp/diagnostics.go` (lines ~80-200)

The PoC runs the compiler frontend in stages:
1. `FrontendStageParsed` — parse all files
2. `FrontendStageSymbolResolved` — resolve imports and symbols
3. `FrontendStageTopLevelTypeResolved` — resolve top-level type definitions
4. `FrontendStageLocalTypeResolved` — resolve local variable types
5. `FrontendStageSemanticAnalyzed` — semantic analysis
6. `FrontendStageCFGBuilt` — build control flow graph
7. `FrontendStageCFGAnalyzed` — analyze CFG

**For completion**, the PoC runs up to `FrontendStageLocalTypeResolved` (line ~170 in `generalCompletionItems`):
```go
_ = runModuleFrontend(cx, completionSnapshot, completionModule, FrontendStageLocalTypeResolved)
```

This means:
- **Symbol resolution** is required (scope hierarchy with `model.Scope`/`model.SymbolSpace`)
- **Top-level type resolution** is required (type definitions resolved)
- **Local type resolution** is required (local variable types resolved)
- **Semantic analysis, CFG, CFG analysis** are NOT required for completion

### 1d. Request-time Frontend Work

**File**: `ls-ref/lsp/completion.go` (lines ~150-200)

At request time, the PoC:
1. Gets the snapshot for the URI (`snapshotForURI`, line ~50)
2. Gets the module for the source (`moduleForSource`, line ~55)
3. Creates a fresh `CompilerContext` (line ~60)
4. Builds a recovering compilation unit (re-parse, line ~65)
5. Creates a **completion snapshot** with the recovered CU injected (`snapshotWithRecoveredCU`, line ~170)
6. Runs the frontend on the completion snapshot up to `FrontendStageLocalTypeResolved` (line ~175)
7. Walks the scope hierarchy for visible symbols (line ~180+)

**This is a prototype shortcut**: The PoC creates a full copy of the snapshot with the recovered CU injected, then re-runs the frontend. This is expensive but works because the PoC's modules are small.

### 1e. Member Access Completion

**File**: `ls-ref/lsp/completion.go` (lines ~200-280)

Member access completion requires:
1. A `BLangFieldBaseAccess` node at the cursor offset (found via `fieldAccessAtOffset`, line ~230)
2. The receiver expression's determined type (`completionReceiverType`, line ~250)
3. Semtype subtype checks against `MAPPING` and `OBJECT` (lines ~260-270)
4. Enumeration of mapping/object member names from the semtype atomic type

**Prerequisite**: Local type resolution must have run so `expr.GetDeterminedType()` is populated.

### 1f. Imported Symbol Completion

**File**: `ls-ref/lsp/completion.go` (lines ~300-350)

Imported symbol completion requires:
1. Topological sort of modules (`dispatchTopoSort`, line ~310)
2. Import resolution for the target module (`runModuleFrontend` up to `FrontendStageTopLevelTypeResolved`, line ~320)
3. Access to the module's `Exported` symbol space (line ~330)
4. For external (langlib) modules: `langlib.Build()` to get public symbols (line ~340)

### 1g. Expected-Type Completion

**The PoC does NOT implement expected-type completion.** There is no `ExpectedTypeIndex`, no slot-based expected-type projection, and no precomputed assignability tiers. This is a **major gap** in the PoC.

### 1h. Invocation Completion

**The PoC does NOT implement invocation completion.** There is no `InvocationCompletionIndex`, no callable catalog, no argument relevance tiers, and no named-argument completion. This is a **major gap** in the PoC.

---

## 2. Current Worktree Status (ls/ls/)

The current worktree in `ls/ls/` has already implemented a significantly more sophisticated architecture:

### 2a. Architecture Split

The current worktree splits the PoC's flat `lsp/` package into:
- `ls/ls/core/query/` — query-layer (completion, symbols, etc.)
- `ls/ls/core/compile/` — compilation engine with dual-snapshot store
- `ls/ls/core/workspace/` — workspace/project management
- `ls/ls/server/` — LSP protocol boundary adapter
- `ls/projects/` — index builders (completion, expected-type, member, invocation, import catalog)

### 2b. Five Completion Indexes (Already Built)

The current worktree already has all five indexes that the PoC lacked:

1. **`projects.CompletionIndex`** (`ls/projects/completion_index.go`) — module-level declaration facts
2. **`projects.ExpectedTypeIndex`** (`ls/projects/expected_type_index.go`) — contextual expected-type projection
3. **`projects.MemberCompletionIndex`** (`ls/projects/member_completion_index.go`) — member-access projection
4. **`projects.InvocationCompletionIndex`** (`ls/projects/invocation_completion_index.go`) — callable catalog + argument relevance
5. **`projects.ImportCatalog`** (`ls/projects/import_catalog.go`) — importable modules + alias exports

### 2c. Non-Blocking Lease Mechanism (Already Built)

**File**: `ls/ls/core/compile/compile.go` (lines ~280-320)

```go
func (s *CompilationService) Lease(root string, generation uint64) (CompletionLease, bool) {
    snap, release, ok := s.store.lease(root, generation)
    if !ok {
        return CompletionLease{}, false
    }
    return CompletionLease{...indexes from snap..., release: release}, true
}
```

The lease mechanism with generation matching, reference-counted pinning, and non-blocking acquisition is already implemented.

### 2d. Fallback-First Completion (Already Built)

**File**: `ls/ls/core/query/completion.go` (lines ~200-280)

The query layer always computes current-syntax and static keyword candidates first, then enriches with semantic facts from a lease. If the lease fails, the fallback is still a valid completion list.

### 2e. Cursor Classification (Already Built)

**File**: `ls/ls/core/query/completion.go` (lines ~100-180)

The current worktree classifies cursor context into:
- `contextImport` — inside an import declaration
- `contextFunctionBody` — inside a function body
- `contextModulePart` — at module-part declaration position
- `contextAliasMember` — at an alias-qualified member access

### 2f. Scope Collection (Already Built)

**File**: `ls/ls/core/query/completion_body.go` (lines ~200-280)

The current worktree collects shadowing-aware scope (parameters + preceding locals) from the syntax tree, without needing compiler symbol resolution.

### 2g. Member Access Completion (Already Built)

**File**: `ls/ls/core/query/completion.go` (lines ~300-360)

The current worktree implements `completeMemberAccess` which reads from a generation-matched `MemberCompletionIndex` lease.

### 2h. Invocation Completion (Already Built)

**File**: `ls/ls/core/query/completion_invocation.go` (lines 1-200)

The current worktree implements:
- `enrichCallableSnippets` — sets snippet invocation forms
- `boostInvocationTiers` — applies precomputed argument relevance tiers
- `namedArgsFromIndex` — derives named-argument candidates

### 2i. Expected-Type Completion (Already Built)

**File**: `ls/ls/core/query/completion.go` (lines ~280-300)

The current worktree implements `boostExpectedCompatible` which lowers the rank of precomputed-compatible candidates.

---

## 3. What the PoC Has That the Current Worktree May Still Need

### 3a. Recovering AST Builder for Cursor Classification

**File**: `ls-ref/lsp/completion.go` (lines ~100-110)

The PoC uses `ast.NewRecoveringNodeBuilder` to build a recovering AST from the parse tree. The current worktree has `completion_ast.go` which implements `classifyContextAST` using the recovering AST via `env.ScopedSyntaxEnv().RecoveringAST(st)`.

**Status**: Already implemented in `ls/ls/core/query/completion_ast.go`.

### 3b. Node Chain at Offset

**File**: `ls-ref/lsp/completion.go` (lines ~400-420)

The PoC uses `nodeChainAtOffset` to find the AST node chain at the cursor offset. This is used for context classification.

**Status**: The current worktree uses a different approach — it classifies context from the syntax tree directly (`classifyCompletion` in `completion.go`) and from the recovering AST (`classifyContextAST` in `completion_ast.go`). The node chain approach is not needed.

### 3c. Bad Node Detection

**File**: `ls-ref/lsp/completion.go` (lines ~430-460)

The PoC uses `badCompletionKindAtOffset` to detect `BLangBadStmt`, `BLangBadExprOrAction`, and `BLangBadIdentifier` nodes for recovery-aware completion.

**Status**: The current worktree handles recovery differently — it uses the syntax tree's recovery nodes directly (e.g., `classifyBodyPosition` in `completion_body.go`).

### 3d. Record Type Descriptor Completion

**File**: `ls-ref/lsp/completion.go` (lines ~500-520)

The PoC offers `inclusive record` and `exclusive record` snippets when the user types `rec` in a type definition context.

**Status**: The current worktree's `modulePartItems` in `completion_module.go` offers `type` as a snippet but does not specifically detect the record type descriptor context. This may be a gap.

### 3e. Record Field Completion

**File**: `ls-ref/lsp/completion.go` (lines ~530-540)

The PoC offers `required field` and `optional field` snippets inside a record body.

**Status**: The current worktree does not appear to have record field completion. This is a gap.

### 3f. Function Return Type Descriptor Completion

**File**: `ls-ref/lsp/completion.go` (lines ~550-570)

The PoC offers a `returns` snippet when the cursor is between the parameter list and the function body (before any explicit return type).

**Status**: The current worktree does not appear to have return type descriptor completion. This is a gap.

### 3g. Module Variable Declaration Completion

**File**: `ls-ref/lsp/completion.go` (lines ~580-600)

The PoC offers declaration snippets (`const`, `function`, `type`, `var`, `variable decl`) at the module level, filtered to types only when a prefix is typed.

**Status**: The current worktree's `modulePartItems` in `completion_module.go` offers similar snippets (`function`, `type`, `const`, `class`, `enum`, `listener`, `final`, `public`, `private`, `import`, `main`). This is already covered.

### 3h. Statement Begin Completion (Loop/Function Gating)

**File**: `ls-ref/lsp/completion.go` (lines ~600-650)

The PoC gates `break`/`continue` to loop bodies and `return` to function bodies.

**Status**: The current worktree's `bodyCatalogItems` in `completion_body.go` implements the same gating with `loopOnly` and `elseFollowOn` flags. This is already covered.

### 3i. Auto-Import Completion

**File**: `ls-ref/lsp/completion.go` (lines ~700-750)

The PoC offers auto-import candidates for unimported modules referenced by alias.

**Status**: The current worktree's `autoImportItems` in `completion_module.go` implements the same pattern with alias collision handling (`firstFreeAlias`). This is already covered.

---

## 4. Prototype Shortcuts in the PoC (Do NOT Copy)

### 4a. Re-parsing on Every Completion Request

**File**: `ls-ref/lsp/completion.go` (line ~100)

The PoC calls `recoveringCompilationUnit()` on every completion request, which re-parses the source and rebuilds the AST. The current worktree reads from the workspace's syntax tree directly, avoiding re-parsing.

### 4b. Full Snapshot Copy for Completion

**File**: `ls-ref/lsp/completion.go` (line ~170)

The PoC creates a full copy of the snapshot with the recovered CU injected (`snapshotWithRecoveredCU`), then re-runs the frontend. The current worktree uses the non-blocking lease mechanism instead.

### 4c. No Expected-Type or Invocation Completion

The PoC has no expected-type or invocation completion. The current worktree already has both.

### 4d. Panic Recovery as Primary Error Handling

**File**: `ls-ref/lsp/completion.go` (line ~30)

The PoC uses `defer/recover` as the primary error handling mechanism. The current worktree uses proper error handling.

### 4e. No Incremental Index Updates

The PoC rebuilds all indexes from scratch every compile cycle. The current worktree has the same limitation (indexes are built in `realCompilePackage`), but the lease mechanism makes this acceptable since indexes are only read when a matching generation exists.

---

## 6. Verified PoC Code Pointers for Ticket 36

### 6a. PoC re-parses on every completion request (prototype shortcut)

**File**: `ls-ref/lsp/completion.go:100-110`
```go
func recoveringCompilationUnit(cx *context.CompilerContext, module *Module, source SourceFile) *ast.BLangCompilationUnit {
    syntaxTree, err := parser.GetSyntaxTree(cx, source.File, source.Content)
    if err != nil || syntaxTree == nil {
        return nil
    }
    builder := ast.NewRecoveringNodeBuilder(cx)
    builder.PackageID = module.PackageID
    compilationUnit := builder.TransformModulePart(syntaxTree.RootNode.(*tree.ModulePart)).(*ast.BLangCompilationUnit)
    compilationUnit.SetPackageID(module.PackageID)
    return compilationUnit
}
```
Every completion request calls `parser.GetSyntaxTree` + `ast.NewRecoveringNodeBuilder` — no cached compilation units are reused.

### 6b. PoC creates a full snapshot copy per request (prototype shortcut)

**File**: `ls-ref/lsp/completion.go:1119-1156` — `snapshotWithRecoveredCU()` shallow-copies all modules, deep-copies the `CompilationUnits` map, resets the target module's `Stage` to `FrontendStageNone`, clears all frontend-derived state, then re-runs `runModuleFrontend` synchronously.

### 6c. PoC uses live AST walks + scope hierarchy for symbol visibility (production pattern for live reads)

**File**: `ls-ref/lsp/completion.go:1304-1343` — `visibleSymbolCompletionItemsWithFilter()` walks the scope hierarchy from inner to outer, extracting symbols from `model.SymbolSpace` attached to scopes. No precomputed index.

**File**: `ls-ref/lsp/completion.go:1345-1374` — `addScopeSymbols()` type-switches on `BlockScope`, `FunctionScope`, `ModuleScope`, `PackageScope` and extracts `SymbolSpace.Symbols()`.

### 6d. PoC member access uses semtype subtype checks at request time (production pattern for live reads)

**File**: `ls-ref/lsp/completion.go:324-356` — `memberAccessCompletionItemsFromReceiver()`:
1. Calls `fieldAccessAtOffset(cu, offset)` to find the `BLangFieldBaseAccess` node (line 336)
2. Calls `completionReceiverType(expr)` which reads `expr.GetDeterminedType()` or `cx.SymbolType(symbol)` (line 345)
3. Does `semtypes.IsSubtype(tyCtx, receiverTy, semtypes.MAPPING)` or `semtypes.IsSubtype(tyCtx, receiverTy, semtypes.OBJECT)` at request time (lines 350-355)
4. Enumerates member names from the semtype atomic type (lines 435-468)

### 6e. PoC has no expected-type or invocation completion (gap)

Confirmed: no `ExpectedTypeIndex`, no `InvocationCompletionIndex`, no slot-based expected-type projection, no callable catalog, no argument relevance tiers, no named-argument completion in `ls-ref/lsp/completion.go`.

### 6f. PoC uses `defer/recover` as primary error handling (prototype shortcut)

**File**: `ls-ref/lsp/completion.go:30-35` — `defer func() { if recovered := recover(); recovered != nil { logLS(...) } }()` wraps the entire completion handler.

### 6g. PoC frontend stages for completion

**File**: `ls-ref/lsp/completion.go:267` — `generalCompletionItems()` calls `runModuleFrontend(cx, completionSnapshot, completionModule, FrontendStageLocalTypeResolved)`.

**File**: `ls-ref/lsp/completion.go:330` — `memberAccessCompletionItemsFromReceiver()` also calls `runModuleFrontend(cx, completionSnapshot, completionModule, FrontendStageLocalTypeResolved)`.

**File**: `ls-ref/lsp/diagnostics.go:109-111` — `runModuleFrontend` has an early-return `if module.Stage >= target { return true }`, meaning already-resolved state is reused if the pipeline has resolved far enough.

### 6h. PoC completion context classification (12+ node types)

**File**: `ls-ref/lsp/completion.go:586-650` — `completionContextAtChainNode()` type-switches on:
- `BLangCompilationUnit` → `completionKindModuleVarDecl`
- `BLangFunction`/`BLangResourceMethod` → `invokableCompletionContext`
- `BLangFunctionType`/`BMethodDecl` → `functionTypeCompletionContext`
- `BLangTypeDefinition` → `completionKindRecordTypeDesc`
- `BLangRecordType` → `completionKindType` or `completionKindRecordField`
- `BLangFieldBaseAccess` → `completionKindMemberAccess`
- `BLangSimpleVariable` → `completionKindType` or `completionKindExpression`
- `BLangExpressionStmt` → `statementBeginCompletionContext`
- `BLangSimpleVarRef`/`BLangInvocation` → `completionKindImportedSymbol`
- `BLangBlockFunctionBody`/`BLangBlockStmt` → `statementBeginCompletionContext`

### 6i. PoC auto-import completion

**File**: `ls-ref/lsp/completion.go:470-510` — `autoImportModuleCompletionItems()`:
1. Gets `knownImportableModuleAliases` from the snapshot
2. Filters out already-imported aliases
3. For each unimported alias, creates a completion item with `AdditionalTextEdits` that inserts the import declaration
4. Uses `importCompletionTextEdit` to compute the insert position and text

### 6j. PoC record type descriptor and record field completion (gap in current worktree)

**File**: `ls-ref/lsp/completion.go:500-520` — `recordTypeDescriptorCompletionItems()` offers `inclusive record` and `exclusive record` snippets.

**File**: `ls-ref/lsp/completion.go:530-540` — `recordFieldCompletionItems()` offers `required field` and `optional field` snippets.

**File**: `ls-ref/lsp/completion.go:694-704` — `isRecordTypeDescriptorCompletionContext()` detects `type X rec` pattern.

**File**: `ls-ref/lsp/completion.go:706-718` — `recordTypeCompletionContext()` detects cursor inside a record body.

## 7. Assessment for Ticket 36

### What the PoC proves about live reads

The PoC demonstrates that completion can work without precomputed indexes by:
1. Walking the scope hierarchy at request time (`visibleSymbolCompletionItemsWithFilter`, line 1304)
2. Reading `expr.GetDeterminedType()` from the compiled AST at request time (line 345)
3. Doing semtype subtype checks at request time (lines 350-355)
4. Enumerating member names from semtype atomic types at request time (lines 435-468)

**Prerequisite**: The module must be compiled to at least `FrontendStageLocalTypeResolved` for these reads to work. The PoC achieves this by re-compiling synchronously on the request goroutine.

### What the current worktree already has

The current worktree (`ls/ls/`) has already moved beyond the PoC with:
1. Non-blocking lease mechanism (`ls/ls/core/compile/compile.go:Lease()`)
2. Five precomputed indexes built during compile (`realCompilePackage()`)
3. Fallback-first completion (`completeFunctionBody()`)
4. Protocol-free boundary with copied DTOs (`CompletionItem`)
5. Generation-matched lease with pinning (`SnapshotStore.lease()`)

### The trade-off for ticket 36

| Approach | Pros | Cons |
|----------|------|------|
| **Drop indexes, use live reads** (PoC pattern) | No index build cost; simpler architecture | Requires module compiled to `LocalTypeResolved`; O(n) scope walk per request; semtype subtype checks at request time |
| **Keep indexes, make cheaper** (current pattern) | O(1) slot lookup; non-blocking lease; fallback-first | Index build is O(n) in AST nodes; built unconditionally every compile cycle |

**Recommendation**: The PoC proves live reads are feasible, but the current worktree's lease-based index architecture is more production-ready. If the index build cost is a concern, the PoC's live-read approach is a valid alternative — but the current worktree would need to ensure the background pipeline compiles to `FrontendStageLocalTypeResolved` for all modules (not just the changed one).
