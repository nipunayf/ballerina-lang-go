# Dispatch, snapshots, and file edits

### Dispatch
- `main_loop.rs:1290` — Completion dispatched via `on_latency_sensitive::<RETRY, ...>` (higher-priority threadpool, retry on cancel)
- `handlers/dispatch.rs:1-50` — `RequestDispatcher` with four dispatch modes: `on_sync_mut` (main thread, &mut), `on_sync` (main thread, snapshot), `on` (threadpool, VFS-ready guard, default fallback), `on_latency_sensitive` (higher-priority threadpool)
- `handlers/dispatch.rs:108-115` — `on::<ALLOW_RETRYING, R>` checks `vfs_done`; returns `R::Result::default()` if VFS not ready
- `handlers/dispatch.rs:126-220` — `on_latency_sensitive` captures `GlobalStateSnapshot`, spawns on `ThreadIntent::LatencySensitive` pool; on Salsa `Cancelled` with `ALLOW_RETRYING` → `Task::Retry(req)`, else → `ContentModified` error
- `handlers/dispatch.rs:240-280` — `on_with_vfs_default` variant for requests needing custom default + custom cancelled handler
- `handlers/dispatch.rs:280-310` — `on_identity` for requests where `Result = Params` (passthrough on VFS-not-ready)
- `handlers/dispatch.rs:304-309` — `content_modified_error()` returns `ErrorCode::ContentModified`
- `handlers/dispatch.rs:310-340` — `on_fmt_thread` for formatting (dedicated pool, never blocks on task thread)
- `main_loop.rs:791-793` — `Task::Retry(req)` re-dispatched via `on_request(req)` if not already cancelled by client
- `main_loop.rs:1292` — FIXME: "Retrying can make the result of this stale?"

### Retry-on-cancel mechanics
- `handlers/dispatch.rs:126-220` — The `on_with_thread_intent` method is the shared implementation for both `on` and `on_latency_sensitive`. It:
  1. Captures `self.global_state.snapshot()` (creates a `GlobalStateSnapshot` with a cloned `RootDatabase`)
  2. Spawns the handler on the appropriate thread pool (`ThreadIntent::Worker` or `ThreadIntent::LatencySensitive`)
  3. Wraps the handler in `panic::catch_unwind`
  4. On success: converts result to `Task::Response`
  5. On cancellation: if `ALLOW_RETRYING` is true, returns `Task::Retry(req)`; otherwise returns `Task::Response` with `ContentModified` error
- `main_loop.rs:785-800` — The main loop handles `Task::Retry(req)` by re-dispatching via `on_request(req)`. The request is re-parsed from the original JSON, so it gets a fresh snapshot.
- `handlers/dispatch.rs:108-115` — The `on` method (non-latency-sensitive) has an additional guard: if `!self.global_state.vfs_done`, it returns a default response immediately instead of spawning. This avoids queueing work when the VFS hasn't finished loading.
- **Key design:** The retry mechanism is coarse-grained: the entire request is re-dispatched from scratch. There is no partial result preservation across retries. The FIXME at `main_loop.rs:1292` acknowledges this can produce stale results.

### The `on_latency_sensitive` path for completion
- `main_loop.rs:1290` — `on_latency_sensitive::<RETRY, lsp_request::Completion>(handlers::handle_completion)`
- Completion is classified as latency-sensitive (higher priority than `on` requests like `goto_definition`).
- It uses `RETRY` (a const `true`), so cancelled completions are retried automatically.
- The `LatencySensitive` thread pool has fewer threads than the `Worker` pool, ensuring latency-sensitive requests don't get queued behind long-running worker tasks.
- `handlers/request.rs:1121-1160` — `handle_completion()`: gets `FilePosition`, `line_index`, `completion_config` from snapshot; calls `snap.analysis.completions()`; converts via `to_proto::completion_items()`; returns `CompletionList { is_incomplete: true, items }`
- `handlers/request.rs:1129` — FIXME: "We should fix up the position when retrying the cancelled request instead" — the position is clamped to `min(offset, line_index.index.len())` to handle the case where the file was shortened between retries.

### The `on` path (non-latency-sensitive)
- `handlers/dispatch.rs:108-115` — The `on` method checks `vfs_done` and returns `R::Result::default()` if not ready. This is a hard fallback, not a staged approach.
- `handlers/dispatch.rs:126-220` — The `on_with_thread_intent` method is shared between `on` and `on_latency_sensitive`. The only difference is the `ThreadIntent` (Worker vs LatencySensitive).
- **Key design:** The VFS-not-ready fallback returns a default (empty) response. There is no mechanism to queue the request and retry when VFS is ready. This is a deliberate tradeoff: the client can re-request if needed.

### Completion handler (LSP layer)
- `handlers/request.rs:1121-1160` — `handle_completion()`: gets `FilePosition`, `line_index`, `completion_config` from snapshot; calls `snap.analysis.completions()`; converts via `to_proto::completion_items()`; returns `CompletionList { is_incomplete: true, items }`
- `handlers/request.rs:1129` — FIXME: "We should fix up the position when retrying the cancelled request instead"
- `handlers/request.rs:1159` — `is_incomplete: true` always set (client may re-request on type)
- `handlers/request.rs:1163-1241` — `handle_completion_resolve()`: re-runs `completions()` at same position, finds matching item by label prefix + hash, resolves deferred fields

### Snapshot / stale analysis
- `global_state.rs:567-580` — `snapshot()` creates `GlobalStateSnapshot` containing `Analysis` (cloned `RootDatabase` — Salsa snapshot)
- `global_state.rs:207-220` — `GlobalStateSnapshot` struct: holds `config`, `analysis: Analysis`, `mem_docs`, `vfs`, `workspaces`, `flycheck`, etc. Implements `UnwindSafe`.
- `ide/src/lib.rs:148` — `Cancellable<T> = Result<T, Cancelled>`
- `ide/src/lib.rs:193-203` — `AnalysisHost::apply_change()` triggers cancellation on all outstanding snapshots
- `ide-db/src/apply_change.rs:11-14` — `RootDatabase::apply_change()` calls `trigger_cancellation()` before applying change
- `ide/src/lib.rs:930-948` — `Analysis::with_db()` wraps in `Cancelled::catch()` — Salsa cancellation via unwinding
- `ide/src/lib.rs:229-240` — `Analysis` struct: wraps cloned `RootDatabase`; all public methods go through `with_db()` which catches Salsa cancellation unwinding
- `ide/src/lib.rs:755-760` — `Analysis::completions()`: calls `self.with_db(|db| ide_completion::completions(db, config, position, trigger_character))` — thin delegation to ide-completion crate
- `ide/src/lib.rs:359-362` — `Analysis::file_line_index()`: calls `self.with_db(|db| db.line_index(file_id))`
- `ide/src/lib.rs:943-955` — `Analysis::with_db()`: uses `Cancelled::catch(|| f(&self.db))` — Salsa cancellation via Rust unwinding, caught at API boundary
- `hir/src/semantics.rs:161-170` — `Semantics<'db, DB>`: lightweight handle holding `&'db DB` + `SemanticsImpl` (with `SourceToDefCache` + `macro_call_cache`). Created per-request, cheap to construct.
- `hir/src/semantics.rs:2522-2530` — `SemanticsScope<'db>`: scoped handle with `db`, `file_id`, `resolver`. Returned by `scope_at_offset()`. Provides `module()`, `krate()`, `containing_function()`, `process_all_names()`, `visible_traits()`.
- `hir/src/source_analyzer.rs:71-80` — `SourceAnalyzer<'db>`: internal convenience wrapper with `resolver`, `body_or_sig`, `file_id`. Created by `SemanticsImpl::analyze_impl()` which finds the containing def and dispatches to `new_for_body_no_infer()` or `new_generic_def()`.
- `hir/src/source_analyzer.rs:109-116` — `new_for_body_no_infer()`: creates SourceAnalyzer **without** inference results — used for scope-only queries like completion
- `hir/src/source_analyzer.rs:118-145` — `new_for_body_()`: queries `db.expr_scopes(def)` and `db.body_with_source_map(def)` — both Salsa queries, cached
- `hir/src/semantics.rs:2116-2150` — `analyze_impl()`: finds container (DefWithBodyId, VariantId, TraitId, etc.) via `SourceToDefCache`, then creates appropriate SourceAnalyzer. This is the bridge from syntax position to semantic context.

### File edit handling
- `notification.rs:100-127` — `handle_did_change_text_document` updates `mem_docs` and VFS immediately on main thread
- `global_state.rs:333-530` — `process_changes()` called after each event loop turn: triggers cancellation in parallel, then applies changes to `AnalysisHost`
- `global_state.rs:350-355` — Cancellation runs in parallel via `cancellation_pool.scoped()`
- `global_state.rs:350-380` — `process_changes()`: spawns cancellation on `cancellation_pool` (1 thread), downgrades VFS write lock to upgradable, normalizes text, then applies changes. Cancellation is all-or-nothing — no fine-grained invalidation.

### Version tracking
- `lsp/ext.rs:835-848` — `CompletionResolveData` stores `version: Option<i32>` alongside position and hash
- `lsp/to_proto.rs:421-443` — Version captured at completion time, stored in resolve data
- `global_state.rs:767-770` — `file_version()` returns version from `mem_docs`

### MemDocs (in-memory document state)
- `mem_docs.rs:1-80` — Tracks open documents with version and content bytes; driven by didOpen/didClose/didChange
