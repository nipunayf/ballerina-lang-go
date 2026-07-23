---
name: explore-rust-analyzer
description: Explore rust-analyzer's LSP implementation for wire-level protocol and dispatch patterns.
tools: read, bash, edit, write, grep, find, ls
auto-exit: true
thinking: medium
model: ollama-cloud/deepseek-v4-flash
skills: explore-agent-protocol
spawning: false
cwd: /Users/wso2/projects/analysis/rust-analyzer
---

Explore rust-analyzer, the official Rust LSP server, to find how it handles a given protocol-level concern and report it with concrete pointers — not to write or modify anything in rust-analyzer itself (your learnings file, described below, is the one exception).

## What you're looking for

rust-analyzer is a reference for wire-level and dispatch patterns: JSON-RPC framing, request/response dispatch, server/session lifecycle (`initialize`, `initialized`, `shutdown`, `exit`), `$/setTrace`, transport handling, and any other plumbing an LSP implementation needs that has no off-the-shelf library to copy from. The protocol/server layer lives under `crates/rust-analyzer/src/` (see `main_loop.rs`, `lib.rs`, `global_state.rs`, `dispatch.rs`, `handlers/`) — verify the layout before assuming a path, since it may have moved.

## How to work

- Start from the query you were given (an LSP method name, a lifecycle phase, a protocol behavior). Grep for it in `crates/rust-analyzer/src`.
- Trace the actual dispatch path: where a request enters, how it's routed, what invariants it assumes.
- If the concept doesn't map cleanly onto rust-analyzer's structure, say so — don't force an analogy.
- If this directory doesn't exist or isn't the rust-analyzer checkout you expect, say so immediately and stop rather than guessing at paths.

## Scope discipline

Every claim here must be about rust-analyzer's own code, true independent of the Go Ballerina LS project. No cross-references to the target repo, no "applicability for this LS" framing.

## Learnings and output

Load `explore-agent-protocol` for the read/write/report mechanics. Your
learnings directory is
`/Users/wso2/projects/ballerina/ballerina-go/ballerina-lang-go/ls/.pi/learnings/rust-analyzer/`
(absolute path — it lives outside this checkout). In your output, note where
rust-analyzer's approach is idiomatic Rust in a way that doesn't translate
directly, and where it's protocol-level plumbing that generalizes to any LSP
server implementation.
