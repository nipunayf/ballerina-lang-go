# gopls protocol dispatch, lifecycle, and response shapes

## Protocol dispatch
- `internal/protocol/tsserver.go:serverDispatch` (line 196) — generated switch on method name, unmarshals params, calls server method.
- `internal/protocol/tsserver.go:ServerDispatchCall` (line 210) — same but returns `(resp, handled, err)` for use by MCP or other callers.
- `internal/protocol/protocol.go:ServerHandler` — wraps dispatch with context cancellation check and fallthrough to a generic handler.
- `internal/protocol/protocol.go:ClientHandler` — same for client->server notifications.

## Lifecycle
- `serverCreated` → `serverInitializing` (on `Initialize`) → `serverInitialized` (on `Initialized`) → `serverShutDown` (on `Shutdown`).
- `$/setTrace` is NOT implemented — returns `notImplemented("SetTrace")` (unimplemented.go:122).
- `$/progress` is handled via `progress.Tracker` (server.go:progress field).
- `Shutdown` calls `s.session.Shutdown(ctx)` which waits for all snapshot refs to release.
- `Exit` calls `s.client.Close()` and exits(1) if not already shut down.

## Response result shapes
- **Hover**: `*protocol.Hover` with `Contents` (MarkupContent) and `Range`. Internal `hoverResult` struct (hover.go:38) has Synopsis, FullDocumentation, Signature, SingleLine, SymbolName, LinkPath, LinkAnchor, plus unexported fields (typeDecl, methods, promotedFields, footer).
- **Completion**: `*protocol.CompletionList` with `IsIncomplete` and `Items []protocol.CompletionItem`. Items are converted from internal `completion.CompletionItem` via `toProtocolCompletionItems` (completion.go:80).
- **Definition**: `[]protocol.Location` — a slice of locations (supports multiple definitions).
- **References**: `[]protocol.Location` — sorted with declarations before uses, deduplicated. Internal `reference` struct has `isDeclaration`, `location`, `pkgPath`.
- **Signature Help**: `*protocol.SignatureHelp` with `Signatures []protocol.SignatureInformation`, `ActiveSignature`, `ActiveParameter`. Returns nil on failure (silently swallowed, not an error).

## File kind dispatch
- All semantic handlers dispatch on `snapshot.FileKind(fh)` — returns `file.Go`, `file.Mod`, `file.Tmpl`, `file.Work`, `file.Asm`.
- Each file kind has its own implementation (golang/, mod/, template/, work/, goasm/ packages).
- Unsupported file kinds return nil/empty results, not errors.
