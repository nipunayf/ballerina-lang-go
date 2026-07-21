# gopls type-checking batching and no-compile policy

## Type-checking batching (futureCache)
- `typeCheckBatch` (check.go:54) — groups type-checking work, shares import graph across requests.
- `futureCache` (future.go:1-97) — key-value store of cancellable+retryable futures. Supports persistent and transient entries.
- `futureCache.get` (future.go:72-97): if value already computed, returns it. Otherwise joins or starts a computation. Uses 1-buffered `acquire` channel: first requester starts work; if cancelled, pushes unit back so another goroutine can retry.
- `typeCheckBatch` (check.go:54-76): groups type-checking work. Uses two `futureCache` instances: `syntaxPackages` (transient) and `importPackages` (persistent).
- `typeCheckBatch.query` (check.go:254-280): starts one goroutine per requested package via `errgroup.Group`. Each goroutine checks `ctx.Err()` before proceeding.
- `typeCheckBatch.getPackage` (check.go:351-445): calls `syntaxPackages.get(ctx, id, ...)`. Inside the compute function: waits for import dependencies via `getImportPackage` (which uses `importPackages.get`), acquires CPU token, then type-checks. If context cancelled, returns `ctx.Err()`.
- `typeCheckBatch.acquireTypeChecking` (check.go:217-240): ref-counted batch access. When refs hit 0, batch is discarded.

## No-compile request policy
- gopls does NOT have an explicit "no-compile" policy for any semantic request. All semantic queries (hover, completion, definition, references, signature help, semantic tokens) call `NarrowestPackageForFile` which triggers `snapshot.TypeCheck(ctx, mp.ID)`.
- The only "lightweight" path is `snapshot.ParseGo` for parse-only operations (e.g., folding range, document symbols, some code actions).
- `internal/golang/snapshot.go:selectPackageForFile` (line 95) — calls `snapshot.MetadataForFile` then `snapshot.TypeCheck`.
- `internal/cache/check.go:TypeCheck` (line 112) — checks for existing valid packages first (optimization), then joins/creates a `typeCheckBatch`.
- `internal/cache/check.go:acquireTypeChecking` (line 217) — joins or starts a concurrent type-checking batch with ref-counting.
- The `futureCache` (future.go) pattern allows multiple goroutines to share type-checking work and retry on cancellation.

## Request concurrency against didChange (no lease, no generation match)
- **No per-request snapshot isolation**: requests don't pin a snapshot; they acquire current snapshot at call time via `s.session.FileOf(ctx, uri)` (session.go:458), which returns the view's current `v.snapshot` pointer.
- **futureCache as the memoization engine** (future.go): a key-value store of cancellable+retryable futures, similar in spirit to rust-analyzer's Salsa. `futureCache.get(ctx, key, compute)` joins or starts a computation; 1-buffered `acquire` channel lets first requester run compute, others wait; on cancellation, pushes unit back so another goroutine can retry.
- **Racing scenario**: completion request arrives → gets current snapshot S1 → calls TypeCheck(ctx, snapshot) → checks package cache (line 160 of check.go, if valid, return immediately) → if not cached, joins or starts typeCheckBatch.query (check.go:172) which uses futureCache to share work.
- **After didChange**: old snapshot S0's context is cancelled via `prevSnapshot.cancel()` (view.go:799). Any in-flight typeCheckBatch.query on S0 checks `ctx.Err()` (check.go:276) and returns context.Cancelled. New request gets S1 (new snapshot) and starts fresh type-check or joins new batch.
- **No "lease" pattern**: contrast with rust-analyzer (cancel reads on write, no separate background compile) and LS's current design (precompute + generation-matched lease). gopls instead: snapshot + futureCache inside it, where requests join work transparently.

## Transferable patterns
- **`futureCache` retry-on-cancel**: the 1-buffered `acquire` channel pattern for retrying cancelled computations is clever and avoids wasted work.
- **Snapshot-local memoization**: each snapshot owns its own futureCache batch for type-checking, so invalidation (snapshot clone) naturally gives new snapshot a fresh batch.
