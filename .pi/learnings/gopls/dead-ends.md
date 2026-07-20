# gopls dead ends

- No `internal/lsp/` directory — layout moved to `internal/server/`, `internal/cache/`, `internal/protocol/`.
- No explicit "no-compile" request classification — all semantic queries type-check.
- `$/setTrace` is unimplemented (returns notImplemented error).
