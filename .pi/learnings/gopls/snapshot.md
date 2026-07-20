# gopls snapshot lifecycle

## Snapshot acquisition pattern
- Every semantic handler calls `s.session.FileOf(ctx, uri)` → returns `(file.Handle, *Snapshot, func(), error)` — the `func()` is the release function, always deferred.
- `internal/cache/session.go:FileOf` (line 458) calls `SnapshotOf` then `snapshot.ReadFile`.
- `internal/cache/session.go:SnapshotOf` (line 382) finds the best View for a URI, then calls `view.Snapshot()`.
- `internal/cache/view.go:Snapshot` (line 589) returns `(v.snapshot, v.snapshot.Acquire(), nil)` — snapshots are ref-counted.
- `internal/cache/snapshot.go:Snapshot` struct (line 54) has `refcount`, `cancel`, `backgroundCtx`.
- `internal/cache/snapshot.go:clone` (line 1512) creates new snapshot with `context.WithCancel(bgCtx)` — each snapshot has its own cancel function.

## Snapshot lifecycle and cancellation
- **Snapshot struct** (snapshot.go:62-142): has `cancel func()`, `backgroundCtx context.Context`, `refcount int`, `done func()`.
- **`Snapshot.Acquire()`** (snapshot.go:216-225): increments refcount, returns `s.decref` as release function.
- **`Snapshot.decref()`** (snapshot.go:228-246): decrements refcount; when 0, destroys maps and calls `s.done()` (the `snapshotWG.Done` from session).
- **`Snapshot.clone()`** (snapshot.go:1512-1591): creates new snapshot with `context.WithCancel(bgCtx)`, clones persistent maps, inherits refcount=1. Old snapshot's cancel is NOT called here.
- **`Session.invalidateViewLocked`** (view.go:783-811): calls `prevSnapshot.cancel()` (line 797) to cancel all in-flight work on stale data, then clones. The old snapshot's `decref()` is called after clone (line 811).
- **`View.shutdown()`** (view.go:495-513): calls `v.snapshot.cancel()` and `v.snapshot.decref()`, sets `v.snapshot = nil`.
- **`Session.Shutdown`** (session.go:89-98): shuts down all views, stops parse cache, then `snapshotWG.Wait()` — waits for all snapshot refs to release.

## Transferable patterns
- **Snapshot ref-counting**: `Acquire()`/`decref()` pattern with deferred release is simple and prevents use-after-free.
