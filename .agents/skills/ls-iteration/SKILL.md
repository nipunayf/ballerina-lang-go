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
| `grill-me` (global) | At the **HIL gate** for full-tier targets (shared API / decision-only). |
| `ls-record` | Twice: at **persist design** the moment the gate passes, and at **record** once verify is green. |
| `manage-ls-fixtures` | At **verify** — running the gates and gap-filling any golden-JSON fixture the worker missed. (The `worker` subagent loads it itself during **implement**.) |

## The stages

1. **Pick ticket** — invoke `ls-backlog`. It reads the map + roadmap/api-coverage crosswalks, proposes or creates the ticket, claims it. If the frontier is empty, the session becomes a charting session (that skill's rules) and stops there — charting and resolving never share a session.

2. **Classify** — ticket type picks the path: `grilling` → decision-only; `implement` → local handler or shared API (judge by the trigger below); `research`/`prototype`/`task` resolve per their definition in `ls-backlog` and skip stages 3–8 unless they grow a design question — then reclassify.

   | Path | Trigger | HIL gate | Implement/verify |
   |---|---|---|---|
   | **Local handler** | Local to one handler, no new shared surface (e.g. wiring `didSave`, an AST walk for `documentSymbol`) | Plan-mode confirm | Yes |
   | **Shared API** | Introduces a public API/facade other iterations will build on (new package boundary, shared type, anything another feature will import or extend) | Full interview | Yes |
   | **Decision-only** | Cross-cutting design decision with no code target (e.g. SemanticModel facade shape, external package resolution) | Full interview | Skipped — output is the wiki note + ADR draft |

   **Escalation rule:** if a local-tier target turns out to need a new shared abstraction partway through, stop and escalate to the full interview before implementing it. Never retroactively justify a facade that was never reviewed as one.

3. **Research** — fan out the explore subagents; **never read a research source tree inline**. The agents carry per-source instructions and persistent learnings indexes that inline reading silently skips. Definitions live in `.pi/agents/`; if a selected agent type isn't available in the current harness, stop and say so — do not fall back to exploring that source inline.

   Select agents by the ticket's target shape ("always" is the floor, not a cap — when in doubt, include; a "nothing relevant" return is cheap):

   | Agent (source) | Local handler | Shared API | Decision-only |
   |---|---|---|---|
   | `explore-compiler` (this repo's compiler surface) | always | always | when compiler feasibility bears on the decision |
   | `explore-ls-ref` (Go PoC) | always | always | if it took a stance on the question |
   | `explore-rewrite-ls` (current TS LS — behavioral spec) | always | if the API serves a user-visible feature | rarely |
   | `explore-ls-docs` (architecture docs — prior decisions) | if precedent plausibly exists | always | always |
   | `explore-gopls` (wire/dispatch patterns) | only for protocol-level work | always | always |
   | `explore-rust-analyzer` (wire/dispatch patterns) | only for protocol-level work | if gopls's answer looks idiosyncratic | always |

   Launch all selected agents in one message so they run in parallel, each prompted with the ticket's question rephrased for its source's domain (an LSP method for gopls/rust-analyzer, a feature behavior for the rewrite, an API-availability question for the compiler) plus the target shape. After fan-out the orchestrator only synthesizes — spot-checking a load-bearing `path:line` an agent returned is verification and fine; browsing a source for new findings is not. An agent reporting its source path missing (solo-setup caveat in `sources.md`) is a gap to record, not a reason to explore inline.

   Synthesize into `docs/raw/ls-backlog/research/NN-<slug>.md` (paths in `sources.md`), linked from the ticket: per-source findings (every claim keeps its `path:line` pointer — no pointer, no claim), contradictions between sources (surfaced prominently; they become HIL-gate branches, not things research resolves), gaps, and at most a few sentences of design implications — the draft-design stage owns the proposal.

4. **Draft design** — a short, concrete proposal: what API/handler shape, what it depends on, what it doesn't cover yet. Not an ADR. If this is the first iteration that needs test verification, the design MUST include building the LSP test driver (spec in `ls/AGENTS.md`) as an explicit deliverable — it is an M1-scale task, not incidental plumbing.

5. **HIL gate** — tiered:
   - **Local handler:** present the approach via plan mode; a single approve/reject is enough.
   - **Shared API / decision-only:** invoke `grill-me` — walk every branch of the design tree with AskUserQuestion, recommending an answer for each question. No code (and no decision note) until every branch is resolved.

   Rejected or reworked designs loop back to stage 3/4 with the feedback folded in.

6. **Persist design at approval** — invoke `ls-record` (persist mode) the moment the gate passes, before any implementation. Implement against the persisted file, not conversation memory — a compaction or crash must not lose the approved design.

7. **Implement** — delegate to the `worker` subagent (defined in `.pi/agents/`); do not implement inline. Prompt it with the ticket and the path to the persisted design file — the worker reads the design file itself and works red-green with fixture tests. If the worker isn't available in the current harness, stop and say so. The work is standard Go under the top-level `ls/` tree (`ls/protocol`, `ls/server`, `ls/corpus` — roles in `ls/AGENTS.md`), following root `AGENTS.md` conventions (unexported-by-default, `*Base` embedding, `SymbolRef` keys, PAL for all platform interaction including the stdio transport). If the worker reports back that the design needs an unreviewed shared abstraction, that's the stage-2 escalation rule firing — return to the full interview, don't wave it through. Decision-only targets skip stages 7–8.

8. **Verify** — the worker writes the scenario fixtures as it goes (red-green); this stage is independent confirmation of its report, not first-time fixture writing. Re-run the gates yourself — green means the LS suite, `go build ./...`, and `go vet ./ls/...` all pass — and check the worker's scenarios actually cover the design; invoke `manage-ls-fixtures` for any fixture the design promised but the worker missed.

9. **Record + resolve** — invoke `ls-record` (record mode: decision doc finalization, crosswalk, ADR draft), then `ls-backlog` (resolve the ticket, graduate fog, tend out-of-scope).
