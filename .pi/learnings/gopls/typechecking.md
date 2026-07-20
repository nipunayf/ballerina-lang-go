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

## Transferable patterns
- **`futureCache` retry-on-cancel**: the 1-buffered `acquire` channel pattern for retrying cancelled computations is clever and avoids wasted work.
