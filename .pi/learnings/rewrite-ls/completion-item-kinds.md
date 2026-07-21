# Completion item kinds: Java LS vs Go rewrite

How completion item kinds are represented and whether feature/provider layers
use LSP `CompletionItemKind` directly or a local vocabulary.

## Java LS: direct LSP CompletionItemKind everywhere

The Java LS has **no local vocabulary** for completion item kinds. Every
provider and builder sets `CompletionItemKind` (the LSP enum) directly on the
LSP `CompletionItem` object. There is no intermediate representation.

### Kind-setting entry points

- **`CompletionItemBuilder.getKind(SnippetBlock)`** at
  `completions/builder/CompletionItemBuilder.java:35-42` — maps `SnippetBlock.Kind`
  (a local enum with values `KEYWORD`, `SNIPPET`, `STATEMENT`, `TYPE`, `VALUE`)
  to LSP `CompletionItemKind`:
  - `KEYWORD` → `Keyword` (14)
  - `TYPE` → `TypeParameter` (25)
  - `VALUE` → `Value` (12)
  - default (SNIPPET, STATEMENT) → `Snippet` (15)

- **`CompletionItemBuilder.getKind(Symbol)`** at
  `completions/builder/CompletionItemBuilder.java:24-28` — maps `SymbolKind`:
  - `ENUM` → `Enum` (13)
  - default → `Unit` (11)

- **`TypeCompletionItemBuilder.setMeta()`** at
  `completions/builder/TypeCompletionItemBuilder.java:65-126` — maps type
  descriptor kinds to LSP kinds:
  - `MODULE` → `Module` (9)
  - `ENUM` → `Enum` (13)
  - `UNION` (all ERROR) → `Event` (23)
  - `UNION` (all RECORD) → `Struct` (22)
  - `UNION` (all OBJECT) → `Interface` (8)
  - `UNION` (mixed) → `Enum` (13)
  - `RECORD` → `Struct` (22)
  - `OBJECT` → `Interface` (8)
  - `ERROR` → `Event` (23)
  - default → `TypeParameter` (25)

- **`FunctionCompletionItemBuilder`** at
  `completions/builder/FunctionCompletionItemBuilder.java:120,169,409`:
  - `FunctionSymbol` with `SymbolKind.VARIABLE` → `Variable` (6)
  - `FunctionSymbol` with `SymbolKind.FUNCTION` → `Function` (3)
  - Anonymous function snippet → `Snippet` (15)

- **Other builders** (all set LSP `CompletionItemKind` directly):
  - `ParameterCompletionItemBuilder.java:47` → `Variable` (6)
  - `SpreadCompletionItemBuilder.java:58,74` → `Variable` (6) or `Function` (3)
  - `NamedArgCompletionItemBuilder.java:54` → `Snippet` (15)
  - `ForeachCompletionItemBuilder.java:55` → `Snippet` (15)
  - `ServiceTemplateCompletionItemBuilder.java:57` → `Snippet` (15)
  - `XMLNSCompletionItemBuilder.java:46` → `Variable` (6)
  - `StreamTypeInitCompletionItemBuilder.java:55,66` → `Function` (3)
  - `TypeGuardCompletionItemBuilder.java:53` → `Snippet` (15)
  - `FieldAccessContext.java:180` → `Snippet` (15)
  - `AnnotationAccessExpressionNodeContext.java:202` → `Property` (10)
  - `CompilerPluginCompletionExtension.java:105` → `Snippet` (15)
  - `AbstractCompletionProvider.java:694` → `Module` (9)
  - `ImportDeclarationContextUtil.java:65` → `Module` (9)

### Sorting uses LSP CompletionItemKind

- **`SortingUtil.toRank()`** at `SortingUtil.java:530` — reads
  `completionItem.getCompletionItem().getKind()` (the LSP kind) to assign sort
  rank. Ranks: Constant=1(onQName)/1, Variable=3/2, Function=1/3, Method=4,
  Constructor=5, ObjectField=6, RecordField=7, EnumMember=8, Enum=9, Class=10,
  Interface=11, Event=12, Struct=13, TypeParameter=14, Module=15, Snippet=16,
  Keyword=17, default=18; `main(` gets rank 25.

### SnippetBlock.Kind (local enum, not a kind vocabulary)

- **`SnippetBlock.Kind`** at `SnippetBlock.java:133-136` — enum with values
  `KEYWORD`, `SNIPPET`, `STATEMENT`, `TYPE`, `VALUE`. This is a *snippet
  classification*, not a completion kind vocabulary. It is used only to select
  the LSP `CompletionItemKind` via `CompletionItemBuilder.getKind(SnippetBlock)`.

### StaticCompletionItem.Kind (local enum, not a kind vocabulary)

- **`StaticCompletionItem.Kind`** at `StaticCompletionItem.java:42-50` — enum
  with values `LANG_LIB_MODULE`, `MODULE`, `TYPE`, `KEYWORD`,
  `SERVICE_TEMPLATE`, `MAIN_FUNCTION`, `OTHER`. This is a *static item
  classification* used for sorting and filtering, not a completion kind
  vocabulary. The LSP `CompletionItemKind` is still set directly on the
  `CompletionItem`.

### LSCompletionItem.CompletionItemType (item type, not kind)

- **`LSCompletionItem.CompletionItemType`** at
  `LSCompletionItem.java:40-47` — enum with values `OBJECT_FIELD`,
  `RECORD_FIELD`, `SNIPPET`, `STATIC`, `SYMBOL`, `TYPE`, `FUNCTION_POINTER`,
  `NAMED_ARG`, `SPREAD`. This is a *completion item type* (how the item was
  produced), not a kind vocabulary. The LSP `CompletionItemKind` is still set
  directly on the `CompletionItem`.

## Go rewrite: protocol-free local vocabulary, mapped at server boundary

The Go rewrite introduces a **protocol-free local vocabulary** (`CompletionItemKind`
in `ls/core/query`) that is mapped to LSP `CompletionItemKind` only at the
server boundary. This is the target shape for ticket 31.

### Query-layer kind (protocol-free)

- **`query.CompletionItemKind`** at `ls/core/query/completion.go:45-60` — `uint8`
  enum with 7 values: `CompletionKindKeyword`, `CompletionKindVariable`,
  `CompletionKindConstant`, `CompletionKindFunction`, `CompletionKindType`,
  `CompletionKindModule`, `CompletionKindSnippet`.

- Used in `CompletionItem.Kind` field at `completion.go:77`.

- Set by:
  - `semanticItem()` at `completion.go:385-402` — maps `projects.CompletionFactKind`
    (Function→`CompletionKindFunction`, ModuleVar→`CompletionKindVariable`,
    Constant→`CompletionKindConstant`, Type→`CompletionKindType`)
  - `memberCandidateItem()` at `completion.go:506-520` — maps
    `projects.MemberCandidateKind` (Field→`CompletionKindVariable`,
    Method→`CompletionKindFunction`)
  - `bodyCatalogItems()` at `completion_body.go:355-385` — all body catalog items
    get `CompletionKindSnippet`
  - `modulePartItems()` at `completion_module.go:446-476` — all module-part snippet
    items get `CompletionKindSnippet`
  - `importModuleItems()` / `importOrgItems()` / `autoImportItems()` at
    `completion_module.go:536-577,577-609,653-691` — all import items get `CompletionKindModule`
  - `collectScope()` at `completion_body.go:387-412` — scope entries (parameters,
    locals) get `CompletionKindVariable`

### Compiler/projects-layer kind (protocol-free)

- **`projects.CompletionFactKind`** at `projects/completion_index.go:28-38` —
  `uint8` enum with 4 values: `CompletionFactFunction`, `CompletionFactModuleVar`,
  `CompletionFactConstant`, `CompletionFactType`. This is the compiler's own
  protocol-free kind, used in `CompletionFact` and `AliasExport.Facts`.

- **`projects.MemberCandidateKind`** at `projects/member_completion_index.go:45-52`
  — `uint8` enum with 2 values: `MemberCandidateField`, `MemberCandidateMethod`.

### Server boundary mapping

- **`toLSPCompletionItemKind()`** at `ls/server/completion.go:178-199` — maps
  the 7 query kinds to LSP `protocol.CompletionItemKind`:
  - `CompletionKindKeyword` → `CompletionItemKindKeyword` (14)
  - `CompletionKindVariable` → `CompletionItemKindVariable` (6)
  - `CompletionKindConstant` → `CompletionItemKindConstant` (21)
  - `CompletionKindFunction` → `CompletionItemKindFunction` (3)
  - `CompletionKindType` → `CompletionItemKindStruct` (22)
  - `CompletionKindModule` → `CompletionItemKindModule` (9)
  - `CompletionKindSnippet` → `CompletionItemKindSnippet` (15)
  - default → `CompletionItemKindText` (1)

### LSP protocol types

- **`protocol.CompletionItemKind`** at `ls/protocol/types_generated.go:295-325`
  — `uint32` enum with all standard LSP values (1-25).

## Key differences

| Aspect | Java LS | Go rewrite |
|--------|---------|------------|
| Kind vocabulary | None — uses LSP `CompletionItemKind` directly everywhere | Three-layer: `projects.CompletionFactKind` → `query.CompletionItemKind` → `protocol.CompletionItemKind` |
| Where kind is set | Directly on `CompletionItem.setKind()` in each builder/provider | On `query.CompletionItem.Kind` field; mapped at server boundary |
| Number of distinct kinds used | ~15 LSP kinds (Text, Method, Function, Constructor, Variable, Class, Interface, Module, Property, Unit, Value, Enum, Keyword, Snippet, Struct, Event, TypeParameter) | 7 query kinds (Keyword, Variable, Constant, Function, Type, Module, Snippet) |
| Sorting | Reads LSP `CompletionItemKind` from `CompletionItem.getKind()` | Uses `Rank` field (int), not kind-based |
| Snippet classification | `SnippetBlock.Kind` (KEYWORD/SNIPPET/STATEMENT/TYPE/VALUE) — local enum for snippet type, not kind | `CompletionKindSnippet` for all snippet/construct items |
| Static item classification | `StaticCompletionItem.Kind` (MODULE/TYPE/KEYWORD/etc.) — local enum for static item type | No equivalent — static items use `CompletionKindSnippet` or `CompletionKindModule` |
| Item type classification | `LSCompletionItem.CompletionItemType` (SYMBOL/SNIPPET/STATIC/etc.) — how item was produced | No equivalent — all items are `query.CompletionItem` |

## Gaps / contradictions

1. **Java LS uses `CompletionItemKind.Struct` (22) for Ballerina record types**,
   while the Go rewrite maps `CompletionKindType` to `CompletionItemKind.Struct`
   (22) as well — consistent for records, but the Java LS also uses
   `CompletionItemKind.Interface` (8) for object types and `CompletionItemKind.Event`
   (23) for error types, which the Go rewrite's single `CompletionKindType` → `Struct`
   mapping does not distinguish.

2. **Java LS uses `CompletionItemKind.Class` (7) for class symbols** (via
   `SortingUtil.toRank()` rank 10), but the Go rewrite maps all types
   (including classes) to `CompletionKindType` → `Struct` (22). This is a
   behavioral divergence: the Java LS distinguishes classes from records/objects
   at the kind level; the Go rewrite does not.

3. **Java LS uses `CompletionItemKind.Enum` (13) and `EnumMember` (20)** for
   enum types and members; the Go rewrite has no `CompletionKindEnum` or
   `CompletionKindEnumMember` — enums are mapped to `CompletionKindType` →
   `Struct` (22).

4. **Java LS uses `CompletionItemKind.Constant` (21) for constants**; the Go
   rewrite has `CompletionKindConstant` → `CompletionItemKind.Constant` (21) —
   consistent.

5. **Java LS uses `CompletionItemKind.Method` (2) for methods**; the Go rewrite
   maps member methods to `CompletionKindFunction` → `CompletionItemKind.Function`
   (3), not `Method` (2). This is a behavioral divergence.

6. **Java LS uses `CompletionItemKind.Constructor` (4) for constructors**; the
   Go rewrite has no `CompletionKindConstructor` — constructors are not yet
   represented.

7. **Java LS uses `CompletionItemKind.Property` (10) for annotation access
   expressions**; the Go rewrite has no equivalent.

8. **Java LS uses `CompletionItemKind.Value` (12) for value-type snippets**;
   the Go rewrite maps all snippets to `CompletionKindSnippet` → `Snippet` (15),
   losing the `Value` distinction.

9. **Java LS uses `CompletionItemKind.Unit` (11) as a fallback** for unknown
   symbols; the Go rewrite uses `CompletionItemKindText` (1) as fallback.
