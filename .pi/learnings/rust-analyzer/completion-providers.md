# Per-kind completion providers

### Expression completion
- `completions/expr.rs:1-400` — `complete_expr_path()`: handles qualified/unqualified expression paths
- `completions/expr.rs:200-300` — For `Qualified::No`: calls `ctx.process_all_names()` which iterates all names in scope including locals, adds keywords (match/while/loop/if/for/let/return/break/continue/true/false/unsafe/const), handles functional update fields
- `completions/expr.rs:300-400` — For `Qualified::With { resolution }`: dispatches on resolution type (Module → module scope, Adt/TypeAlias/BuiltinType → iterate path candidates, Trait → trait items, TypeParam/SelfType → type items)
- `completions/expr.rs:400-500` — For `Qualified::TypeAnchor`: iterates path candidates split by inherent/trait, handles enum variants through type aliases
- `completions/expr.rs:500-600` — `complete_expr()`: term search (expression synthesis from expected type)

### Function call completion (callable snippets)
- `render/function.rs:1-300` — `render_fn()` / `render_method()`: builds completion item for function calls
- `render/function.rs:100-200` — `add_call_parens()`: three modes based on `CallableSnippets` config:
  - `FillArguments`: generates snippet with `${1:param_name}` placeholders for each parameter
  - `AddParentheses`: generates `func_name($0)` without arg placeholders
  - `None` (no snippet cap): no parentheses added
- `render/function.rs:150-200` — Self-param handling: when completing assoc fn as `S::foo`, includes `${1:&self}` placeholder
- `render/function.rs:200-250` — `ref_of_param()`: checks if a local with matching name has compatible type for auto-inserting `&` or `&mut ` prefix
- `render/function.rs:250-300` — Semicolon completion: for unit-returning functions, appends `;` (or `,` in match arms) before `$0`
- `render/function.rs:300-350` — `params()`: skips parentheses if expected type is a function reference with matching signature
- `config.rs:30-40` — `CallableSnippets` enum: `FillArguments` | `AddParentheses`

### Function parameter completion
- `completions/fn_param.rs:1-150` — `complete_fn_param()`: for function params, scans all functions in file for repeated parameter patterns; for closure params, scans surrounding stmt list scope
- `completions/fn_param.rs:150-200` — `fill_fn_params()`: walks parent ancestors collecting fn params from SourceFile/ItemList/AssocItemList; deduplicates against existing params; adds self completions if applicable
- `completions/fn_param.rs:200-250` — `comma_wrapper()`: adds leading/trailing commas based on surrounding tokens

### Record field completion
- `completions/record.rs:1-100` — `complete_record_expr_fields()` / `complete_record_pattern_fields()`: gets missing fields from type, suggests them; for unions, shows all fields if none specified
- `completions/record.rs:100-150` — `add_default_update()`: adds `..` functional update completion for struct literals

### Snippet completion
- `completions/snippet.rs:1-100` — `complete_expr_snippet()` / `complete_item_snippet()`: adds built-in snippets (pd/ppd/macro_rules/tmod/tfn) and custom user snippets
- `completions/snippet.rs:100-150` — `add_custom_completions()`: iterates `config.prefix_snippets()`, adds each with imports and documentation
- `snippet.rs` — `Snippet` struct: `prefix_triggers`, `postfix_triggers`, `scope`, `body`, `description`, `imports`

### Postfix completion
- `completions/postfix/` — Postfix completions: `if`, `match`, `while`, `ref`, `refm`, `let`, `lete`, `letm`, `not`, `dbg`, `dbgr`, `call`
- `completions/postfix.rs` — `complete_postfix()`: triggered by `.` access, checks `config.postfix_snippets()`

### Flyimport completion
- `completions/flyimport.rs` — Auto-import completions: fuzzy matches importable items, adds `use` statement via `ImportScope::find_insert_use_container`
- `completions.rs:500-600` — `add_method_with_import()`: renders method with import path attached
