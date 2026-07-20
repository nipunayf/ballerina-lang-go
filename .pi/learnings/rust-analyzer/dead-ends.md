# Dead-ends

### No staged/partial completion
rust-analyzer has no two-phase completion (syntax-only first, then upgrade with semantics). It always waits for full semantic analysis or gets cancelled+retried. The `vfs_done` check (`dispatch.rs:108-115`) returns a default result if VFS isn't ready, but this is a hard fallback, not a staged approach.

### No fine-grained cancellation
Cancellation is all-or-nothing: when a change arrives, all in-flight Salsa queries are cancelled. There's no mechanism to preserve position-independent semantic facts across edits.

### `ExprScopes` is a Salsa query
`ExprScopes` depends on the body, so it's invalidated when the file changes. It's not purely syntax-derived in the sense of being independent of semantic analysis — it's a cached query that gets recomputed on change.
