# Completion item structure, ranking, sort/filter

### Completion item structure
- `item.rs:1-200` — `CompletionItem` struct: `label`, `source_range`, `text_edit`, `is_snippet`, `kind`, `lookup`, `detail`, `documentation`, `deprecated`, `trigger_call_info`, `relevance`, `ref_match`, `import_to_add`
- `item.rs:200-400` — `Builder` pattern: `from_resolution()`, `build()`, setters for all fields

### Kind system: local enum, not LSP
- `item.rs:363-374` — `CompletionItemKind` is a **local** enum, not LSP's. Wraps `SymbolKind(ide_db::SymbolKind)` plus extra variants: `Binding`, `BuiltinType`, `InferredType`, `Keyword`, `Snippet`, `UnresolvedReference`, `Expression`.
- `ide-db/src/lib.rs:267-298` — `SymbolKind` is a language-level enum (Struct, Function, Method, Enum, Trait, etc.) — no LSP dependency.
- `ide-db/src/lib.rs:311-330` — `SymbolKind::from_module_def()` maps HIR `ModuleDef` to `SymbolKind`.
- `render.rs:525-560` — `res_to_kind()` maps `ScopeDef` (resolution) to `CompletionItemKind` — the central mapping from semantic resolution to kind.
- `render/function.rs:68-75` — Providers construct `CompletionItem::new(CompletionItemKind::SymbolKind(SymbolKind::Method/Function), ...)` — they use the local kind, not LSP.
- `render/const_.rs:22` — `CompletionItem::new(SymbolKind::Const, ...)` — `SymbolKind` auto-converts via `impl_from!`.
- `render/macro_.rs:61` — `CompletionItem::new(SymbolKind::from(macro_.kind(...)), ...)` — HIR kind → SymbolKind via `From<hir::MacroKind>`.

### Kind mapping to LSP (single boundary)
- `lsp/to_proto.rs:129-175` — `completion_item_kind()`: **single exhaustive match** maps local `CompletionItemKind` → `lsp_types::CompletionItemKind`. Every variant (including all 30 `SymbolKind` sub-variants) is mapped explicitly.
- `lsp/to_proto.rs:379` — Called at `kind: Some(completion_item_kind(item.kind))` inside `completion_item()`.
- `lsp/to_proto.rs:248-287` — `completion_items()`: receives `Vec<CompletionItem>` (internal), calls `completion_item()` for each, which calls `completion_item_kind()`.
- `handlers/request.rs:1149` — Handler calls `snap.analysis.completions()` → gets `Vec<CompletionItem>` (internal), then `to_proto::completion_items()` for LSP conversion.

### Key design takeaway
rust-analyzer uses a **two-tier kind system**: a language-level `SymbolKind` (30 variants) wrapped in a completion-specific `CompletionItemKind` (7 extra variants). The LSP mapping is a single, exhaustive function at the protocol boundary. No provider ever touches an LSP kind constant. This is the idiomatic Rust approach and generalizes to any LSP server.
- `item.rs:400-500` — `CompletionRelevance` struct: `exact_name_match`, `type_match`, `is_local`, `trait_`, `is_name_already_imported`, `requires_import`, `is_private_editable`, `postfix_match`, `function`, `is_skipping_completion`
- `item.rs:500-600` — `CompletionRelevance::score()`: computes u32 score for sorting; BASE_SCORE = u32::MAX/2; exact_name_match +20, type_match Exact +18/CouldUnify +5, postfix Exact +100, local +1, trait methods -5, op methods -5, skipping -7, requires_import -1
- `item.rs:600-700` — `CompletionRelevanceTypeMatch` enum: `CouldUnify` | `Exact`
- `item.rs:700-800` — `CompletionRelevanceReturnType` enum: `Other` | `DirectConstructor` (+15) | `Constructor` (+5) | `Builder` (+10)

### Sort/filter behavior
- `lsp/to_proto.rs:248-280` — `completion_items()`: filters deprecated items if config says so, then calls `completion_item()` for each, then sorts by `sort_text` and truncates to `limit`
- `lsp/to_proto.rs:462-500` — `set_score()`: sets `preselect = true` if relevance is relevant AND is max; computes `sort_score = relevance.score() ^ 0xFFFF_FFFF` (inverted for ascending sort); formats as 8-char hex `sort_text`
- `lsp/to_proto.rs:280-448` — `completion_item()`: maps internal `CompletionItem` to LSP `CompletionItem`, defers resolution for fields in `CompletionFieldsToResolve`
- `lsp/to_proto.rs:429-443` — `CompletionResolveData` stores `position`, `imports`, `version`, `trigger_character`, `for_ref`, `hash`
- `lsp.rs:37-120` — `completion_item_hash()`: hashes label, lookup, kind, relevance, ref_match, import_to_add using TentHash
