# gopls completion: on-demand type information, no precomputed semantic indexes

## Key finding: completion derives everything from the compiled package on demand

gopls does **not** precompute semantic indexes for completion. Every completion
request triggers type-checking of the enclosing package (if not already cached),
then derives candidates directly from the live `*types.Package` and `*types.Info`
structures. The only precomputed/lazily-computed indexes (`xrefs`, `methodsets`,
`tests`) are used by other features (references, diagnostics), **not** by completion.

## Dispatch path: server → snapshot → type-check → completer

```
server.Completion()                          # internal/server/completion.go:28
  → s.session.FileOf(ctx, uri)              # acquires snapshot + release func
  → completion.Completion(ctx, snapshot, fh, pos, context)  # internal/golang/completion/completion.go:514
    → golang.NarrowestPackageForFile()       # internal/golang/snapshot.go:35
      → snapshot.MetadataForFile()           # get package metadata
      → snapshot.TypeCheck(ctx, mp.ID)      # ON-DEMAND type-checking
    → pkg.TypesInfo()                        # live *types.Info
    → pkg.Types()                            # live *types.Package
    → completer{...}                         # per-request state struct
    → collectCompletions()                   # dispatch by context
    → deepSearch()                           # breadth-first deep completion
    → sortItems()                            # score-descending stable sort
```

## On-demand type-checking: `snapshot.TypeCheck`

**File:** `internal/cache/check.go:112`

`TypeCheck` is the central on-demand type-checking entry point. It:

1. **Checks for existing cached packages** (check.go:138-155): Before doing any
   work, it checks `s.packages.Get(id)` for an already-type-checked package
   handle with `state >= validPackage`. If found, returns immediately — no
   redundant type-checking. This is critical because after a file change, many
   LSP requests (semantic tokens, code lens, inlay hints, completion) all
   trigger type-checking of the same modified package.

2. **Joins or starts a type-checking batch** (check.go:217-240):
   `acquireTypeChecking()` returns a shared `typeCheckBatch` with ref-counting.
   Multiple goroutines can join the same batch, sharing parsed files and import
   resolution. The batch is released when all participants are done.

3. **Uses `futureCache` for work sharing** (check.go:34-100 in future.go):
   - `syntaxPackages` (transient): caches in-progress syntax package
     computation. Once all awaiters have received the result, the entry is
     evicted (transient=true). This prevents memory buildup from many concurrent
     requests for different packages.
   - `importPackages` (persistent): caches import results for the batch's
     lifetime. Imports are reused across packages in the same batch.

4. **Cancellable and retryable futures** (future.go:34-100): If the context
   used to compute a future is cancelled, the computation aborts and another
   awaiting goroutine can retry. This is essential for gopls's model where
   snapshot invalidation cancels all in-flight work on stale snapshots.

5. **CPU concurrency limiting** (check.go:395-400): A buffered channel of
   `runtime.GOMAXPROCS(0)` tokens limits CPU-bound type-checking. The token is
   acquired only after awaiting predecessors, to avoid starvation.

6. **Export data caching** (check.go:301-310): After type-checking, results are
   serialized to a file cache (`filecache.Set`). Future imports can load from
   export data instead of re-type-checking. This is the "precomputed" part —
   but it's export data for import, not semantic indexes for completion.

## What completion gets from the compiled package

After `NarrowestPackageForFile` returns, completion has:

- **`pkg.Types()`** → `*types.Package` with full type information for all
  package-level declarations. Used by `packageMembers()` (completion.go:1578)
  which iterates `pkg.Scope().Names()` to find exported symbols.

- **`pkg.TypesInfo()`** → `*types.Info` with `Defs`, `Uses`, `Types`, `Scopes`
  maps. Used throughout:
  - `info.Defs[n]` to check if an identifier is a definition (completion.go:560)
  - `info.Types[sel.X]` to get the type of a selector expression (completion.go:1241)
  - `info.Uses[id]` to resolve qualified identifiers (completion.go:1250)
  - `info.Scopes[n]` to get lexical scopes (completion.go:600)

- **`pkg.Metadata()`** → package metadata (imports, files, etc.)

- **`pkg.File(uri)`** → parsed file with AST, token.File, and Mapper

## How completion uses type information: on-demand, not precomputed

### Method sets: computed on demand per request

**File:** `internal/golang/completion/completion.go:1601-1635`

`methodsAndFields()` calls `types.NewMethodSet(typ)` directly — this is the
standard Go type checker method set computation, not a precomputed index. The
result is cached in a per-request `methodSetCache` map on the `completer`
struct, so repeated lookups for the same type within one completion request
are fast, but there is no cross-request cache.

```go
mset := c.methodSetCache[methodSetKey{typ, addressable}]
if mset == nil {
    if addressable && !types.IsInterface(typ) && !isPointer(typ) {
        mset = types.NewMethodSet(types.NewPointer(typ))
    } else {
        mset = types.NewMethodSet(typ)
    }
    c.methodSetCache[methodSetKey{typ, addressable}] = mset
}
```

### Lexical scope traversal: live `types.Scope` iteration

**File:** `internal/golang/completion/completion.go:1667-1787`

`lexical()` iterates `c.scopes` (built from `info.Scopes[n]` during
initialization) innermost-first, calling `scope.Names()` and
`scope.LookupParent(name, c.pos)` to find visible objects. For objects with
invalid types, it calls `resolveInvalid()` (util.go:103) which constructs a
fake type from the AST — a fallback for incomplete code.

### Selector completion: live type dispatch

**File:** `internal/golang/completion/completion.go:1239-1339`

`selector()` checks `c.pkg.TypesInfo().Types[sel.X]` for the type of the
selector expression's operand. If present (true selector), it calls
`methodsAndFields()` on that type. If absent (qualified identifier), it
resolves via `info.Uses[id]` or falls back to unimported completion.

### Unimported completion: metadata + goimports scoring

**File:** `internal/golang/completion/completion.go:1841-1921`

`unimportedPackages()` uses `snapshot.AllMetadata()` to find candidate
packages, then calls `snapshot.RunProcessEnvFunc()` to run goimports' scoring
algorithm. This is the only completion path that uses metadata rather than
type-checked information — and it's explicitly a fallback for packages not yet
imported.

## Precomputed indexes: what exists and what completion does NOT use

The `syntaxPackage` struct (package.go:38-120) has three lazily-computed indexes:

| Index | File | Used by completion? |
|-------|------|---------------------|
| `xrefs` (cross-references) | `xrefs.Index()` | **No** — used by references, implementations |
| `methodsets` | `methodsets.NewIndex()` | **No** — used by diagnostics, hover |
| `tests` | `testfuncs.NewIndex()` | **No** — used by test UI, code lens |

All three use `sync.Once` for lazy initialization and are cached to disk via
`storePackageResults()` (check.go:420-440). Completion deliberately does not
use them — it works directly with `types.Package` and `types.Info`.

## Snapshot read interfaces: the safe shared-boundary pattern

### `Session.FileOf` — the standard acquisition pattern

**File:** `internal/cache/session.go:458`

Every semantic handler (including completion) calls:
```go
fh, snapshot, release, err := s.session.FileOf(ctx, uri)
defer release()
```

This returns:
- `file.Handle` — content, hash, version of the file
- `*Snapshot` — the current snapshot for the file's view
- `func()` — release function (decrements snapshot refcount)

### `Snapshot.ReadFile` — file content access

**File:** `internal/cache/snapshot.go:1086`

Returns a `file.Handle` for any URI. Uses a `fileMap` (persistent map) for
cached entries, falling back to the view's `fs.ReadFile`. The `lockedSnapshot`
wrapper (snapshot.go:1101) ensures the map is accessed under `s.mu`.

### `Snapshot.ParseGo` — parse without type-checking

**File:** `internal/cache/parse.go:21`

For callers that only need syntax (not types), `ParseGo` parses a file using
the shared `parseCache`. Completion does NOT use this — it goes through
`NarrowestPackageForFile` which does both parse and type-check.

### `Snapshot.MetadataForFile` — metadata without type-checking

**File:** `internal/cache/snapshot.go:673`

Returns package metadata for a file. Used by `NarrowestPackageForFile` as the
first step before type-checking. Also used directly by features that only need
metadata (e.g., document symbols).

## Safe shared-boundary patterns

### 1. Snapshot ref-counting

**File:** `internal/cache/snapshot.go:216-246`

`Acquire()` increments refcount, returns `decref` as release. When refcount
reaches 0, maps are destroyed and `done()` is called (signals
`snapshotWG.Done()` in session). This prevents use-after-free when a snapshot
is invalidated while requests are still in flight.

### 2. Persistent maps for immutable state

**File:** `internal/util/persistent/map.go`

The `persistent.Map[K, V]` is a persistent treap that can be cloned in O(1)
time. Snapshot clone (snapshot.go:1512) clones `s.packages`, `s.files`, and
other maps — the new snapshot shares structure with the old one until
modified. This is the foundation of gopls's copy-on-write snapshot model.

### 3. `futureCache` for work sharing

**File:** `internal/cache/future.go:34-100`

As described above: cancellable, retryable futures that allow multiple
goroutines to share type-checking work. Transient futures (syntax packages)
are evicted after all awaiters receive the result, preventing memory buildup.

### 4. Snapshot clone + cancel for invalidation

**File:** `internal/cache/snapshot.go:1512-1591`

When files change:
1. `Session.DidModifyFiles` → `invalidateViewLocked` (view.go:783-811)
2. Calls `prevSnapshot.cancel()` to cancel all in-flight work on stale data
3. Clones new snapshot with updated file handles
4. New requests acquire the new snapshot; old requests finish on the old
   snapshot (or get cancelled)

### 5. Budget as soft deadline, not context timeout

**File:** `internal/golang/completion/completion.go:649-664`

Completion budget is stored as a `*time.Time` pointer, deliberately NOT set on
the context. User cancellation (context done) = fail the operation. Budget
exceeded = stop searching and return partial results. These are separate
concerns. Only after collecting initial candidates does completion apply
`context.WithTimeout` for expensive callbacks (completion.go:686-689).

## Request-time completion reads from immutable compiled snapshots: the shared-API lease boundary

### The lease boundary: `Session.FileOf` → `Snapshot` + `release()`

**File:** `internal/cache/session.go:458`

Every semantic handler (including completion) acquires a snapshot lease via:
```go
fh, snapshot, release, err := s.session.FileOf(ctx, params.TextDocument.URI)
defer release()
```

This is the **shared-API lease boundary**:
- `file.Handle` — immutable content handle (hash, version, bytes)
- `*Snapshot` — immutable view of the world (packages, files, metadata)
- `func()` — release function (decrements snapshot refcount)

**Key insight:** The snapshot is the *only* shared API surface. Everything completion needs flows through it:
- `snapshot.ReadFile(ctx, uri)` → file content (`internal/cache/snapshot.go:1086`)
- `snapshot.MetadataForFile(ctx, uri, true)` → package metadata (`internal/cache/snapshot.go:673`)
- `snapshot.TypeCheck(ctx, id)` → compiled package (`internal/cache/check.go:112`)
- `snapshot.Options()` → settings (`internal/cache/snapshot.go:388`)
- `snapshot.RunProcessEnvFunc(ctx, callback)` → goimports scoring (`internal/cache/snapshot.go:1170`)

### What completion reads from the snapshot (and what it doesn't)

**Reads from snapshot:**
1. **File handle** via `snapshot.ReadFile` (indirectly through `Session.FileOf`)
2. **Package metadata** via `snapshot.MetadataForFile` (in `selectPackageForFile`, `golang/snapshot.go:95`)
3. **Type-checked package** via `snapshot.TypeCheck` (in `selectPackageForFile`, `golang/snapshot.go:99`)
4. **Options** via `snapshot.Options()` (completion.go:640)
5. **goimports scoring** via `snapshot.RunProcessEnvFunc` (completion.go:686-689)

**Does NOT read from snapshot:**
- No precomputed completion index
- No `xrefs`, `methodsets`, or `tests` indexes (those are lazy on `syntaxPackage` but completion never calls them)
- No `snapshot.ParseGo` (completion always goes through full type-check)

### The snapshot as immutable lease: what guarantees it provides

1. **Consistent file content**: `ReadFile` returns the same content for the same URI throughout the snapshot's lifetime (`snapshot.go:1086-1100`). The `file.Handle` is immutable.

2. **Consistent package results**: `TypeCheck` returns cached results from `s.packages.Get(id)` (`check.go:138-155`). Once a package is type-checked on this snapshot, it stays type-checked.

3. **No mutation during request**: The snapshot is never modified while a request holds a reference. New snapshots are created via `clone()` (`snapshot.go:1512`), and the old snapshot is cancelled but not mutated.

4. **Cancellation on invalidation**: `prevSnapshot.cancel()` (`view.go:797`) cancels all in-flight contexts on the old snapshot. Handlers detect this via `ctx.Done()` and return `RequestCancelledError`.

### Portable shared-API / lease boundary lessons

1. **The snapshot is the lease, not the package**: Completion doesn't hold a lease on the `*types.Package` directly. It holds a lease on the `*Snapshot`, and reads the package through it. This means the package can be garbage-collected when the snapshot is released, and the snapshot's `persistent.Map` handles the lifecycle.

2. **File handles are the currency of content**: `file.Handle` is the immutable content token. It carries hash, version, and bytes. The snapshot's `fileMap` (`filemap.go`) maps URI → Handle. On clone, only changed URIs get new handles; unchanged URIs share handles with the old snapshot.

3. **Type-checking is on-demand, not precomputed**: `TypeCheck` is called per-request. The `futureCache` (`future.go`) ensures concurrent requests for the same package share the work. The `typeCheckBatch` (`check.go:217-240`) allows multiple goroutines to piggy-back on the same type-checking pass.

4. **No eager indexes for completion**: gopls explicitly chose not to precompute completion-specific indexes. The `syntaxPackage` has lazy indexes (`xrefs`, `methodsets`, `tests`) but completion doesn't use them — it reads directly from `types.Package` and `types.Info`.

5. **Budget ≠ cancellation**: Completion budget is a `*time.Time` deadline, NOT set on the context (`completion.go:649-664`). User cancellation (context done) = fail the operation. Budget exceeded = stop searching and return partial results. These are separate concerns.

6. **The completer struct is per-request state**: `completer` (`completion.go:203-300`) is created fresh for each request. It holds `methodSetCache` (per-request), `tooNewSymbolsCache` (per-request), and `seen` map (per-request). No cross-request state.

7. **`persistent.Map` enables O(1) clone**: The snapshot's maps (`packages`, `files`, etc.) are `persistent.Map` (`internal/util/persistent/map.go`). Clone is O(1) — the new snapshot shares structure with the old one until modified. This is the foundation of gopls's copy-on-write model.

## Go-idiomatic patterns worth mirroring

1. **On-demand type-checking**: Don't precompute. Type-check when a request
   needs it. Cache results in a persistent map that can be cloned cheaply.

2. **`futureCache` for work sharing**: Multiple goroutines requesting the same
   package share the computation. Cancellable and retryable. This is a clean
   pattern for any concurrent cache.

3. **Snapshot ref-counting**: Simple `Acquire()`/`decref()` with deferred
   release. Prevents use-after-free without GC pressure.

4. **Persistent data structures for copy-on-write**: Clone in O(1), share
   structure until modified. Well-suited for LSP's snapshot model.

5. **Budget ≠ context deadline**: User cancellation and latency budget are
   separate concerns. Budget = stop searching, return partial results. Context
   cancellation = fail the operation.

## gopls-specific plumbing that doesn't generalize

1. **`typeCheckBatch` with `futureCache`**: This is heavily optimized for Go's
   package graph (imports, re-exports, test variants). A simpler language
   server might not need this complexity.

2. **Export data caching** (`gcimporter.IExportShallow`/`IImportShallow`):
   Go-specific serialization format. Not applicable to other languages.

3. **`resolveInvalid` for incomplete code**: Go's type checker produces
   `types.Invalid` for many incomplete constructs. gopls has special fallback
   logic to construct fake types from AST. A language with a different error
   recovery model might not need this.

4. **`goimports` integration**: The `RunProcessEnvFunc` callback and
   `imports.ScoreImportPaths` are Go-specific. Other languages would need their
   own import scoring.

5. **`persistent.Map` with reference counting**: The treap-based persistent map
   with ref-counted values is a Go-specific optimization. Simpler approaches
   (copy-on-write with full copy) may suffice for smaller codebases.

## Contradictions and gaps

1. **`methodSetCache` is per-request but not per-snapshot**: The
   `methodSetCache` map on the `completer` struct is created fresh for each
   completion request. If the same type is queried by multiple concurrent
   requests, each does its own `types.NewMethodSet` computation. This is
   acceptable because `types.NewMethodSet` is fast for typical types, but it
   means there's no cross-request sharing of method set computation.

2. **No precomputed completion index**: gopls explicitly chose not to
   precompute a completion-specific index. The comment in
   `completion.go:1310-1320` explains that the deep completion algorithm is
   "exceedingly complex and deeply coupled to the now obsolete notions that all
   token.Pos values can be interpreted by a single FileSet" — and that
   completion of unimported packages "cannot use the deep completion machinery
   which is based on type information" and instead uses "only syntax
   information from a quick parse."

3. **`resolveInvalid` is a heuristic**: When the type checker produces an
   invalid type (common during editing), `resolveInvalid` (util.go:103)
   constructs a fake `*types.Named` with `types.Invalid` underlying type. This
   is a best-effort fallback — the fake type has no methods, so method
   completions won't work for incompletely typed expressions.
