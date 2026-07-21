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

## LS-side: server adapter (ls/server)

- `completionLeaseAdapter` — adapts `compile.CompilationService.Lease()` to `query.CompletionLeaser`. `ls/ls/server/completion.go:20-35`
- `handleCompletion(ctx, params)` — maps `textDocument/completion` to query, converts result to LSP. `ls/ls/server/completion.go:40-60`
- `toLSPCompletionList(result, text, snippets)` — converts byte-offset prefix range to UTF-16 TextEdit. `ls/ls/server/completion.go:65-100`
- `byteRangeToUTF16Range`, `byteOffsetToUTF16Position` — byte→UTF-16 conversion. `ls/ls/server/completion.go:105-130`
