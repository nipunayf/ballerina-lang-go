# Completion pipeline

## Entry point and dispatch

- `ls/ls/server/completion.go:50-80` — `Server.handleCompletion()` — maps `textDocument/completion` to protocol-free query, converts UTF-16 position to byte offset, converts result to LSP CompletionList with explicit TextEdits
- `ls/ls/server/completion.go:55-60` — `emptyCompletionList()` — returns `IsIncomplete: false, Items: []`
- `ls/ls/server/completion.go:65-100` — `toLSPCompletionList()` — converts query DTOs to LSP items; selects snippet vs plaintext per client capability; converts AdditionalEdits from byte offsets to UTF-16 TextEdits
- `ls/ls/server/completion.go:105-130` — `byteRangeToUTF16Range()` / `byteOffsetToUTF16Position()` — UTF-16 conversion with precomputed line starts
- `ls/ls/server/completion.go:135-155` — `toLSPCompletionItemKind()` — maps 7 query kinds to LSP CompletionItemKind

## Core query layer

- `ls/ls/core/query/completion.go:1-200` — `query.Service.Completion()` — main entry: classifies context (import/function-body/module-part/alias-member), dispatches to specialized path
- `ls/ls/core/query/completion.go:60-80` — `Completion()` — 4 context kinds: `contextImport`, `contextFunctionBody`, `contextModulePart`, `contextAliasMember`
- `ls/ls/core/query/completion.go:85-120` — `completeImport()` — import-context completion: org candidates, module candidates (full-path filtered, active-segment-only replacement), alias position (free-form, no candidates)
- `ls/ls/core/query/completion.go:125-155` — `completeModulePart()` — top-level declaration/type/keyword snippet matrix; detects alias-member access and member access; defers to specialized paths
- `ls/ls/core/query/completion.go:160-200` — `completeFunctionBody()` — function-body completion: scope walking (parameters + preceding locals), position-gated keyword/construct catalog, semantic module facts from generation-matched lease, expected-type boost
- `ls/ls/core/query/completion.go:205-240` — `completeAliasMember()` / `completeAliasMemberWith()` — imported alias's public exports; auto-import candidate for unimported stdlib module

## Context classification

- `ls/ls/core/query/completion_module.go:60-100` — `classifyContext()` — walks module part's imports and members once; detects import, function body, module-part declaration positions; returns `contextUnsupported` for signature/type-descriptor positions
- `ls/ls/core/query/completion_module.go:105-160` — `classifyImport()` — detects cursor inside import declaration; classifies sub-position (org, module, `as` alias); computes replacement span and filter/strip prefixes
- `ls/ls/core/query/completion_module.go:165-210` — `classifyImportDecl()` — org region, alias region, module region with active segment span
- `ls/ls/core/query/completion.go:245-280` — `classifyCompletion()` — function-body classification: finds enclosing function, verifies cursor in body, extracts prefix, classifies body position (statement-start vs expression), computes loop/else gating, collects scope
- `ls/ls/core/query/completion_body.go:60-90` — `classifyBodyPosition()` — recovery-aware: scans left over whitespace to last non-whitespace char; `{`, `;`, `}`, `)`, `]` → statement-start; operators/punctuation/ident → expression; ambiguous → conservative statement-start
- `ls/ls/core/query/completion_body.go:95-120` — `loopEncloses()` — descends statement chain, returns true at first while/foreach, false at first fork (scope barrier)
- `ls/ls/core/query/completion_body.go:125-180` — `descendStatementChain()` — walks containing statements outer-to-inner, recurses only into cursor-containing branch
- `ls/ls/core/query/completion_body.go:185-210` — `canFollowWithElse()` — checks preceding sibling is if/else-if without else clause

## Completion routing (4 paths)

### Path 1: Function body (contextFunctionBody)

- `ls/ls/core/query/completion.go:160-200` — `completeFunctionBody()` — scope + catalog + semantic facts + expected-type boost
- `ls/ls/core/query/completion_body.go:215-260` — `collectScope()` — parameters (outermost) + preceding locals; innermost-wins shadowing via `seen` map
- `ls/ls/core/query/completion_body.go:265-320` — `collectLocalsScope()` — walks statement sequence, adds local declarations, recurses into cursor-containing nested blocks; skips sibling branches
- `ls/ls/core/query/completion.go:285-310` — `extractParameters()` — iterates parameter list; handles RequiredParameterNode, DefaultableParameterNode, RestParameterNode, IncludedRecordParameterNode
- `ls/ls/core/query/completion.go:315-330` — `paramInfo()` — extracts name and type-detail from each parameter kind

### Path 2: Module part (contextModulePart)

- `ls/ls/core/query/completion_module.go:215-260` — `modulePartItems()` — declaration/type/keyword snippet matrix; `import` only in import region; `main` only when no main exists; qualifier suppression
- `ls/ls/core/query/completion_module.go:265-300` — `modulePartCompletions` — 11 entries: function, type, const, class, enum, listener, final, public, private, import, main
- `ls/ls/core/query/completion_module.go:305-340` — `inImportRegion()` / `documentHasMain()` / `precedingQualifier()` — gating helpers

### Path 3: Import (contextImport)

- `ls/ls/core/query/completion_module.go:345-390` — `importModuleItems()` — catalog modules filtered by full dot-path; already-imported suppressed; InsertText strips preceding segments
- `ls/ls/core/query/completion_module.go:395-420` — `importOrgItems()` — distinct org names from catalog
- `ls/ls/core/query/completion_module.go:425-450` — `aliasMemberItems()` — imported alias's public exports
- `ls/ls/core/query/completion_module.go:455-490` — `autoImportItems()` — unimported catalog module with AdditionalEdit for import insertion
- `ls/ls/core/query/completion_module.go:495-520` — `buildImportEdit()` — constructs import declaration text; handles preceding import separation, trailing newline

### Path 4: Member access (detectMemberAccess)

- `ls/ls/core/query/completion.go:340-380` — `detectMemberAccess()` — recovery-aware: scans left of cursor for `.`, `?.`, `->`; yields to alias-member route for `.` on declared/catalog alias
- `ls/ls/core/query/completion.go:385-420` — `completeMemberAccess()` — reads generation-matched MemberCompletionIndex slot; prefix-filters, dedups, sorts
- `ls/ls/core/query/completion.go:425-440` — `memberCandidateItem()` — converts MemberCandidate to CompletionItem

## Body keyword/construct catalog

- `ls/ls/core/query/completion_body.go:30-55` — `statementStartCompletions` — 18 entries: if, else, while, foreach, do, return, fail, panic, break, continue, lock, transaction, retry, match, fork, worker, var, final, type
- `ls/ls/core/query/completion_body.go:60-90` — `expressionCompletions` — 25 entries: check, checkpanic, trap, start, wait, flush, from, new, typeof, function, ?, is, <>, [], {}, object, error(), true, false, null, "", 0, base16, base64, re, string ``, xml ``
- `ls/ls/core/query/completion_body.go:95-120` — `bodyCatalogItems()` — gates loop-only (break/continue) by inLoop, else by elseFollowOn; all items carry both plaintext and snippet forms

## Semantic module facts (generation-matched lease)

- `ls/ls/core/query/completion.go:445-480` — `importCatalog()` — acquires generation-matched lease, returns ImportCatalog or nil
- `ls/ls/core/query/completion.go:485-510` — `boostExpectedCompatible()` — lowers rank to `rankExpectedMatch` (-1) for precomputed-compatible labels
- `ls/ls/core/query/completion.go:515-540` — `semanticItem()` — converts CompletionFact to CompletionItem with rank
- `ls/ls/projects/completion_index.go:54-100` — `CompletionIndex` — immutable, module-level facts (functions, module vars, constants, types); `Facts(fileKey, offset)` returns all facts for a module
- `ls/ls/projects/completion_index.go:105-160` — `BuildCompletionIndex()` — walks compiled package's syntax trees; extracts facts from FunctionDefinition, TypeDefinitionNode, ConstantDeclarationNode, ModuleVariableDeclarationNode, EnumDeclarationNode, ClassDefinitionNode
- `ls/ls/projects/expected_type_index.go:1-200` — `ExpectedTypeIndex` — immutable, generation-scoped; captures 10 slot kinds: Assignment, Return, Condition, Panic, Check, Argument, NamedArgument, MappingField, ListMember, NewArg
- `ls/ls/projects/expected_type_index.go:60-90` — `FactAt()` — innermost (smallest-span) fact wins; positional arg index breaks ties
- `ls/ls/projects/expected_type_index.go:95-160` — `BuildExpectedTypeIndex()` — reads resolver-captured slot records; computes display-safe type label and compatible candidate labels via `semtypes.IsSubtype`
- `ls/ls/projects/expected_type_index.go:165-220` — `buildModuleExpectedTypeIndex()` — called in analyzeAndDesugar after local-node resolution; recovers from panic without crashing compilation
- `ls/ls/projects/expected_type_index.go:225-280` — `moduleValueCandidates()` — enumerates module variables and constants with resolved types; excludes types and functions from compatibility precomputation
- `ls/ls/projects/expected_type_index.go:285-310` — `compatibleCandidates()` — `semtypes.IsSubtype` check at projection-build time
- `ls/ls/projects/member_completion_index.go:1-200` — `MemberCompletionIndex` — immutable, generation-scoped; captures `.`, `?.`, `->` access slots
- `ls/ls/projects/member_completion_index.go:60-90` — `SlotAt()` — exact match by kind + dot byte offset
- `ls/ls/projects/member_completion_index.go:95-160` — `BuildMemberCompletionIndex()` — walks resolved AST for field-access expressions; reads receiver's determined type
- `ls/ls/projects/member_completion_index.go:165-240` — `fieldAccessCollector` — ast.Visitor that collects BLangFieldBaseAccess and BLangRemoteMethodCallAction
- `ls/ls/projects/member_completion_index.go:245-310` — `accessibleMembers()` — dispatches to `mappingFields()` or `objectMembers()` based on subtype check
- `ls/ls/projects/member_completion_index.go:315-370` — `objectMembers()` — fields get plaintext; methods get snippet `name($1)$0`; `$`-prefixed generated names excluded
- `ls/ls/projects/member_completion_index.go:375-420` — `remoteMethods()` — strips `$remote$` prefix; snippet form is plaintext-equivalent (parens already in source)
- `ls/ls/projects/member_completion_index.go:425-450` — `mappingFields()` — record fields from MappingAtomicType
- `ls/ls/projects/import_catalog.go` — `ImportCatalog` — embedded stdlib + current-project modules; AliasExports per file

## Ranking/filter/sort

- `ls/ls/core/query/completion.go:545-580` — `filterDedupSort()` — prefix filter (case-sensitive), dedup by label keeping lowest rank, sort by rank then label
- `ls/ls/core/query/completion.go:30-40` — Rank constants: `rankExpectedMatch=-1`, `rankParameter=0`, `rankLocal=1`, `rankKeyword=2`, `rankModuleVar=3`, `rankConstant=4`, `rankFunction=5`, `rankType=6`, `rankSnippet=0`, `rankImportModule=1`
- `ls/ls/projects/member_completion_index.go:455-460` — Member ranks: `rankMemberField=0`, `rankMemberMethod=1`

## Expected-type completion (type-directed)

- **Slot kinds captured**: Assignment, Return, Condition, Panic, Check, Argument, NamedArgument, MappingField, ListMember, NewArg — `ls/ls/projects/expected_type_index.go:10-25`
- **Capture mechanism**: `recordExpectedSlot()` in type resolver — `ls/ls/semantics/type_resolver.go:3201-3230` — called from `resolveActionOrExpression`; gated by `CompilerContext.CaptureExpectedSlots()`
- **Compiler context**: `CompilerContext.RecordExpectedSlot()` — `ls/ls/context/context.go:297-305` — thread-safe via `slotMu`; no-op when capture disabled
- **ExpectedSlotRecord**: `FileKey`, `StartOffset`, `EndOffset`, `ArgIndex`, `Kind`, `Expected semtypes.SemType` — `ls/ls/context/context.go:97-110`
- **Innermost span wins**: `FactAt()` in `expected_type_index.go:60-90` — positional arg index breaks ties
- **Compatible candidates**: only module variables and constants are precomputed for assignability; types and functions are excluded — `ls/ls/projects/expected_type_index.go:225-280`
- **Tests**: `TestCompletionExpectedTypeRanksCompatibleFirst`, `TestCompletionExpectedTypeArgumentRanking`, `TestCompletionExpectedTypeMappingFieldRanking`, `TestCompletionExpectedTypeListMemberRanking`, `TestCompletionExpectedTypeNamedArgRanking`, `TestCompletionExpectedTypeRestArgumentRanking`, `TestCompletionExpectedTypeSupersessionFallback`, `TestCompletionExpectedTypeFallbackNoFactAtCursor`, `TestCompletionExpectedTypeConcurrentRequests` — `ls/ls/core/compile/completion_query_test.go:168-470`

## Non-blocking lease architecture

- `ls/ls/core/compile/completion_store_test.go` — `SnapshotStore` with LRU eviction, active lease pinning, deferred disposal
- `ls/ls/core/compile/completion_lease_test.go` — lease lifecycle tests
- `ls/ls/server/completion.go:15-40` — `completionLeaseAdapter` — adapts compile service lease to query layer's `CompletionLeaser` interface
- `ls/ls/core/query/completion.go:20-30` — `CompletionLease` interface: `Index()`, `ExpectedTypeIndex()`, `ImportCatalog()`, `MemberCompletionIndex()`, `Release()`
- `ls/ls/core/query/completion.go:445-480` — `importCatalog()` — acquires lease, returns catalog or nil; never waits/parses/compiles

## GAPS: What does NOT exist

### No invocation snippet generation
- Parameters are only used for scope walking (offering names as variables), never for generating `functionName(${1:param1}, ${2:param2})$0` invocation snippets
- `extractParameters()` at `ls/ls/core/query/completion.go:285-310` collects parameter names for scope entries, not for invocation completion
- No function-signature-aware completion that would offer a function name with its parameter list as a snippet

### No named argument name completion
- When cursor is inside a function call's argument list, the PoC doesn't offer named argument labels (`foo:`, `bar:`) as completion items
- `ExpectedSlotNamedArgument` is captured and used for expected-type ranking of values, but the argument *names* themselves are never offered as completion candidates

### No required/defaultable/rest snippet differentiation
- All parameter types are treated the same for scope walking
- No generation of different snippet forms for required vs defaultable vs rest parameters
- No `IncludedRecordParameterNode` expansion into its constituent fields as named argument snippets

### No function signature help integration
- Completion doesn't show function signatures or parameter info alongside completion items
- No `textDocument/signatureHelp` handler exists in the server

### No `new` constructor completion
- `ExpectedSlotNewArg` is captured but there's no completion that offers object constructors matching the expected type
- The `"new"` keyword in `expressionCompletions` is a plain keyword, not a type-directed constructor offer

### No malformed call recovery
- No specific handling for malformed function calls (e.g., cursor inside `add(1, ` with missing closing paren)
- `classifyBodyPosition()` handles general recovery but doesn't specifically recognize call argument positions

### No trigger characters advertised
- `ls/ls/server/completion.go:50-80` — `handleCompletion()` doesn't set trigger characters on the server capabilities
- The server advertises only basic completion with no trigger characters, snippets, insert/replace edits, resolve, or incomplete lists

## Test fixtures

- `ls/ls/core/query/completion_test.go` — unit tests for query layer: unsupported positions, function body locals/keywords, prefix filter, dedup, defaultable parameters, if/else branch isolation, comment positions, statement-start vs expression, loop gating, fork barrier, else follow-on, closed block boundary, shadowing
- `ls/ls/core/compile/completion_query_test.go` — integration tests with compile service: fallback before snapshot, expected-type ranking (assignment, argument, mapping field, list member, named arg, rest arg), supersession fallback, concurrent requests
- `ls/ls/corpus/corpus_test.go:146-362` — 30+ corpus transcript tests: function-body, prefix-filter, unsupported-position, sibling-files, comment-position, expected-type-ranking, module-part-decl, module-part-snippet, import-module, import-org, import-alias, import-dotted, import-org-filter, auto-import-after-import, auto-import-collision, auto-import-project, alias-member, record-field, member-prefix-filter, optional-field, member-unsupported-receiver, object-method, object-method-snippet, member-recovery, remote-method, remote-method-filter, auto-import, recovered-function, statement-start, expression-position, loop-gating, fork-barrier, shadowing, astral-prefix, snippet-capability, absent-lease

## Extension points

- New completion kind: add to `contextKind` enum, add case in `classifyContext()`, add dispatch in `Completion()`
- New snippet: add to `statementStartCompletions` or `expressionCompletions` in `completion_body.go`
- New module-part snippet: add to `modulePartCompletions` in `completion_module.go`
- New expected-type slot kind: add to `ExpectedSlotKind` enum, add capture site in type resolver, add mapping in `toProjectedKind()`
- New member access kind: add to `MemberAccessKind` enum, add detection in `detectMemberAccess()`, add projection in `fieldAccessCollector`
- New auto-import source: add to catalog's `StdlibModules()` or `ProjectModules()`
