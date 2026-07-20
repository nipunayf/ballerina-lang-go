# gopls request cancellation architecture

## Two-layer cancellation
1. **jsonrpc2 wire-level `CancelHandler`** maps request IDs to `context.CancelFunc` (jsonrpc2/handler.go:33-60)
2. **Snapshot-level cancellation** via `Snapshot.cancel` (snapshot.go:64)

## Handler chain
- `Handlers()` = `CancelHandler(AsyncHandler(MustReplyHandler(handler)))` (protocol.go:247).
- `CancelHandler` wraps every non-`$/cancelRequest` handler with `replyWithDetachedContext` that converts cancelled ctx to `RequestCancelledError` (protocol.go:226-232).
- **`$/cancelRequest` dispatch**: `CancelHandler` intercepts the notification, unmarshals `CancelParams`, calls `canceller(id)` which looks up the `context.CancelFunc` in the jsonrpc2 map and cancels the request's context (protocol.go:234-244).

## jsonrpc2.CancelHandler
- `jsonrpc2/handler.go:33-60`: maintains `map[ID]context.CancelFunc` guarded by a mutex. For each `*Call`, wraps ctx with `context.WithCancel`, stores the cancel func, and wraps the reply to delete the entry on reply. The returned `canceller(id)` function cancels the stored context.

## v2 jsonrpc2
- `jsonrpc2_v2/conn.go:453-460`: `Connection.Cancel(id)` looks up `incomingByID` map and calls `req.cancel()`. The `Preempter` interface (jsonrpc2_v2/jsonrpc2.go:37-48) allows `$/cancelRequest` to be handled out-of-order before the main handler queue.

## xcontext.Detach
- `xcontext/xcontext.go:22-28`: creates a context that inherits values but detaches from cancellation/deadline — used to send `$/cancelRequest` notifications and to reply after cancellation.

## Client-side cancellation
- `clientConnV2.Call` (protocol.go:72-78) sends `$/cancelRequest` if `ctx.Err() != nil` after `call.Await`. v1 `clientConn.Call` (protocol.go:56-59) uses `cancelCall` helper (protocol.go:100-106) which sends `$/cancelRequest` with a detached context.

## Transferable patterns
- **Handler chain composition**: `CancelHandler(AsyncHandler(MustReplyHandler(handler)))` is clean and composable. Each layer has one responsibility.
- **`replyWithDetachedContext`**: wrapping the reply function to convert cancelled ctx to `RequestCancelledError` is elegant — handlers don't need to check cancellation themselves.
- **`xcontext.Detach`**: essential utility for sending notifications after cancellation (e.g., `$/cancelRequest` to server, or replying after ctx is done).
