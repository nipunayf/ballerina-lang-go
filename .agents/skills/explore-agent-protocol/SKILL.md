---
name: explore-agent-protocol
description: Vault-read/write and output protocol for a read-only research-fanout subagent — how to traverse and update the docs/ Zettelkasten vault, and how to shape its final report.
---

Use this skill for the mechanical parts of running a read-only research-fanout
subagent: how to traverse and update the `docs/` Zettelkasten vault (its
persistent memory), and how to shape your final report. Your agent prompt
tells you the source name (e.g. `compiler`, `gopls`) and whatever
source-specific rules apply on top of this — this skill covers only the part
that's identical across every such agent.

The full save-gate rules and note schema live in `docs/AGENTS.md`'s
"Zettelkasten vault constraints" section — read it once if you haven't
already; this skill assumes it.

## Before every task: load the obsidian skill, then precondition-check

1. Read `docs/.agents/skills/obsidian/SKILL.md` in full and follow it as your
   default mechanism for reading and writing the vault — the `obsidian` CLI,
   not plain file Read/Write. Fall back to plain Read/Edit only for the two
   documented CLI gaps: in-place body-content edits, and array-valued
   frontmatter (`sources: []`). Never use `obsidian eval` for these gaps.
2. Run `obsidian vault` as a precondition check. If it fails, or reports a
   vault other than `docs`, **abort and report clearly** ("Obsidian not
   running, or `docs` vault not focused — vault access unavailable") rather
   than silently falling back to plain file ops for the whole run.

## Vault read/write protocol

1. Read `moc/<source>.md` first (`obsidian read path="moc/<source>.md"`);
   follow its `[[wikilinks]]` into the relevant `notes/<source>/*.md` files.
2. **Before creating** anything new, search `notes/<source>/` for an
   overlapping claim (`obsidian search:context query="..." path="notes/<source>"`).
   Link to or sharpen an existing note rather than create a variant.
3. Apply the six-point save gate from `docs/AGENTS.md`. Pass → write/update
   the note under `notes/<source>/` (`obsidian create ...` for a new note;
   `property:set` for scalar frontmatter; plain Read+Edit for the body or a
   `sources:` array). Ambiguous → write to `notes/inbox/` instead, and flag
   the unresolved call in your final report.
4. Update `moc/<source>.md` in the same run (`obsidian append ...`) to
   reflect the new/changed note — same responsibility as the old "update
   INDEX.md" step, just retargeted.
5. Route dead-end/gap findings to `logs/<source>-dead-ends.md` /
   `logs/<source>-gaps.md` (`obsidian append`/`prepend`) — flat, pruned, not
   atomized, same spirit as before.

## Output contract

A findings summary with concrete `path:line` pointers for every claim.
Include any extra source-specific callout your agent prompt asks for, plus:
notes created, notes modified, unresolved save-gate calls (routed to
`notes/inbox/`), and any dead-ends/gaps logged. Keep it terse; you're one
input to a larger research fan-out, not the final report.
