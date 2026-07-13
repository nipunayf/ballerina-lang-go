---
name: ls-record
description: Persist and record LS decisions — wiki decision notes, roadmap/api-coverage crosswalk updates, draft-only ADR promotion, and commit rules. Use at two points of an ls-iteration — persist mode when the HIL gate passes, record mode when verify is green — or standalone to write up an LS decision.
---

Record-stage paths (wiki, crosswalks, decision notes, ADR home) are in `../ls-iteration/sources.md`.

## Persist mode — the moment the HIL gate passes, before implementing

- **Full tier (shared API / decision-only):** write the approved design as a wiki note under `docs/wiki/decisions/`, matching the frontmatter/style of existing wiki pages. Fold the research summary in — what gopls/Java LS/ADRs said, with file pointers — so future iterations don't re-run that fan-out. Mark status `implementing` (or `decided` for decision-only).
- **Local tier:** a brief design note in the session scratchpad is enough; the commit message carries the durable record.

## Record mode — once verify is green

- **Commit on green:** commit to the `ls` branch with a message naming the target and the design decision. **Never push** — pushing/PR is the user's call.
- **Full tier:** finalize the wiki note — status, verify outcomes, anything discovered that the design didn't anticipate.
- **Crosswalk:** update `language-server-roadmap.md` / `language-server-api-coverage.md` in place — mark what's now built, what's still a gap, fold in discoveries. Update, don't duplicate.
- **ADR promotion (major decisions only):** draft the numbered ADR text matching the `architecture-resources/ADR-0xx-*.md` convention and hand it to the user in the final message — **do NOT write into the Java LS repo**; placing team-visible ADRs is the user's act.
- Routine local-tier iterations need only the commit and the crosswalk update.

## Boundaries (both modes)

- Wiki: direct writes are fine — notes and crosswalk edits go straight in.
- `architecture-resources/` (Java LS repo): read-only, always. Draft ADRs are delivered as text, never placed.
- Git: commit yes, push never.
