# Java LS async compilation model & legacy behaviors

Keep entries summarized and pointer-dense — `path` + symbol, one line each.
Java LS root: `ballerina-lang/language-server/`.

## Async compilation / debounce / locking

- `ProjectUpdateEventPublisher.publish()` (`BallerinaTextDocumentService.java:586-593`): 1s debounce via `CompletableFuture.delayedExecutor`, cancels previous `latestScheduled` via `completeExceptionally`
- `DiagnosticsHelper.compileAndSendDiagnostics()` (`DiagnosticsHelper.java:130-140`): second debounce layer with `latestScheduled`, 1s delay, then `thenApplyAsync` → `waitAndGetPackageCompilation` → `thenAccept` → `compileAndSendDiagnostics`
- `BallerinaWorkspaceManager.waitAndGetPackageCompilation()` (`BallerinaWorkspaceManager.java:230-260`): per-project `ReentrantLock`, `project.currentPackage().getCompilation()`, checks `BAD_SAD_FROM_COMPILER`/`CYCLIC_MODULE_IMPORTS_DETECTED` to set `compilationCrashed`
- `BallerinaWorkspaceManager.createOrGetProjectPair()`: fresh `Project` via `BuildProject.load()`/`SingleFileProject.load()`, wrapped in `ProjectContext` with lock
- `BallerinaWorkspaceManagerProxyImpl`: dual workspace managers — `baseWorkspaceManager` for `file:` scheme, `ClonedWorkspace` for `expr:` scheme (inlay hints, code actions on virtual documents)
- `ClonedWorkspace` (`BallerinaWorkspaceManagerProxyImpl.java:80-100`): duplicates project via `project.duplicate()`, separate `sourceRootToProject` map, removed on `didClose`

## Event sync

- `EventKind.PROJECT_UPDATE` — published on didOpen, didChange, loadProject. Enum: `langserver-commons/.../eventsync/EventKind.java`
- Subscribers: `PublishDiagnosticSubscriber.java` and `ResolveModulesSubscriber.java` — both subscribe to `PROJECT_UPDATE`

## Legacy/workaround behavior (don't cargo-cult)

- **Double debounce**: `ProjectUpdateEventPublisher` (1s) then `DiagnosticsHelper` (1s) — two layers of the same mechanism. The Go rewrite should have a single, well-defined debounce/scheduling layer.
- **`completeExceptionally` cancellation**: Java idiom for cancelling the previous future; Go should use `context.WithCancel` per key.
- **Per-project `ReentrantLock`**: coarse-grained lock around the whole project; the Go rewrite's ADR-042 modifier chain is a better model.
- **`ClonedWorkspace` for `expr:` scheme**: duplicates the entire project for virtual documents; Go should consider a lighter approach (overlay FS / virtual documents).
- **`compilationCrashed` flag**: set on BAD_SAD/CYCLIC_MODULE, blocks further `semanticModel` calls; Go should handle these as regular diagnostics.
- **Fresh project load per change**: `createOrGetProjectPair` re-loads each time — full re-Load cost; the Go modifier chain avoids this.
- **`DiagnosticEnv` singleton per Load**: documented in Go `workspace.go` comments as "panics on re-registration of a changed file" — the reason early Go phases re-Loaded per publication.
