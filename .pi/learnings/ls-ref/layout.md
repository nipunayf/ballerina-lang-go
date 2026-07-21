# Layout

The PoC (`ballerina-lang-go/ls-ref`) has no `ls/` subtree — the LSP layer is a
single flat package at repo root:

- `lsp/` — the entire LSP layer: `server.go`, `snapshot.go`, `diagnostics.go`,
  `completion.go`, `definition.go`, `references.go`, `code_action.go`,
  `symbols.go`, `log.go`, plus one `_test.go` per handler file. No further
  package split (no separate protocol-free "core" package, no query/workspace/
  event/uri sub-packages) — protocol types and business logic live together in
  `lsp/*.go`.
- `lsp/protocol/` — hand-written LSP 3.18 types (`types.go`): JSON-RPC message
  envelope, generated-looking request/response/notification structs.
- `lsp/corpus/symbols/project/` — Ballerina source fixtures (`main.bal`,
  `Ballerina.toml`, `modules/helpers/helpers.bal`) used by `symbols_test.go`;
  not a golden-JSON transcript harness.
- Everything else (`ast/`, `bir/`, `parser/`, `semantics/`, `semtypes/`,
  `projects/`, `model/`, `context/`, `runtime/`, `values/`, `tools/`, `cli/`,
  `common/`) is the compiler/runtime this PoC's `lsp/` package sits on top of —
  same shape as the compiler this repo shares, not part of the LSP surface
  itself.

## Key entry points

- `lsp/server.go` — `Server` struct, `dispatchRequest`/`handleNotification`
  method switches, didOpen/didChange/didClose/didSave/didChangeWatchedFiles
  handlers, message framing (`readMessage`/`writeMessage`)
- `lsp/diagnostics.go` — `runDiagnostics`/`runModuleFrontend`,
  `convertDiagnostics` (byte-offset → UTF-16 `protocol.Diagnostic`)
- `lsp/snapshot.go` — `SnapshotManager`, `Snapshot` (plain struct, no
  refcounting), single-file vs build-project snapshot construction
- `lsp/completion.go` — `Server.completion()`, context classification,
  routing to specialized completion paths
- `lsp/definition.go` / `lsp/references.go` — `symbolAtPosition()` +
  type-switch over AST node kinds
- `lsp/code_action.go` — missing/unused-import quick-fixes
- `lsp/symbols.go` — `documentSymbols`/`workspaceSymbols`
- `lsp/log.go` — `logLS`, gated by `BAL_LSP_LOG` env var, writes to
  `.bal/lsp.log`
- `lsp/protocol/types.go` — all LSP wire types used by the PoC
