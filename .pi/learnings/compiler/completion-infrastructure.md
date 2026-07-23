# Completion infrastructure (compiler-side & LS-side)

Keep entries summarized and pointer-dense — `path` + symbol, one line each.

## Compiler-side: expected-type capture

- `context.ExpectedSlotKind` enum — `ExpectedSlotAssignment`, `ExpectedSlotReturn`, `ExpectedSlotCondition`, `ExpectedSlotPanic`, `ExpectedSlotCheck`, `ExpectedSlotArgument`, `ExpectedSlotNamedArgument`, `ExpectedSlotMappingField`, `ExpectedSlotListMember`, `ExpectedSlotNewArg` — `ls/context/context.go:65-90`
- `context.ExpectedSlotRecord` struct — `FileKey`, `StartOffset`, `EndOffset`, `ArgIndex`, `Kind`, `Expected semtypes.SemType` — `ls/context/context.go:97-110`
- `CompilerContext.RecordExpectedSlot(rec)` — appends a resolver-derived expected-type fact; no-op when capture disabled. Thread-safe via `slotMu`. `ls/context/context.go:297-305`
- `CompilerContext.ExpectedSlotRecords()` — returns captured facts in derivation order. `ls/context/context.go:310-315`
- `CompilerContext.CaptureExpectedSlots()` / `SetCaptureExpectedSlots(bool)` — gate for LS build path. `ls/context/context.go:280-290`
- `recordExpectedSlot(t, expr, kind, expectedType, argIndex)` — called from `resolveActionOrExpression` in type resolver. Resolves file key from `DiagnosticEnv.FileName(loc)`. `ls/semantics/type_resolver.go:3201-3230`

## Compiler-side: completion index (projects package)

- `projects.CompletionFactKind` — `CompletionFactFunction`, `CompletionFactModuleVar`, `CompletionFactConstant`, `CompletionFactType` — `ls/projects/completion_index.go:28-35`
- `projects.CompletionFact` — `Label`, `Kind`, `Detail` — copied, protocol-free fact. `ls/projects/completion_index.go:40-45`
- `projects.CompletionIndex` — `moduleFacts map[string][]CompletionFact`, `fileModule map[string]string` — immutable, built after full compilation. `ls/projects/completion_index.go:54-60`
- `CompletionIndex.Facts(fileKey, offset)` — returns module-level facts for a document. `ls/projects/completion_index.go:65-70`
- `BuildCompletionIndex(pkg)` — walks syntax trees, extracts facts from top-level members. `ls/projects/completion_index.go:80-110`
- `completionFactForNode(node, text)` — extracts fact from `FunctionDefinition`, `TypeDefinitionNode`, `ConstantDeclarationNode`, `ModuleVariableDeclarationNode`, `EnumDeclarationNode`, `ClassDefinitionNode`. `ls/projects/completion_index.go:115-160`

## Compiler-side: expected-type index (projects package)

- `projects.ExpectedSlotKind` — mirrors `context.ExpectedSlotKind` without exposing it. `ls/projects/expected_type_index.go:28-48`
- `projects.ExpectedTypeFact` — `Kind`, `Known`, `TypeLabel`, `StartOffset`, `EndOffset`, `ArgIndex`, `Compatible []string` — copied, protocol-free. `ls/projects/expected_type_index.go:51-65`
- `projects.ExpectedTypeIndex` — `facts map[string][]ExpectedTypeFact` — immutable, built after compilation. `ls/projects/expected_type_index.go:68-72`
- `ExpectedTypeIndex.FactAt(fileKey, offset)` — returns innermost (smallest-span) fact containing offset. `ls/projects/expected_type_index.go:77-90`
- `BuildExpectedTypeIndex(comp)` — reads `moduleContext.expectedTypeIndex` from root package's modules. `ls/projects/expected_type_index.go:95-115`
- `buildModuleExpectedTypeIndex(moduleCtx)` — reads `compilerCtx.ExpectedSlotRecords()`, computes display-safe type labels and compatible candidates. `ls/projects/expected_type_index.go:120-170`
- `moduleValueCandidates(pkg, cc)` — enumerates module-level variables and constants with resolved types. `ls/projects/expected_type_index.go:175-200`
- `compatibleCandidates(cx, candidates, expected)` — subtype check at projection-build time. `ls/projects/expected_type_index.go:205-215`
- `moduleMainSpaces(scope)` — extracts `*model.SymbolSpace` from `PackageScope` or `ModuleScope`. `ls/projects/expected_type_index.go:220-230`

## Compiler-side: import catalog (projects package)

- `projects.CatalogModule` — `Org`, `ModuleName` — `ls/projects/import_catalog.go:35-40`
- `projects.AliasExport` — `Alias`, `Facts []CompletionFact` — `ls/projects/import_catalog.go:44-50`
- `projects.ImportCatalog` — `StdlibModules()`, `ProjectModules()`, `AliasExports(fileKey)` — built after compilation. `ls/projects/import_catalog.go:56-70`

## CompletionItemKind ownership chain

- **Compiler projections (`projects/`) own protocol-free kind enums** — `CompletionFactKind` (Function/ModuleVar/Constant/Type), `MemberCandidateKind` (Field/Method), `CallableKind` (Function/Method/Remote). None reference `protocol.CompletionItemKind`. `ls/projects/completion_index.go:28-35`, `ls/projects/member_completion_index.go:45-50`, `ls/projects/invocation_completion_index.go:45-50`
- **Query layer (`ls/core/query/`) owns `query.CompletionItemKind`** — 7 values: Keyword, Variable, Constant, Function, Type, Module, Snippet. Comment at definition explicitly says "owned by the query layer. The server maps it to an LSP CompletionItemKind." `ls/ls/core/query/completion.go:45-55`
- **Server (`ls/server/`) maps query kinds to LSP protocol kinds** — `toLSPCompletionItemKind()` at `ls/ls/server/completion.go:178-195`. Mapping: Keyword→14, Variable→6, Constant→21, Function→3, Type→22(Struct), Module→9, Snippet→15, default→1(Text).
- **Mapping chain**: `projects.CompletionFactKind` → `query.CompletionItemKind` (via `semanticItem()` at `ls/ls/core/query/completion.go:383-400`); `projects.MemberCandidateKind` → `query.CompletionItemKind` (via `memberCandidateItem()` at `ls/ls/core/query/completion.go:505-520`); `query.CompletionItemKind` → `protocol.CompletionItemKind` (via `toLSPCompletionItemKind()` at `ls/ls/server/completion.go:178-195`).
- **Compiler does NOT import `protocol.CompletionItemKind`** — zero grep hits in `ls/projects/`. Protocol-free by design.
- **`ls/core/query` carrying `protocol.CompletionItemKind` would break protocol-free design** — would create a dependency from query layer to LSP protocol package, violating the separation of concerns. Current architecture is correct.

## LS-side: query layer (ls/core/query)

- `query.CompletionLease` / `query.CompletionLeaser` — non-blocking, generation-matched lease. `ls/ls/core/query/completion.go:20-30`
- `query.Completion(u, byteOffset, ctx)` — protocol-free entry point. `ls/ls/core/query/completion.go:100-130`
- `classifyContext(part, offset, text)` — classifies cursor as import/function-body/module-part/alias-member/unsupported. `ls/ls/core/query/completion_module.go:100-140`
- `classifyCompletion(part, offset, text)` — extracts parameters and preceding locals for function-body positions. `ls/ls/core/query/completion.go:200-240`
- `classifyAliasMember(part, offset, text)` — detects `alias.<prefix>` access. `ls/ls/core/query/completion_module.go:200-230`
- `classifyImport(part, offset, text)` — classifies import sub-position. `ls/ls/core/query/completion_module.go:150-190`
- `collectPrecedingLocals(statements, offset)` — walks statement sequence, collects local var declarations before cursor. `ls/ls/core/query/completion.go:240-290`
- `extractParameters(fn)` — extracts named parameters from function signature. `ls/ls/core/query/completion.go:220-235`
- `boostExpectedCompatible(items, fact)` — lowers rank of precomputed-compatible items. `ls/ls/core/query/completion.go:170-185`
- `semanticItem(fact)` — converts `CompletionFact` to `CompletionItem`. `ls/ls/core/query/completion.go:190-205`
- `filterDedupSort(items, prefix)` — deduplicates by label, sorts by rank then label. `ls/ls/core/query/completion.go:365-380`

### classifyContext: purely red-node tree walking

- `classifyContext` takes `*tree.ModulePart`, `offset int`, `text string` — no compiler objects. `completion_module.go:100`
- Returns `(contextKind, importContext)` — `contextKind` is one of 5 values. `completion_module.go:91-98`
- First checks `cursorInComment(text, offset)` — returns `contextUnsupported`. `completion_module.go:103-105`
- Then checks `classifyImport(part, offset, text)` — walks `part.Imports()`, checks `rangeContains(imp, offset)`. `completion_module.go:106-108`
- Then walks `part.Members()` — for each member, checks `rangeContains(m, offset)`. `completion_module.go:109-138`
- For `*tree.FunctionDefinition`: checks `functionBodyBlock(fn)` then `cursorInBody(body, offset)`. `completion_module.go:112-124`
- For `*tree.ModuleVariableDeclarationNode`: checks semicolon/equals token presence for recovery. `completion_module.go:125-133`
- All position checks use `TextRange()` byte offsets. `completion_module.go:passim`
- `classifyImport` sub-classifies into `importOrg`, `importModule`, `importAsAlias` — walks `imp.OrgName()`, `imp.ModuleName()`, `imp.Prefix()`. `completion_module.go:150-190`
- `classifyAliasMember` scans left from cursor for `identifier.` pattern. `completion_module.go:200-230`
- `importAlias(imp)` extracts alias, org, module name from import declaration. `completion_module.go:235-255`
- `alreadyImportedModules(part)` builds `org/moduleName` set from imports. `completion_module.go:260-270`
- `importInsertionOffset(part, text)` finds insertion point after last import. `completion_module.go:275-290`
- `firstFreeAlias(part, moduleName, taken)` finds first unused alias suffix. `completion_module.go:295-310`
- `buildImportEdit(part, org, moduleName, alias)` constructs additional edit for auto-import. `completion_module.go:315-340`
- `modulePartItems(part, text, prefixStart, prefix)` builds snippet matrix. `completion_module.go:345-375`
- `importModuleItems(catalog, org, filterPrefix, stripPrefix, part)` — catalog-backed module completion. `completion_module.go:380-410`
- `importOrgItems(catalog, prefix)` — distinct org names from catalog. `completion_module.go:415-435`
- `aliasMemberItems(exports, alias, prefix)` — imported alias's public exports. `completion_module.go:440-455`
- `autoImportItems(catalog, alias, part, taken)` — unimported module candidates with additional edit. `completion_module.go:460-490`
- `detectMemberAccess(part, offset, text, catalog)` — classifies `recv.<prefix>`, `recv?.<prefix>`, `recv-><prefix>`. `completion.go:395-435`
- `completeMemberAccess(u, st, offset, ma)` — reads from generation-matched `MemberCompletionIndex`. `completion.go:440-475`
- `completeFunctionBody(u, part, st, offset, text)` — main function-body path: scope + body catalog + semantic facts. `completion.go:280-340`
- `completeModulePart(u, part, st, offset, text)` — snippet matrix + alias-member + member-access. `completion.go:240-260`
- `completeImport(part, st, offset, text, ic)` — dispatches by import sub-kind. `completion.go:215-235`
- `completeAliasMember(part, st, offset, text)` — alias-member or auto-import. `completion.go:265-275`
- `importCatalog(fileKey)` — acquires generation-matched lease, returns `*projects.ImportCatalog`. `completion.go:350-370`

### bodyPosition classification (completion_body.go)

- `bodyPosition` enum — `bodyExpression` (0), `bodyStatementStart` (1). `completion_body.go:22-26`
- `classifyBodyPosition(body, offset, text)` — classifies cursor as statement-start or expression. `completion_body.go:30-80`
- `statementStartCompletions` — 19 entries: if/else/while/foreach/do/return/fail/panic/break/continue/lock/transaction/retry/match/fork/worker/var/final/type. `completion_body.go:35-55`
- `expressionCompletions` — action/check/conditional/type-test/cast/literal/constructor catalog. `completion_body.go:60-80`
- `bodyCatalogItems(position, inLoop, elseFollowOn)` — returns position-gated catalog. `completion_body.go:85-100`
- `loopEncloses(body, offset)` — walks ancestor chain for enclosing loop. `completion_body.go:105-130`
- `canFollowWithElse(body, offset)` — detects if cursor follows an if without else. `completion_body.go:135-160`
- `collectScope(fn, body, offset)` — parameters + preceding locals, shadowing-aware. `completion_body.go:165-200`
- `collectPrecedingLocals(statements, offset)` — walks statements, collects locals before cursor. `completion.go:240-290`

## LS-side: server adapter (ls/server)

- `completionLeaseAdapter` — adapts `compile.CompilationService.Lease()` to `query.CompletionLeaser`. `ls/ls/server/completion.go:20-35`
- `handleCompletion(ctx, params)` — maps `textDocument/completion` to query, converts result to LSP. `ls/ls/server/completion.go:40-60`
- `toLSPCompletionList(result, text, snippets)` — converts byte-offset prefix range to UTF-16 TextEdit. `ls/ls/server/completion.go:65-100`
- `byteRangeToUTF16Range`, `byteOffsetToUTF16Position` — byte→UTF-16 conversion. `ls/ls/server/completion.go:105-130`
