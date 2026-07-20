# Completion item structure, ranking, sort/filter

### Completion item structure
- `item.rs:1-200` — `CompletionItem` struct: `label`, `source_range`, `text_edit`, `is_snippet`, `kind`, `lookup`, `detail`, `documentation`, `deprecated`, `trigger_call_info`, `relevance`, `ref_match`, `import_to_add`
- `item.rs:200-400` — `Builder` pattern: `from_resolution()`, `build()`, setters for all fields
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
