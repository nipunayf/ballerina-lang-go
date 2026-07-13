---
name: ls-research
description: Research an LS target against the four reference sources (gopls, Java Ballerina LS, Rewrite-LS ADRs, this repo's compiler APIs), weighted by target shape, fanning out to parallel subagents. Use during the research stage of an ls-iteration, or standalone when the user wants an LS question investigated against these sources.
---

Source paths are in `../ls-iteration/sources.md` — read it first. If a path is missing, say so and degrade gracefully rather than guessing.

## Weight sources by target shape

Don't uniformly consult all four:

| Target shape | Lean on | Why |
|---|---|---|
| Protocol/lifecycle (`initialize`, text sync, `$/setTrace`, transport) | gopls | Wire-level and dispatch patterns; Go has no off-the-shelf LSP 3.18 library. gopls's protocol layer is internal but it is the only Go implementation with production mileage. |
| Compiler-backed feature (hover, completion, definition, references, code actions) | Java LS + this repo's APIs (`ast`, `model`, `semantics`, `projects`, `context`) | Java LS defines *what* the feature must do; the Go compiler defines *what's actually available* to build it from. |
| Cross-cutting/architectural (SemanticModel facade, package resolution, workspace model) | ADRs + this repo's APIs | The Rewrite LS ADRs already fought through these decisions once; check for a conflicting or reusable precedent before re-deciding. |

## Fan out

Run parallel Explore subagents — one per relevant source, prompts built from the paths in `sources.md`. This stage has no human in it; parallelism is free.

Each subagent must return findings with concrete file pointers (`path:line`), not prose summaries alone.

## Before researching, check the trail

Past full-tier designs live in `docs/wiki/decisions/` (path in `sources.md`) with their research summaries folded in — read the relevant ones first so the fan-out isn't repeated for questions already answered.

## Output

A findings summary organized by source, with pointers, feeding the design draft. For a `research`-type backlog ticket, write the summary as a markdown file and link it from the ticket instead.
