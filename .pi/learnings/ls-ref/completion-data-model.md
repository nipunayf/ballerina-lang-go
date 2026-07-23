# Completion data model: on-demand reads from compiled state

## Research question
Is ls-ref's DATA MODEL (persisted structures post-compile that enable request-time reads) reusable evidence for "read directly from compiled state, no precomputed index," separate from its scheduling (which recompiles per request)?

**Answer: Yes.** The read primitives are pure reads over bound-AST structures. The recompile is a freshness choice for unsaved buffers, not required by the read logic. A pure-read completion design can reuse this data model directly.

## Member-access completion (recv.<cursor>)

- `lsp/completion.go:324-356` — `memberAccessCompletionItemsFromReceiver()`:
  1. **Live walk to find context**: `fieldAccessAtOffset(cu, offset)` (line 336) walks the AST to find `BLangFieldBaseAccess` node containing the cursor
  2. **On-demand type read**: `completionReceiverType(expr)` (line 345) reads type from `expr.GetDeterminedType()` OR `cx.SymbolType(symbol)` — no index, live read
  3. **Member enumeration**: Routes to `mappingMemberCompletionItems()` or `objectMemberCompletionItems()` depending on receiver type

- `lsp/completion.go:435-468` — Member enumeration (live walks, NOT indexed):
  - `mappingMemberCompletionItems()` (435-446): calls `semtypes.ToMappingAtomicType(tyCtx, receiverTy)`, iterates `atomic.Names` directly
  - `objectMemberCompletionItems()` (448-468): calls `semtypes.ToObjectAtomicType()`, iterates `atomic.Names`, queries `semtypes.ObjectMemberKind()` per name to determine field vs method

- **Data model used**: `Module.Package` (bound AST with types attached to nodes), `semtypes.Context` (type environment for querying member info)

## Module-level and context-sensitive declaration completion

- `lsp/completion.go:1304-1343` — `visibleSymbolCompletionItemsWithFilter()` — live scope walk, NOT indexed:
  1. Find nearest scope from node chain (1305-1308)
  2. Walk scope hierarchy from inner to outer (1330-1333, via `addScopeSymbols`)
  3. For each scope, extract symbols from attached `model.SymbolSpace` (1366-1374)
  4. Filter by symbol kind (function, variable, type, etc.) and name prefix

- `lsp/completion.go:1345-1374` — Scope hierarchy traversal:
  - `addScopeSymbols()` type-switches on `BlockScope` (lexical block), `FunctionScope` (function body), `ModuleScope` (module-level), `PackageScope` (package-level)
  - Extracts `SymbolSpace.Symbols()` and calls `space.RefAt(i)` to get individual symbol references
  - Each symbol reference is queried via `cx.SymbolKind()`, `cx.SymbolName()`, `cx.SymbolLocation()` — all live reads from the scope, not from a precomputed index

- `lsp/completion.go:1376-1383` — `nearestScopeInNodeChain()` — walks the node chain to find the first node with a scope, used as the starting point for scope hierarchy walk

- **Data model used**: `Module.Package` (bound AST with scopes attached to nodes), `model.SymbolSpace` (attached to scopes, contains symbol references), `context.CompilerContext` (provides methods to query symbol metadata)

- **Used for**: Module var decl completion (1215-1237), visible variables/functions (1245-1249), type completion (1251-1260), expression completion (299-302)

## Invocation/call completion (argument position)

**NOT IMPLEMENTED.** ls-ref does not have a dedicated argument-position completion handler. Argument positions fall to `completionKindExpression` (line 103) which calls `visibleVariableAndFunctionCompletionItems()` (line 300-302, which filters `visibleSymbolCompletionItems()` to variables and functions).

No expected-type-driven ranking, no argument-slot-specific candidate filtering.

## Expected-type-driven completion

**NOT IMPLEMENTED.** No code path queries the expected type at a value slot to rank/filter candidates. The only type-related logic is:
- `completionReceiverType()` (385-394) — get the type of a receiver expression for member access
- `completionKindType` context (251-278) — offer type symbols when cursor is in a type position

No query for "what type does the context expect here?"

## Persisted data structures post-compile

- `Module.Package` (`*ast.BLangPackage`) — bound and type-checked AST
  - Nodes carry scopes via `ast.NodeWithScope` interface (completion.go:1378)
  - Expressions carry type via `GetDeterminedType()` (completion.go:345-386)
  - Each scope (`model.Scope` — `BlockScope`, `FunctionScope`, `ModuleScope`, `PackageScope`) carries a `SymbolSpace` with symbol references
  - `model.SymbolSpace.Symbols()` returns all symbols in the space; `RefAt(i)` returns individual `SymbolRef`

- `Module.CompilationUnits` (`map[DocumentURI]*ast.BLangCompilationUnit`) — recovered (syntactically fixed) ASTs with scopes attached
  - Used when a recovered CU exists (line 332-334, fallback if `Module.Package` is nil)

- `Module.ImportedSymbols` (`map[string]model.ExportedSymbolSpace`) — exported symbols from imported modules, keyed by import alias

- `Module.Exported` (`model.ExportedSymbolSpace`) — symbols exported by this module

- **No precomputed indices**: No member-access index, no invocation index, no expected-type map. All queries are live walks or symbol-space lookups.

## Compilation stages relevant to completion

- `lsp/diagnostics.go:77` — Background diagnostics for non-changed modules in build projects reaches only `FrontendStageTopLevelTypeResolved`
- `lsp/completion.go:330` — Member-access completion requires `FrontendStageLocalTypeResolved` (more advanced)

**Design gap for pure-read completion**: If the background pipeline stops at `TopLevelTypeResolved` but a completion request needs `LocalTypeResolved`, a pure-read design must either:
1. Upgrade the background pipeline to always resolve to `LocalTypeResolved`, or
2. Accept that member-access completion may work with incomplete type information until the next event forces a recompile

## Recompile vs. read: the scheduling choice

- `lsp/completion.go:1119-1156` — `snapshotWithRecoveredCU()` injects a recovered CU and resets `Stage=FrontendStageNone`, `Package=nil`, `ImportedSymbols=nil`, etc.
- Then `runModuleFrontend(cx, snapshot, module, stage)` recompiles from scratch
- **But**: The read code (member access, scope walks, symbol queries) is unchanged — it would work identically against an already-resolved persisted `Module` state from the background pipeline
- `lsp/diagnostics.go:110` — `runModuleFrontend` has an early-return `if module.Stage >= target { return true }`, meaning already-resolved state is reused if the pipeline has resolved far enough
- **Conclusion**: The recompile is a FRESHNESS CHOICE for the unsaved buffer, not a requirement of the read primitives

## Assessment

**Data model is reusable**: The read pattern (scope hierarchy + symbol space + types on expressions) directly supports on-demand completion without precomputed indices. To port to a background-pipeline design:
1. Keep the persisted `Package` (bound AST) and attached scopes/types
2. Keep the read logic: `visibleSymbolCompletionItemsWithFilter`, `memberAccessCompletionItems`, etc.
3. Replace `snapshotWithRecoveredCU` + `runModuleFrontend` with a read from the background-compiled `Module.Package`
4. Ensure the background pipeline resolves to at least `LocalTypeResolved` for all modules (not just `TopLevelTypeResolved`)

**Scheduling difference noted**: ls-ref recompiles per request; the target will read from already-compiled state. This is orthogonal to the data model.

**Gaps to avoid**: Don't expect ls-ref's completion to handle argument-position or expected-type ranking — those are unimplemented and would require additional scope/type analysis.
