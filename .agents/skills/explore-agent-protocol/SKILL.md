---
name: explore-agent-protocol
description: Learnings-index and output protocol for a read-only research-fanout subagent — how to consult and update its persistent learnings memory, and how to shape its final report.
---

Use this skill for the two mechanical parts of running a read-only
research-fanout subagent: how to consult and update its persistent learnings
memory, and how to shape its final report. Your agent prompt tells you the
learnings directory path and whatever source-specific rules apply on top of
this — this skill covers only the part that's identical across every such
agent.

## Learnings index protocol

- **First**: read `INDEX.md` in your learnings directory; open only the topic
  files relevant to your query, plus any always-open files (see below).
  Verify a pointer before reusing it.
- **Last**: merge this run's findings as one-liners under the matching topic
  file's headings — never a ticket- or date-named section. Dedupe and prune
  stale entries in files you touch. Route special-purpose findings to their
  dedicated log (see below). No fitting topic → new file + routing line in
  `INDEX.md`. File over ~150 lines → split it, update `INDEX.md`.
- Only the learnings files are writable; the source you're exploring stays
  read-only.

## Special-purpose learnings files

Your agent prompt may point you at some of these — always open the ones it
names, and route the matching findings there instead of a topic file:

- A **dead-ends log** (e.g. `dead-ends.md`) for fruitless searches.
- A **gaps log** (e.g. `gaps.md`) for gaps found against what's needed.
- An **unverified-facts file** (e.g. `unsorted-facts.md`) — treat its
  contents with extra scrutiny; it may be flagged as such in its own header.

## Output contract

A findings summary with concrete `path:line` pointers for every claim. 
Include any extra source-specific callout your agent prompt asks for. Keep it
terse; you're one input to a larger research fan-out, not the final report.
