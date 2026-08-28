# LS iteration — context input paths

Shared config for the LS skill family (`ls-iteration`, `ls-backlog`, `ls-record`, `manage-ls-fixtures`).

Machine-local absolute paths for the research inputs and the record-stage targets.
(Solo setup — paths are hardcoded for this machine. If a path is missing, say so and degrade
gracefully rather than guessing.)

## Research inputs

| Input | Path | Use for |
|---|---|---|
| gopls | `/Users/wso2/projects/analysis/tools/gopls` | Wire-level/protocol patterns: JSON-RPC framing, dispatch, lifecycle. Protocol layer is under `internal/`. |
| Rewrite Ballerina LS | `/Users/wso2/projects/ballerina/ballerina-vscode/ls-rewrite/packages/ballerina-language-server` | Current language-server implementation, feature behavior, extension services, and implementation-level reference points. |
| LS architecture docs | `/Users/wso2/projects/ballerina/bls-docs/feat3` | Current architecture decisions, plans, task specifications, and design constraints for the language-server rewrite. |
| Language-server PoC implementation | `/Users/wso2/projects/ballerina/ballerina-go/ballerina-lang-go/ls-ref/lsp` | Go language-server implementation and feature reference, including protocol, snapshots, diagnostics, completion, definitions, references, symbols, and code actions. |
| Compiler (this repo) | this worktree — `ast/`, `model/`, `semantics/`, `projects/`, `context/` | What's actually available to build features from. |

## Compilation reference

The Go language-server reference is a standalone Go module rooted at
`/Users/wso2/projects/ballerina/ballerina-go/ballerina-lang-go/ls-ref`.
It requires Go 1.26 or later. Compile and run its tests with:

```bash
cd /Users/wso2/projects/ballerina/ballerina-go/ballerina-lang-go/ls-ref
go build ./lsp
go test ./lsp
```

To compile and test every package in the reference module, run:

```bash
go test ./...
```

The language-server package is `lsp`; its protocol types are under `lsp/protocol`. The
reference is a library package rather than a standalone executable, so `go build ./lsp`
is the compilation check.

## Backlog (wayfinder map — durable working state, docs vault)

| Target | Path |
|---|---|
| Map | `/Users/wso2/projects/ballerina/ballerina-go/docs/raw/ls-backlog/map.md` |
| Tickets | `/Users/wso2/projects/ballerina/ballerina-go/docs/raw/ls-backlog/issues/NN-<slug>.md` |
| Research (per ticket) | `/Users/wso2/projects/ballerina/ballerina-go/docs/raw/ls-backlog/research/NN-<slug>.md` |
| Draft design (per ticket) | `/Users/wso2/projects/ballerina/ballerina-go/docs/raw/ls-backlog/design/NN-<slug>.md` |

Lives in the docs vault, a separate repo from this worktree — nothing here is committed to `ls`. Curating any of it into the indexed wiki is the docs vault's own concern, not this skill family's.

## Record-stage targets

| Target | Path |
|---|---|
| Decision docs | `/Users/wso2/projects/ballerina/ballerina-go/docs/raw/decisions/` |
| Roadmap crosswalk (wiki, direct edit) | `/Users/wso2/projects/ballerina/ballerina-go/docs/wiki/concepts/language-server-roadmap.md` |
| API coverage crosswalk (wiki, direct edit) | `/Users/wso2/projects/ballerina/ballerina-go/docs/wiki/concepts/language-server-api-coverage.md` |
| Architecture docs (reference-only) | `/Users/wso2/projects/ballerina/bls-docs/feat3` |

## Related references

- Full-Go LS proposal: `/Users/wso2/projects/ballerina/ballerina-go/docs/wiki/proposals/full-go-language-server.md`
- Harness engineering spec (vault): `/Users/wso2/my-projects/projects/Implementing Go Language Server/Go LS Harness Engineering.md`
- Language-server reference implementation: `/Users/wso2/projects/ballerina/ballerina-go/ballerina-lang-go/ls-ref/lsp`
