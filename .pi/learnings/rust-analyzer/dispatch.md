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

### Completion handler (LSP layer)
- `handlers/request.rs:1121-1160` — `handle_completion()`: gets `FilePosition`, `line_index`, `completion_config` from snapshot; calls `snap.analysis.completions()`; converts via `to_proto::completion_items()`; returns `CompletionList { is_incomplete: true, items }`
- `handlers/request.rs:1129` — FIXME: "We should fix up the position when retrying the cancelled request instead"
- `handlers/request.rs:1159` — `is_incomplete: true` always set (client may re-request on type)
- `handlers/request.rs:1163-1241` — `handle_completion_resolve()`: re-runs `completions()` at same position, finds matching item by label prefix + hash, resolves deferred fields

### Snapshot / stale analysis
- `global_state.rs:567-580` — `snapshot()` creates `GlobalStateSnapshot` containing `Analysis` (cloned `RootDatabase` — Salsa snapshot)
- `ide/src/lib.rs:148` — `Cancellable<T> = Result<T, Cancelled>`
- `ide/src/lib.rs:193-203` — `AnalysisHost::apply_change()` triggers cancellation on all outstanding snapshots
- `ide-db/src/apply_change.rs:11-14` — `RootDatabase::apply_change()` calls `trigger_cancellation()` before applying change
- `ide/src/lib.rs:930-948` — `Analysis::with_db()` wraps in `Cancelled::catch()` — Salsa cancellation via unwinding

### File edit handling
- `notification.rs:100-127` — `handle_did_change_text_document` updates `mem_docs` and VFS immediately on main thread
- `global_state.rs:333-530` — `process_changes()` called after each event loop turn: triggers cancellation in parallel, then applies changes to `AnalysisHost`
- `global_state.rs:350-355` — Cancellation runs in parallel via `cancellation_pool.scoped()`

### Version tracking
- `lsp/ext.rs:835-848` — `CompletionResolveData` stores `version: Option<i32>` alongside position and hash
- `lsp/to_proto.rs:421-443` — Version captured at completion time, stored in resolve data
- `global_state.rs:767-770` — `file_version()` returns version from `mem_docs`

### MemDocs (in-memory document state)
- `mem_docs.rs:1-80` — Tracks open documents with version and content bytes; driven by didOpen/didClose/didChange
