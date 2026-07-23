# Completion context construction and scope

### Completion pipeline (ide-completion)
- `ide-completion/src/lib.rs:191-196` — `completions()` takes `db`, `config`, `position`, `trigger_character`
- `ide-completion/src/lib.rs:191-260` — creates `CompletionContext`, dispatches to handlers based on `CompletionAnalysis` type
- `ide-completion/src/lib.rs:200-260` — Match on `CompletionAnalysis` variants: `Name` → `complete_name`, `NameRef` → `complete_name_ref`, `Lifetime` → `complete_label`/`complete_lifetime`, `String` → extern_abi/format_string/env_vars/ra_fixture, `UnexpandedAttrTT` → attribute, `MacroSegment` → macro_def
- `ide-completion/src/lib.rs:220-230` — Special case: `(` trigger only completes vis paths; `_` trigger suppresses completion for trivial type/pattern paths

### Completion context construction
- `context.rs:501-900` — `CompletionContext` struct fields: `sema`, `scope`, `db`, `config`, `position`, `trigger_character`, `original_token`, `token`, `krate`, `module`, `containing_function`, `is_nightly`, `edition`, `expected_name`, `expected_type`, `qualifier_ctx`, `locals`, `depth_from_crate_root`, `exclude_flyimport`, `exclude_traits`, `complete_semicolon`
- `context.rs:700-900` — `CompletionContext::new()`:
  1. Creates `Semantics::new(db)` — lightweight handle, cheap per-request
  2. Parses file with fake ident marker (`COMPLETION_MARKER`) for valid parse tree via `parse.reparse()`
  3. Gets `original_token` via `original_file.syntax().token_at_offset(offset).left_biased()` — direct CST navigation
  4. Calls `expand_and_analyze()` for macro expansion + cursor classification
  5. Gets `scope` via `sema.scope_at_offset(&token.parent()?, original_offset)` — uses current syntax tree (with fake ident)
  6. Populates `locals` via `scope.process_all_names()` — iterates all names in scope
  7. Scope derived from `ExprScopes` (computed from body/syntax tree)
- `context.rs:777-785` — `locals` populated by filtering `ScopeDef::Local` from `process_all_names`
- `context.rs:850-870` — `complete_semicolon` determined by checking next non-trivia token after terminator node
- `context.rs:724` — `original_token` obtained via `token_at_offset(offset).left_biased()` — always picks token immediately left of cursor
- `context.rs:728-747` — Token boundary validation: skips completion on invalid colons (`:` without `::`), checks `prev_token()` for `::` pattern, handles macro matcher context

### Scope derivation
- `semantics.rs:2064-2075` — `scope_at_offset()` → `analyze_with_offset_no_infer()` → `SourceAnalyzer::new_for_body_no_infer()`
- `source_analyzer.rs:109-116` — `new_for_body_no_infer()` creates SourceAnalyzer **without** inference results
- `source_analyzer.rs:118-145` — `new_for_body_()` uses `db.expr_scopes(def)` and `scope_for_offset()` to find scope
- `source_analyzer.rs:1545-1600` — `scope_for_offset()` finds containing scope by matching expression text ranges to offset
- `hir-def/src/expr_store/scope.rs:54-95` — `ExprScopes::expr_scopes_query()` computes scopes from body (Salsa query, cached, invalidated on file change)

### Completion analysis dispatch (cursor classification)
- `context/analysis.rs:57-130` — `expand_and_analyze()`: inserts fake ident, expands macros, calls `analyze()`
- `context/analysis.rs:57-96` — `expand_and_analyze()`: takes `sema: &Semantics`, `original_file: InFile<SyntaxNode>`, `speculative_file`, `offset`, `original_token`. Uses `right_biased()` on speculative file to find fake ident, computes relative offset, expands macros, then calls `analyze()`.
- `context/analysis.rs:439-638` — `analyze()`: classifies cursor position into `CompletionAnalysis` variants: `Name`, `NameRef`, `Lifetime`, `String`, `UnexpandedAttrTT`, `MacroSegment`
- `context/analysis.rs:439-450` — `analyze()` signature: takes `sema: &Semantics`, `expansion_result`, `original_token`, `self_token`. Returns `(CompletionAnalysis, (Option<Type>, Option<NameOrNameRef>), QualifierCtx)`.
- `context/analysis.rs:460-470` — Token kind validation: returns `None` if `original_token.kind() != self_token.kind()` (with exceptions for punct/trivia/identifier cross-matches)
- `context/analysis.rs:480-510` — Derive macro special case: if `derive_ctx` is present, classifies as `CompletionAnalysis::NameRef` with `PathKind::Derive`
- `context/analysis.rs:510-540` — Fallback when no `NameLike` found: checks for `String` (string completion), `UnexpandedAttrTT` (attribute input), `MacroSegment` (macro definition)
- `context/analysis.rs:540-570` — Main dispatch: matches `NameLike` variants → `Lifetime` → `classify_lifetime()`, `NameRef` → `classify_name_ref()`, `Name` → `classify_name()`
- `context/analysis.rs:905-1500` — `classify_name_ref()`: determines `PathCompletionCtx` with `qualified` (No/With/TypeAnchor/Absolute), `kind` (Expr/Type/Attr/Derive/Item/Pat/Vis/Use), `has_call_parens`, `has_macro_bang`, `parent`, `path`, `has_type_args`
- `context/analysis.rs:905-920` — `classify_name_ref()`: first checks `RecordExprField::for_field_name()`, `RecordPatField::for_field_name_ref()`, then `FieldExpr`/`MethodCallExpr` for dot-access, then `PathSegment` for path completion
- `context/analysis.rs:1500-1900` — `classify_name()`: determines `NameContext` with `NameKind` (Const/Enum/Function/IdentPat/etc.)
- `context/analysis.rs:1500-1520` — `classify_name()`: checks `LetStmt`, `FnDef`, `ConstDef`, `EnumVariant`, `TypeAlias`, `RecordField`, `IdentPat`, `SelfParam`, `Label` — each maps to a `NameKind` variant
