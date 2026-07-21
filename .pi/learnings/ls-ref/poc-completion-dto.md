# PoC completion DTO design: CompletionItemKind ownership and mapping

## No intermediate/core-level DTO — protocol types used directly

The PoC (`ls-ref/lsp/completion.go`) does **not** have a separate core/query-level DTO for completion items. The entire pipeline constructs `protocol.CompletionItem` directly — there is no local mirror or intermediate representation.

- `ls-ref/lsp/completion.go:1525-1536` — `completionItemKind()` — single mapping function from `model.SymbolKind` to `protocol.CompletionItemKind` constants. This is the **only** mapping point for completion items.
- `ls-ref/lsp/protocol/types.go:155-161` — `CompletionItemKind` constants: only 6 defined (`Function=3`, `Variable=6`, `Class=7`, `Module=9`, `Keyword=14`, `Constant=21`). This is a **subset** of the full LSP spec — no `Method`, `Property`, `Enum`, `EnumMember`, `Struct`, `Interface`, `Constructor`, `Value`, `Snippet`, etc.

## Hardcoded kind values throughout

Items are constructed with direct `protocol.CompletionItemKind*` references, not through a central mapping:

- `ls-ref/lsp/completion.go:248` — `Kind: protocol.CompletionItemKindKeyword` (return type desc)
- `ls-ref/lsp/completion.go:253-254` — `Kind: protocol.CompletionItemKindVariable` (record fields)
- `ls-ref/lsp/completion.go:260` — `Kind: protocol.CompletionItemKindModule` (import alias)
- `ls-ref/lsp/completion.go:270` — `Kind: protocol.CompletionItemKindModule` (auto-import)
- `ls-ref/lsp/completion.go:300-310` — `Kind: protocol.CompletionItemKindKeyword` (foreach, while, if)
- `ls-ref/lsp/completion.go:320-325` — `Kind: protocol.CompletionItemKindKeyword` (return, break, continue)
- `ls-ref/lsp/completion.go:340-350` — `Kind: protocol.CompletionItemKindFunction`, `Keyword`, `Variable` (module var decl snippets)
- `ls-ref/lsp/completion.go:400-410` — `Kind: protocol.CompletionItemKindVariable` (member access fields)
- `ls-ref/lsp/completion.go:420-430` — `Kind: protocol.CompletionItemKindFunction` (member access methods)
- `ls-ref/lsp/completion.go:1525-1536` — `completionItemKind()` maps `model.SymbolKindType` → `CompletionItemKindClass` (7), not `CompletionItemKindStruct` or `CompletionItemKindInterface`

## Dual mapping inconsistency

There are two separate `model.SymbolKind` → protocol kind mapping functions with different mappings for the same input:

- `ls-ref/lsp/completion.go:1525-1536` — `completionItemKind()`: `SymbolKindType` → `CompletionItemKindClass` (7)
- `ls-ref/lsp/symbols.go:120-130` — `lspSymbolKind()`: `SymbolKindType` → `SymbolKindStruct` (23)

This means a type symbol gets `CompletionItemKindClass` in completion but `SymbolKindStruct` in document/workspace symbols. This is a prototype inconsistency, not a deliberate design choice.

## Assessment: prototype shortcut, not production pattern

**Do NOT copy as-is.** The direct coupling to `protocol.CompletionItem` in the query layer means:
1. The query layer is tied to LSP wire format — can't be reused for non-LSP consumers
2. No type safety around which kind values are valid for which context
3. Hardcoded kind values scattered across 20+ construction sites make it hard to audit or extend
4. The 6-kind subset is insufficient for a production LS (missing `Method`, `Property`, `Enum`, `EnumMember`, `Snippet`, `Value`, `Operator`, `Color`, `File`, `Reference`, `Folder`, `Unit`, `Struct`, `Interface`, `Constructor`, `Event`, `TypeParameter`)

**What a production design should do instead:**
- Define a core/query-level `CompletionItem` DTO with a domain-specific kind enum (e.g., `KindFunction`, `KindVariable`, `KindType`, `KindKeyword`, `KindSnippet`, `KindModule`, `KindConstant`, `KindMethod`, `KindField`, `KindEnum`, `KindEnumMember`, `KindConstructor`, `KindProperty`)
- Convert to `protocol.CompletionItemKind` in a single adapter layer at the server boundary
- The adapter layer also handles snippet vs plaintext selection, UTF-16 position conversion, and additional text edit conversion
