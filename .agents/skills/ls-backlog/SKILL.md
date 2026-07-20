---
name: ls-backlog
description: Operate the LS wayfinder backlog in docs/raw/ls-backlog/ — pick/claim/create/resolve tickets, chart new ones when the frontier is empty, tend fog and out-of-scope. Use at the bookends of an ls-iteration, or standalone when the user wants to chart, groom, or inspect the LS backlog.
---

The LS backlog is a wayfinder-style markdown tracker living under `docs/raw/ls-backlog/` in the docs vault — a separate repo from this worktree, so nothing here is committed to `ls`. Curating any of it into the indexed wiki (see "Resolve" and "Milestone fold") is the docs vault's own concern, not this skill's — this skill only reads/writes the raw working state.

## Layout

- **Map**: `docs/raw/ls-backlog/map.md` — Destination / Notes / Decisions so far / Not yet specified (fog) / Out of scope. An **index**, not a store: decisions are one-line gists linking the ticket or decision doc that holds the detail.
- **Tickets**: `docs/raw/ls-backlog/issues/NN-<slug>.md`, numbered from `01`. Header lines: `Type:`, `Status:` (`open`/`claimed`/`resolved`), `Blocked by: NN, NN` (or `none`). Body is the `## Question`; the `## Answer` is appended at resolution.
- **Research**: `docs/raw/ls-backlog/research/NN-<slug>.md` — the research-stage subagent fan-out's output for ticket `NN` (mechanics in `ls-iteration`), linked from that ticket.
- **Design**: `docs/raw/ls-backlog/design/NN-<slug>.md` — the draft-design stage's output for ticket `NN` (non-durable HIL-gate input, superseded once the decision doc is persisted), linked from that ticket.
- **Frontier**: open + unblocked + unclaimed tickets, lowest number first. A ticket is unblocked when every ticket it lists is `resolved`.

## Rules

- **Claim before work**: set `Status: claimed` before doing anything else on a ticket.
- **One ticket per session** — resolve it, then stop; the next ticket is a fresh invocation.
- **Refer by name**: in everything the user reads, call tickets by title, never bare numbers. The number rides inside the reference (`Build the JSON-RPC transport (03)`), never stands in for it.
- **Create, then wire**: new tickets get their number first, blocking edges in a second pass.

## Ticket types (`Type:` line)

The wayfinder set plus one execution type — this map carries implementation, an explicit Notes override of wayfinder's plan-don't-do default:

| Type | Human in loop? | What it is |
|---|---|---|
| `grilling` | HITL | A decision to resolve by interview — ls-iteration's decision-only path |
| `research` | AFK | Reading external sources; produces a markdown summary linked from the ticket |
| `prototype` | HITL | A cheap concrete artifact to react to when "how should it behave/look" is the question |
| `task` | either | Manual work that unblocks a decision (provisioning, data moves) — does, not decides |
| `implement` | HITL | Code target run through the full ls-iteration loop |

## Pick a ticket

Read the map (low-res view; zoom into ticket bodies on demand) **and** the roadmap/api-coverage crosswalks (paths in `../ls-iteration/sources.md`) for milestone framing and dependency status. Then:

- User named a target that has a ticket → use it.
- User named a target with no ticket → create it (create-then-wire); surface any dependency gap the crosswalk reveals before starting.
- User named nothing → propose the first frontier ticket; confirm before claiming.
- No map, or empty frontier → **chart** (below).

Claim the chosen ticket before any research.

## Chart — on demand only

Charting is one session's work; never also resolve a ticket. Grill **breadth-first** over the next uncharted milestone chunk — fan out across the space, don't go deep on any thread. Create the tickets that are sharp enough now, wire blocking edges in a second pass, sketch the rest into fog.

**Fog or ticket?** The test is whether the *question* can be stated precisely now — not whether it can be answered. Sharp question → ticket (even if blocked). Dimmer → **Not yet specified**. Don't pre-slice fog into ticket-sized pieces.

## Resolve a ticket

- Append the `## Answer`: one-line gist + pointers to the decision doc and/or commit — detail lives there, not in the ticket.
- Set `Status: resolved`.
- Append a one-line context pointer to the map's Decisions so far.

## Tend the map (after every resolution)

- **Graduate fog** the resolution made specifiable into new tickets (create-then-wire), clearing the graduated patch from Not yet specified.
- **Out of scope**: work revealed to sit beyond the destination gets a one-line gist + why in that section; if it was already a ticket, close it and link it. Out-of-scope never graduates.
- Update or delete tickets the decision invalidated.
- **Milestone fold**: when a milestone's tickets are all resolved, fold that stretch of Decisions so far into the roadmap crosswalk (`language-server-roadmap.md`, in the wiki) — the map must always be deletable without losing history.
