# gopls layout

- `internal/server/` — LSP method handlers (one file per method group: hover.go, completion.go, definition.go, references.go, signature_help.go, semantic.go, general.go, workspace.go, text_synchronization.go, code_action.go, etc.)
- `internal/cache/` — Session, View, Snapshot, type-checking, diagnostics, file overlay
- `internal/protocol/` — LSP protocol types (tsprotocol.go), dispatch (tsserver.go, tsclient.go), JSON-RPC framing (protocol.go)
- `internal/lsprpc/` — StreamServer, binder, middleware
- `internal/golang/` — Go-specific semantic query implementations (hover, definition, references, signature_help, completion, etc.)
- `internal/golang/completion/` — Completion logic (separate subpackage)

## Key entry points

- `internal/server/server.go:New` — creates a `server` struct implementing `protocol.Server`
- `internal/lsprpc/lsprpc.go:ServeStream` — entry point for each LSP connection; creates Session + server
- `internal/protocol/protocol.go:Handlers` — wraps handler with CancelHandler → AsyncHandler → MustReplyHandler
- `internal/protocol/protocol.go:CancelHandler` — intercepts `$/cancelRequest`, delegates to jsonrpc2.CancelHandler
- `internal/protocol/tsserver.go:ServerDispatchCall` — generated switch dispatch for all LSP methods
- `internal/cache/session.go:NewSession` — creates a Session (holds views, overlayFS, parseCache)
- `internal/cache/view.go:Snapshot` — returns current Snapshot for a View (ref-counted)
- `internal/cache/snapshot.go:Snapshot` — the core state container (files, packages, metadata, cancellation)
