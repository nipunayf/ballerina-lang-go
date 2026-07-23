# Database/snapshot completion access: eager indices vs request-time semantic queries

## Summary

rust-analyzer uses a **hybrid approach**: a Salsa-tracked semantic database for all
request-time queries, plus two precomputed FST-based indices for name-based item
lookup (flyimport and workspace symbols). Completion's core logic (scope
derivation, type inference, cursor classification) is entirely request-time via
Salsa queries — there is no completion-specific precomputed index. The
precomputed indices serve a narrow purpose: fast fuzzy/prefix/exact name matching
across modules and crates.

## The two precomputed indices

### 1. `SymbolIndex` (local workspace) — FST per module

- `crates/ide-db/src/symbol_index.rs:361-370` — `SymbolIndex` struct: `symbols: Box<[FileSymbol]>` + `map: fst::Map<Vec<u8>>`. The FST maps lowercased symbol names to ranges in the `symbols` array.
- `crates/ide-db/src/symbol_index.rs:404-412` — `module_symbols()` is a **Salsa-tracked query** (`#[salsa::tracked(returns(ref))]`). It calls `SymbolCollector::new_module()` to walk the module's HIR, collects all publicly visible items, builds an FST.
- `crates/ide-db/src/symbol_index.rs:189-192` — `crate_symbols()` collects all module indices for a crate: `krate.modules(db).into_iter().map(|module| SymbolIndex::module_symbols(db, module)).collect()`.
- `crates/ide-db/src/symbol_index.rs:460-500` — `SymbolIndex::new()`: sorts symbols by lowercased name, builds FST where each key maps to a `(start, end)` range in the sorted array. Multiple items with the same name (due to namespacing) share one FST entry.
- `crates/ide-db/src/symbol_index.rs:565-620` — `Query::search()`: builds an FST automaton (exact/fuzzy/prefix), runs union search across all provided indices, iterates matching ranges.

**Key design:** The FST is built from HIR data (`SymbolCollector` walks `ModuleDef`s), not from syntax. It is invalidated when the module's HIR changes (Salsa dependency tracking). The FST is rebuilt from scratch on each invalidation — there is no incremental FST update.

### 2. `ImportMap` (external dependencies) — FST per crate

- `crates/hir-def/src/import_map.rs:1-20` — "A map of all publicly exported items in a crate." Stores `ItemInNs → Vec<ImportInfo>` mapping.
- `crates/hir-def/src/import_map.rs:79-100` — `import_map_query()`: collects all publicly exported items for a crate, builds an FST. This is a **Salsa-tracked query** (`crates/hir-def/src/db.rs:236-237`: `#[salsa::invoke(ImportMap::import_map_query)] fn import_map(&self, krate: Crate) -> Arc<ImportMap>`).
- `crates/hir-def/src/import_map.rs:419-478` — `search_dependencies()`: queries the import maps of all dependencies of a crate using FST automaton union. Used by `Crate::query_external_importables()` (`crates/hir/src/lib.rs:290-300`).

**Key design:** The `ImportMap` is per-crate, not per-module. It only covers external dependencies (not the local workspace). The local workspace uses `SymbolIndex` instead. This split exists because dependencies are assumed to change rarely (higher Salsa durability).

## What is NOT precomputed

### Completion context is entirely request-time

- `crates/ide-completion/src/context.rs:700-870` — `CompletionContext::new()` creates a fresh `Semantics` handle per request, parses the file (with fake ident marker), navigates the CST directly for token/trivia, calls `sema.scope_at_offset()` (triggers `ExprScopes` Salsa query), calls `scope.process_all_names()` to enumerate locals, resolves expected types, etc. **No completion-specific cache is consulted.**
- `crates/ide-completion/src/context/analysis.rs:57-130` — `expand_and_analyze()`: inserts fake ident, expands macros speculatively, classifies cursor position. All of this is request-time.
- `crates/ide-completion/src/lib.rs:191-260` — `completions()`: creates `CompletionContext`, dispatches to providers based on `CompletionAnalysis` type. Each provider (expr, fn_param, record, snippet, postfix) operates on the context directly — no precomputed completion index.

### Scope derivation is a Salsa query, not an index

- `crates/hir-def/src/expr_store/scope.rs:54-95` — `ExprScopes::expr_scopes_query()`: computes scopes from body. This is a Salsa query, cached and invalidated on file change. It is **not** a precomputed index — it's a memoized computation.
- `crates/hir/src/source_analyzer.rs:109-116` — `new_for_body_no_infer()`: creates `SourceAnalyzer` **without** inference results for scope-only queries. This is the path completion takes — it deliberately avoids triggering type inference.

### Type inference is request-time (and avoided when possible)

- `crates/hir/src/source_analyzer.rs:118-145` — `new_for_body_()`: queries `db.expr_scopes(def)` and `db.body_with_source_map(def)`. The `body_with_source_map` query triggers type inference. Completion uses `new_for_body_no_infer()` to avoid this cost.
- Completion only triggers inference when it needs expected types (e.g., for `expected_type` matching in relevance scoring). This is a deliberate optimization.

## The flyimport path: how precomputed indices are used at request time

- `crates/ide-completion/src/completions/flyimport.rs:1-30` — Flyimport completion is the **only** completion path that uses precomputed indices. It calls `ImportAssets::for_fuzzy_path()` or `ImportAssets::for_fuzzy_method_call()`.
- `crates/ide-db/src/imports/import_assets.rs:115-200` — `ImportAssets` wraps an `ImportCandidate` (path or trait method) and a `SyntaxNode` for scope context.
- `crates/ide-db/src/imports/import_assets.rs:300-400` — `search_for()`: calls `items_locator::items_with_name()` which queries both `symbol_index` (local) and `import_map` (external) via FST, then filters by scope visibility.
- `crates/ide-db/src/items_locator.rs:30-100` — `items_with_name()`: builds both a `symbol_index::Query` (for local workspace) and an `import_map::Query` (for external dependencies), calls `find_items()` which runs both queries and chains results.

**Critical detail:** The FST indices only store **names** and **module paths**. They do NOT store types, scopes, or any semantic information beyond what's needed to find an item by name and construct its import path. All semantic filtering (visibility, trait bounds, type matching) happens at request time after the FST returns candidates.

## Eager precomputation: `prime_caches`

- `crates/ide-db/src/prime_caches.rs:1-30` — "rust-analyzer is lazy and doesn't compute anything unless asked. This sometimes is counter productive when, for example, the first goto definition request takes longer to compute. This module implements prepopulation of various caches."
- `crates/ide-db/src/prime_caches.rs:50-200` — `parallel_prime_caches()`: eagerly computes `crate_def_map`, `import_map`, `SymbolIndex::module_symbols`, and `TraitImpls::for_crate` for all crates in parallel. This runs at startup and after project reload.
- `crates/ide-db/src/prime_caches.rs:200-250` — The priming is **not completion-specific**. It primes the general Salsa caches that any feature might need. The `SymbolIndex` and `ImportMap` are primed as a side effect of this general cache warming.
- `crates/ide-db/src/prime_caches.rs:250-280` — For local crates, `module_symbols` (the FST index) is computed eagerly. For library crates, `import_map` is computed eagerly. This means the FST indices are warm before the first completion request.

## The snapshot model

- `crates/rust-analyzer/src/global_state.rs:567-580` — `snapshot()` creates `GlobalStateSnapshot` containing a cloned `RootDatabase` (Salsa snapshot). The clone is cheap because Salsa uses copy-on-write.
- `crates/ide/src/lib.rs:229-240` — `Analysis` wraps the cloned `RootDatabase`. All public methods go through `with_db()` which catches Salsa cancellation unwinding.
- `crates/ide/src/lib.rs:755-760` — `Analysis::completions()`: `self.with_db(|db| ide_completion::completions(db, config, position, trigger_character))` — thin delegation.
- `crates/ide/src/lib.rs:930-948` — `Analysis::with_db()`: wraps in `Cancelled::catch()` — Salsa cancellation via Rust unwinding, caught at API boundary.

**Key implication for a shared API:** The snapshot is a consistent point-in-time view. All Salsa queries (including the precomputed FST indices) are resolved against this snapshot. When a change arrives, all snapshots are cancelled, and new requests get a fresh snapshot. The precomputed indices are **not** separate from the snapshot — they are Salsa-tracked queries within it.

## Transferable cautions for a "shared API" decision

### 1. The FST indices are narrow — they solve one problem well

The `SymbolIndex` and `ImportMap` only solve **name-based item lookup**. They store no types, no scopes, no inference results. This is a deliberate design choice: the FST is fast for fuzzy/prefix/exact name matching, but it would be the wrong tool for anything more complex. If a "shared API" target is considering precomputed completion indices, the question is: what specific query pattern justifies the index? rust-analyzer's answer is "only name-based item search for flyimport."

### 2. Precomputed indices are Salsa-tracked, not separate

Both `SymbolIndex` and `ImportMap` are Salsa queries (`#[salsa::tracked(returns(ref))]`). They are invalidated and recomputed automatically when their inputs change. They are not standalone files or separate processes. This means:
- **Consistency is automatic**: the index is always consistent with the database snapshot.
- **Recomputation is lazy by default**: the index is only computed when first queried (or eagerly by `prime_caches`).
- **No separate invalidation logic**: Salsa handles dirtying.

For a Go LS with a shared API target, this means: if you build a precomputed index, you need to solve invalidation and consistency yourself. rust-analyzer gets this for free from Salsa.

### 3. The local/external split is a durability optimization

Local workspace uses `SymbolIndex` (per-module FST), external dependencies use `ImportMap` (per-crate FST). This split exists because:
- `crates/ide-db/src/symbol_index.rs:404-412` — `module_symbols` is a Salsa query with **no explicit durability hint** (defaults to low durability, invalidated on any change).
- `crates/hir-def/src/db.rs:236-237` — `import_map` is a Salsa query that depends on crate metadata, which has **high durability** (only invalidated when dependencies change).

This means: for a shared API, if you have a "compiled package" that changes rarely, you can cache its index at higher durability. If you have local source files that change frequently, their index needs lower durability.

### 4. The FST is rebuilt from scratch on invalidation

`crates/ide-db/src/symbol_index.rs:460-500` — `SymbolIndex::new()` sorts all symbols and builds a new FST from scratch. There is no incremental FST update. The `fst` crate does not support cheap updates. This is acceptable because:
- Module-level FSTs are small (one module's symbols).
- Rebuilding is fast (sort + FST build).
- Salsa only invalidates the specific module that changed.

For a Go LS, if you build an index per package, rebuilding from scratch on package changes is likely fine. If you build a global index, incremental updates become important.

### 5. Completion does NOT use the FST indices for core logic

The core completion pipeline (scope derivation, cursor classification, expression completion, function parameter completion, snippet completion, postfix completion) **never touches** `SymbolIndex` or `ImportMap`. These are only used for flyimport (auto-import) completion. This is a critical architectural point: the precomputed indices are an **add-on** to the core completion, not the foundation.

- `crates/ide-completion/src/completions/expr.rs` — Expression completion uses `ctx.process_all_names()` (scope enumeration) and `ctx.sema` (type queries). No FST.
- `crates/ide-completion/src/completions/fn_param.rs` — Function parameter completion walks the syntax tree. No FST.
- `crates/ide-completion/src/completions/record.rs` — Record field completion uses type information from `ctx.sema`. No FST.
- `crates/ide-completion/src/completions/snippet.rs` — Snippet completion uses config. No FST.
- `crates/ide-completion/src/completions/postfix.rs` — Postfix completion uses config. No FST.

### 6. The "no inference" path is a deliberate optimization

`crates/hir/src/source_analyzer.rs:109-116` — `new_for_body_no_infer()` creates a `SourceAnalyzer` without inference results. Completion uses this path for scope derivation. Type inference is only triggered when needed (expected type matching for relevance scoring).

This means: for a shared API, if completion can get scope information without triggering full type inference, that's a significant performance win. rust-analyzer achieves this by having `ExprScopes` as a separate Salsa query from type inference.

### 7. The `prime_caches` pattern is optional but important for UX

`crates/ide-db/src/prime_caches.rs` — Eager precomputation of caches at startup. This is not required for correctness (Salsa is lazy), but it prevents the first completion request from being slow. The priming is parallelized across worker threads.

For a Go LS with a shared API: if the shared API target is always available (e.g., a running compiler daemon), you might not need eager priming — the first request can trigger lazy computation. But if the shared API is a separate process that needs to be warmed up, eager priming may be necessary.

### 8. The `process_changes` cancellation pattern

`crates/rust-analyzer/src/global_state.rs:333-530` — `process_changes()` is the central change-application function. It:
1. Takes a write lock on the VFS
2. Spawns cancellation on a dedicated `cancellation_pool` (1 thread) — this runs in parallel with text normalization
3. Downgrades the VFS write lock to upgradable (allowing other readers during text normalization)
4. Normalizes text (line endings) for each changed file
5. Upgrades back to write lock
6. Applies changes to `AnalysisHost` via `self.analysis_host.apply_change(change)`
7. The `apply_change` call triggers `RootDatabase::apply_change()` which calls `trigger_cancellation()` before applying the change

**Key design:** Cancellation is all-or-nothing — when a change arrives, all in-flight Salsa queries are cancelled. There is no mechanism to preserve position-independent semantic facts across edits. The cancellation runs in parallel with text normalization to minimize latency.

### 9. The `Analysis`/`AnalysisHost` split

`crates/ide/src/lib.rs:193-203` — `AnalysisHost` owns the `RootDatabase` and provides `analysis()` (creates a snapshot) and `apply_change()` (mutates state, cancels snapshots).
`crates/ide/src/lib.rs:229-240` — `Analysis` is a snapshot: wraps a cloned `RootDatabase`. All public methods go through `with_db()` which catches Salsa cancellation unwinding.
`crates/ide/src/lib.rs:943-955` — `Analysis::with_db()`: uses `Cancelled::catch(|| f(&self.db))` — Salsa cancellation via Rust unwinding, caught at API boundary.

**Implication for a Go LS:** The `Analysis`/`AnalysisHost` split is a general pattern: one mutable owner of the semantic state, and immutable snapshots for request handling. In Go, this would map to a read-write lock or copy-on-write pattern, without the Salsa unwinding mechanism.

## Contradictions and gaps

### Contradiction: "lazy" vs "eager" in practice

The code comments in `prime_caches.rs:1-5` say "rust-analyzer is lazy and doesn't compute anything unless asked." But in practice, `parallel_prime_caches()` eagerly computes `crate_def_map`, `import_map`, and `module_symbols` for all crates at startup. The laziness is a **design principle** that is **partially violated** for UX reasons. The violation is explicit and controlled.

### Gap: no completion-specific index

There is no index that maps "cursor position in file" to "likely completions." rust-analyzer does not precompute completion candidates for any position. Every completion request starts from scratch: parse the file, build context, classify cursor, run providers. The only precomputed data that completion uses is the name-based item lookup for flyimport.

### Gap: no incremental FST update

The `fst` crate does not support incremental updates. When a module changes, its entire `SymbolIndex` is rebuilt. This is acceptable because module-level FSTs are small, but it means the approach doesn't scale to a single global FST for a large workspace.

## The `Semantics`/`SemanticsScope` ownership and lifetime pattern

### `Semantics<'db, DB>` — lightweight per-request handle

- `crates/hir/src/semantics.rs:161-170` — `Semantics<'db, DB>` struct: holds `&'db DB` (borrowed database reference) + `SemanticsImpl<'db>` (with `SourceToDefCache` + `macro_call_cache`). Created per-request, cheap to construct.
- `crates/hir/src/semantics.rs:209-213` — `Semantics::new(db: &DB)`: creates a new `SemanticsImpl` and wraps it. The `SemanticsImpl::new()` initializes empty caches (`RefCell<SourceToDefCache>`, `RefCell<FxHashMap<...>>`).
- `crates/hir/src/semantics.rs:166-170` — `SemanticsImpl<'db>`: holds `&'db dyn HirDatabase` + two `RefCell` caches. The `RefCell` caches are populated lazily as the user calls methods like `parse()`, `scope_at_offset()`, `resolve_path()`.
- **Key design:** `Semantics` is a short-lived handle, not a long-lived object. It is created fresh for each completion request in `CompletionContext::new()`. The caches are per-handle, not global — they are discarded when the handle is dropped.

### `SemanticsScope<'db>` — scoped semantic context

- `crates/hir/src/semantics.rs:2522-2530` — `SemanticsScope<'db>` struct: holds `&'db dyn HirDatabase`, `HirFileId`, `Resolver<'db>`. Returned by `scope_at_offset()`.
- `crates/hir/src/semantics.rs:2064-2075` — `scope_at_offset()`: calls `analyze_with_offset_no_infer()` → `SourceAnalyzer::new_for_body_no_infer()`, then extracts `file_id` and `resolver` from the `SourceAnalyzer` to build a `SemanticsScope`.
- `crates/hir/src/semantics.rs:2540-2560` — `SemanticsScope::process_all_names()`: calls `self.resolver.names_in_scope(self.db)` to enumerate all visible names. Used by completion to populate `locals`.
- **Key design:** `SemanticsScope` does **not** hold a reference to the syntax tree — it's purely semantic (resolver + file_id). The syntax tree is owned by the caller and passed in as `&SyntaxNode` to `scope_at_offset()`. This means the scope handle is independent of the syntax tree lifetime.

### `SourceAnalyzer<'db>` — internal bridge

- `crates/hir/src/source_analyzer.rs:71-80` — `SourceAnalyzer<'db>` struct: holds `HirFileId`, `Resolver<'db>`, `Option<BodyOrSig<'db>>`. Created by `SemanticsImpl::analyze_impl()`.
- `crates/hir/src/source_analyzer.rs:109-116` — `new_for_body_no_infer()`: creates `SourceAnalyzer` **without** inference results — used for scope-only queries like completion. Queries `db.body_with_source_map(def)` and `db.expr_scopes(def)` (both Salsa queries) but does **not** query `InferenceResult::for_body(db, def)`.
- `crates/hir/src/source_analyzer.rs:118-145` — `new_for_body_()`: the shared implementation. Queries `db.body_with_source_map(def)` and `db.expr_scopes(def)`, then calls `scope_for_offset()` to find the containing scope by matching expression text ranges to offset.
- `crates/hir/src/source_analyzer.rs:118-145` — The `infer` parameter is `Option<&'db InferenceResult>`. When `None` (the no-infer path), the `SourceAnalyzer` simply doesn't have inference results available — methods that need types will panic or return `None`.
- **Key design:** The `SourceAnalyzer` is internal to the `hir` crate and not exposed to completion. Completion only sees `SemanticsScope`. The `SourceAnalyzer` is consumed to extract the `Resolver` for the `SemanticsScope`.

### Ownership/lifetime chain for a completion request

1. `Analysis::completions()` (`crates/ide/src/lib.rs:755-760`) — takes `&self` (the `Analysis` snapshot), calls `self.with_db(|db| ide_completion::completions(db, ...))`.
2. `ide_completion::completions()` (`crates/ide-completion/src/lib.rs:191-196`) — takes `&RootDatabase` directly.
3. `CompletionContext::new()` (`crates/ide-completion/src/context.rs:700-870`) — creates `Semantics::new(db)` (borrows `db`), parses file, calls `sema.scope_at_offset()` which returns `SemanticsScope<'db>` (borrows `db` via the resolver).
4. The `CompletionContext` struct holds `sema: Semantics<'db, RootDatabase>` and `scope: SemanticsScope<'db>` — both borrow `db` with lifetime `'db`.
5. Providers iterate `ctx.locals` (populated from `scope.process_all_names()`) and call `ctx.sema` methods for type queries.
6. When `completions()` returns, the `CompletionContext` is dropped, releasing the borrow on `db`.

**Implication for a Go LS:** The lifetime chain is: `Analysis` → `&RootDatabase` → `Semantics` → `SemanticsScope`. Each layer borrows from the previous. In Go, this would be a short-lived handle pattern: create a context struct per request, populate it from the compiled package, use it, discard it. No long-lived completion state.

## What generalizes to any LSP server

1. **Name-based fuzzy search via FST** — The pattern of building an FST from collected symbol names and using automaton intersection for fuzzy/prefix/exact search is protocol-level and generalizes. The `fst` crate is Rust-specific, but the concept (build a finite state transducer from names, query with automata) is language-agnostic.

2. **Two-tier search (local + external)** — Separating frequently-changing local data from rarely-changing dependency data, with different caching strategies for each, is a general pattern.

3. **Completion as request-time context construction + provider dispatch** — The pattern of building a rich context struct from syntax + semantics, then dispatching to per-kind providers, is protocol-level and generalizes.

4. **Avoiding type inference for scope-only queries** — If your semantic model separates scope information from type information, completion can get scopes without triggering full type inference. This is a general optimization.

## What is idiomatic Rust and doesn't translate directly

1. **Salsa's automatic invalidation and snapshot model** — Salsa's copy-on-write database clone, automatic dependency tracking, and cancellation via unwinding are deeply Rust-specific. A Go LS would need to implement its own invalidation and snapshot mechanism.

2. **`#[salsa::tracked(returns(ref))]` for index queries** — The pattern of making an index a Salsa-tracked query that returns a reference to a computed value is Rust-specific. In Go, you'd use a different caching mechanism (e.g., `sync.Map` with version stamps).

3. **FST automaton composition** — The `fst` crate's ability to compose automata (union of FST searches) is a Rust library feature. The concept generalizes, but the implementation is Rust-specific.

4. **Parallel cache priming with `rayon`** — The parallel computation of per-module FSTs using `rayon` is Rust-specific. The concept (parallelize independent index builds) generalizes.
