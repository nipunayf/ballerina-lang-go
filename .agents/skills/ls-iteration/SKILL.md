---
name: ls-iteration
description: Orchestrate one iteration of the language-server engineering loop — the stages, and which skill to invoke when. Use when the user wants LS work done — implement a handler or shared API, resolve an LS design decision, or chart the LS backlog.
---

One invocation = one ticket from the LS backlog. This skill holds the loop and the routing; the stage mechanics live in dedicated skills — invoke them at the points below.

The loop, in stage order:

pick ticket → classify → research → draft design → HIL gate → persist design → implement → verify → record + resolve

Two non-linear edges:

- **HIL gate rejects or requests rework** → return to research/draft design (stages 3–4) with the feedback folded in; re-gate before proceeding.
- **record + resolve ends the session** → the next ticket is a fresh invocation starting at pick ticket; never continue into a second ticket.

Shared config: research/record paths in `sources.md` next to this file. Conventions: `ls/AGENTS.md` (LS), root `AGENTS.md` (repo-wide).

## Skills this loop uses

| Skill | Invoke when |
|---|---|
| `ls-backlog` | At **pick ticket** (load map, chart on demand, claim) and at **record + resolve** (resolve ticket, tend the map). Also standalone when the user just wants to chart or groom the backlog. |
| `ls-research` | At **research**, once the ticket is classified — it weights sources by target shape and fans out subagents. |
| `grill-me` (global) | At the **HIL gate** for full-tier targets (shared API / decision-only). |
| `ls-record` | Twice: at **persist design** the moment the gate passes, and at **record** once verify is green. |
| `manage-ls-fixtures` | At **verify** — adding/updating golden-JSON fixtures and running the gates. |

## The stages

1. **Pick ticket** — invoke `ls-backlog`. It reads the map + roadmap/api-coverage crosswalks, proposes or creates the ticket, claims it. If the frontier is empty, the session becomes a charting session (that skill's rules) and stops there — charting and resolving never share a session.

2. **Classify** — ticket type picks the path: `grilling` → decision-only; `implement` → local handler or shared API (judge by the trigger below); `research`/`prototype`/`task` resolve per their definition in `ls-backlog` and skip stages 3–8 unless they grow a design question — then reclassify.

   | Path | Trigger | HIL gate | Implement/verify |
   |---|---|---|---|
   | **Local handler** | Local to one handler, no new shared surface (e.g. wiring `didSave`, an AST walk for `documentSymbol`) | Plan-mode confirm | Yes |
   | **Shared API** | Introduces a public API/facade other iterations will build on (new package boundary, shared type, anything another feature will import or extend) | Full interview | Yes |
   | **Decision-only** | Cross-cutting design decision with no code target (e.g. SemanticModel facade shape, external package resolution) | Full interview | Skipped — output is the wiki note + ADR draft |

   **Escalation rule:** if a local-tier target turns out to need a new shared abstraction partway through, stop and escalate to the full interview before implementing it. Never retroactively justify a facade that was never reviewed as one.

3. **Research** — invoke `ls-research` with the ticket's question and its target shape.

4. **Draft design** — a short, concrete proposal: what API/handler shape, what it depends on, what it doesn't cover yet. Not an ADR. If this is the first iteration that needs test verification, the design MUST include building the LSP test driver (spec in `ls/AGENTS.md`) as an explicit deliverable — it is an M1-scale task, not incidental plumbing.

5. **HIL gate** — tiered:
   - **Local handler:** present the approach via plan mode; a single approve/reject is enough.
   - **Shared API / decision-only:** invoke `grill-me` — walk every branch of the design tree with AskUserQuestion, recommending an answer for each question. No code (and no decision note) until every branch is resolved.

   Rejected or reworked designs loop back to stage 3/4 with the feedback folded in.

6. **Persist design at approval** — invoke `ls-record` (persist mode) the moment the gate passes, before any implementation. Implement against the persisted file, not conversation memory — a compaction or crash must not lose the approved design.

7. **Implement** — standard Go work under the top-level `ls/` tree (`ls/protocol`, `ls/server`, `ls/corpus` — roles in `ls/AGENTS.md`), following root `AGENTS.md` conventions (unexported-by-default, `*Base` embedding, `SymbolRef` keys, PAL for all platform interaction including the stdio transport). Decision-only targets skip stages 7–8.

8. **Verify** — invoke `manage-ls-fixtures` to add the scenario fixtures and run the gates. Green means the LS suite, `go build ./...`, and `go vet ./ls/...` all pass.

9. **Record + resolve** — invoke `ls-record` (record mode: wiki note finalization, crosswalk, ADR draft), then `ls-backlog` (resolve the ticket, graduate fog, tend out-of-scope).
