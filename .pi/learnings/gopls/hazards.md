# gopls hazards and stale-offset prevention

## Hazards in gopls's own design
- **No off-the-shelf jsonrpc2 library**: gopls uses its own `internal/jsonrpc2` and `internal/jsonrpc2_v2` rather than a third-party implementation.
- **Snapshot invalidation cancels ALL in-flight work**: `prevSnapshot.cancel()` (view.go:797) is aggressive — it cancels every request using the old snapshot. This is correct for gopls because requests re-acquire the new snapshot, but is aggressive behavior worth noting.
- **No per-request snapshot isolation**: gopls doesn't pin a snapshot per request — handlers acquire the current snapshot at call time. If the snapshot is invalidated mid-request, the handler's context gets cancelled.
- **`futureCache` requires retryable compute functions**: the contract says compute must be safely retryable and always return the same value. This is hard to guarantee in general.
- **`$/cancelRequest` is a notification, not a request**: it has no response. The handler must not reply with an error — it just calls `canceller(id)` and replies `nil, nil`.

## Completion-specific hazards
- **Completion always type-checks**: `NarrowestPackageForFile` is called unconditionally (`completion.go:516`). No lightweight path exists.
- **Budget is soft, not hard**: the budget deadline is checked only every 100 candidates in deep search, and only after minDepth. A fast completion may exceed budget by a small margin.
- **Snippet builder is a separate package**: `internal/golang/completion/snippet/` — must be imported separately. Not part of the protocol layer.
- **`SortText` hack**: positional index as sort text is a workaround for LSP issue #348. If the client doesn't support server-side ordering, this is needed.
- **No `ResolveProvider`**: gopls does NOT implement `completionItem/resolve`. All data is returned eagerly.
- **`methodSetCache` is per-request, not shared**: The `methodSetCache` map on the `completer` struct (`completion.go:280`) is created fresh for each request. If the same type is queried by multiple concurrent requests, each does its own `types.NewMethodSet` computation. This is acceptable because `types.NewMethodSet` is fast for typical types, but it means there's no cross-request sharing of method set computation.
- **`tooNewSymbolsCache` is per-request**: Similarly, the `tooNewSymbolsCache` map (`completion.go:285`) is per-request. Each completion request re-computes which symbols are too new for the file's Go version.
- **No precomputed completion index**: gopls explicitly chose not to precompute a completion-specific index. The comment in `completion.go:1310-1320` explains that the deep completion algorithm is "exceedingly complex and deeply coupled to the now obsolete notions that all token.Pos values can be interpreted by a single FileSet" — and that completion of unimported packages "cannot use the deep completion machinery which is based on type information" and instead uses "only syntax information from a quick parse."
- **`resolveInvalid` is a heuristic**: When the type checker produces an invalid type (common during editing), `resolveInvalid` (util.go:103) constructs a fake `*types.Named` with `types.Invalid` underlying type. This is a best-effort fallback — the fake type has no methods, so method completions won't work for incompletely typed expressions.

## Lease boundary hazards
- **Snapshot holds a reference to View**: `snapshot.go:72` — `view *View`. This poses lifecycle problems: a view may be shut down while work associated with this snapshot is still in flight. The comment at `snapshot.go:66-71` acknowledges this is not formalized.
- **`futureCache` requires retryable compute functions**: the contract (`future.go:80-82`) says compute must be safely retryable and always return the same value. This is hard to guarantee in general.
- **No per-request snapshot isolation**: gopls doesn't pin a snapshot per request — handlers acquire the current snapshot at call time. If the snapshot is invalidated mid-request, the handler's context gets cancelled.
- **`persistent.Map` values are reference-counted**: The `persistent.Map` (`internal/util/persistent/map.go`) reference-counts values. When a value's refcount reaches 0, the release function is called. This means values can be destroyed while a request holds a reference to the snapshot — but only if the snapshot itself is released. The snapshot's `Acquire`/`decref` pattern prevents this.

## Stale-offset prevention (DidModifyFiles → snapshot clone → FileOf → completion)
- **No deliberate use of prior snapshot**: gopls does NOT serve completion from a stale snapshot. Every semantic handler acquires the *current* snapshot at call time via `s.session.FileOf(ctx, uri)` (session.go:458).
- **DidModifyFiles invalidates before any handler runs**: `text_synchronization.go:DidChange` → `didModifyFiles` → `session.DidModifyFiles` (session.go:784). Inside, `updateOverlays` (session.go:837) writes the new content into the overlayFS *first*, then `invalidateViewLocked` (view.go:783) cancels the old snapshot and clones with new file handles.
- **Snapshot clone replaces file handles**: `snapshot.go:clone` (line 1512) calls `s.files.clone(changedFiles)` which calls `fileMap.clone` (filemap.go:33). For each changed URI, the old file handle is replaced with the new one (from the overlay).
- **Parse cache keyed by file hash**: `parse_cache.go:startParse` (line 319) uses `parseKey{uri, mode, purgeFuncBodies}` + `fh.Identity().Hash` as cache key. Content change → different hash → cache miss → re-parse → new `Mapper` with correct offsets.
- **Overlay-first reads**: `overlayFS.ReadFile` (fs_overlay.go:43) returns the overlay (editor buffer) if present, before falling back to disk. The snapshot's `lockedSnapshot.ReadFile` (snapshot.go:1116) checks the snapshot's `files` map first, then falls back to `view.fs.ReadFile` (which goes through overlayFS).
- **Snapshot cancellation**: `invalidateViewLocked` (view.go:797) calls `prevSnapshot.cancel()` to cancel all in-flight work on stale data. In-flight completions get `RequestCancelledError` via `replyWithDetachedContext` (protocol.go:226).
- **Completion reads file from snapshot**: `server/completion.go:28` calls `s.session.FileOf` → gets current snapshot + file handle. Then `NarrowestPackageForFile` (golang/snapshot.go:35) calls `snapshot.TypeCheck` which uses the snapshot's file handles. The returned `pgf.Mapper` (parsego/file.go:22) is built from `Src` at parse time, which matches the snapshot's file content.
- **Pattern summary**: overlayFS that shadows disk with editor buffers, snapshot-per-view with ref-counted lifecycle, snapshot clone that replaces file handles for changed URIs, parse cache keyed by file hash. `prevSnapshot.cancel()` cancels all in-flight work aggressively on every invalidation rather than isolating per-request.
