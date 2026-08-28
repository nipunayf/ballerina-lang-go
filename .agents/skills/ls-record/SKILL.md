---
name: ls-record
description: Persist and record LS decisions — decision docs in the docs vault's raw/ tier, roadmap/api-coverage crosswalk updates, and draft-only ADR promotion. Use at two points of an ls-iteration — persist mode when the HIL gate passes, record mode when verify is green — or standalone to write up an LS decision.
---

Record-stage paths (decision docs, crosswalks, ADR home) are in `../ls-iteration/sources.md`.

Decision docs live under `docs/raw/decisions/` in the docs vault — pre-curation source material, not a curated wiki page. This skill never writes into `docs/wiki/`, and never edits `docs/AGENTS.md` (that vault's own operating manual belongs to a separate agent). Whether/when a decision doc gets curated into an indexed wiki page is the user's call, made later, in that vault.

## Persist mode — the moment the HIL gate passes, before implementing

- **Full tier (shared API / decision-only):** write the approved design as a decision doc under `docs/raw/decisions/` (e.g. `YYYY-MM-DD-<slug>.md`). No wiki frontmatter required — a short title and status line is enough. Fold the research summary in — what gopls/Java LS/ADRs said, with file pointers — so future iterations don't re-run that fan-out. Mark status `implementing` (or `decided` for decision-only).
- **Local tier:** a brief design note in the session scratchpad is enough.

## Record mode — once verify is green

- **Full tier:** finalize the decision doc — status, verify outcomes, anything discovered that the design didn't anticipate.
- **Crosswalk:** update `language-server-roadmap.md` / `language-server-api-coverage.md` in place, in the wiki — mark what's now built, what's still a gap, fold in discoveries. Update, don't duplicate.
- **ADR promotion (major decisions only):** draft the numbered ADR text matching the `architecture-resources/ADR-0xx-*.md` convention and hand it to the user in the final message — **do NOT write into the Java LS repo**; placing team-visible ADRs is the user's act.
- Routine local-tier iterations need only the crosswalk update.

## Boundaries (both modes)

- `docs/raw/decisions/`: direct writes and in-place updates are fine — this is this skill's own working area.
- Wiki crosswalks: direct edits are fine — they're already-established, continuously-updated pages, not new decision records.
- `docs/wiki/` otherwise, and `docs/AGENTS.md`: not this skill's to write — that's the docs vault's own curation, run by its own agent.
- `architecture-resources/` (Java LS repo): read-only, always. Draft ADRs are delivered as text, never placed.
- Git: never commit, never push.
