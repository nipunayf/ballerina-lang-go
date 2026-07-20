# LS-docs learnings index

Durable index of what past exploration runs learned about the LS architecture
docs (bls-docs/feat3). Read this first, then open only the topic files
matching your query — plus `dead-ends.md`, always. Keep entries summarized and
pointer-dense — path + section heading, one line each.

- [layout.md](layout.md) — doc tree layout: ADRs, domain-model, structural, scenarios, inputs, Go LS backlog/decisions/research/design locations.
- [tickets.md](tickets.md) — backlog dependency chain and per-ticket findings (status, Java oracle contexts, decisions) for tickets 18, 20/21, 22, and the frontier (15, 17).
- [semantic-queries.md](semantic-queries.md) — StableSnapshot vs InProgressSnapshot, two-tier readiness, DualSnapshotStore, expected-type query gaps.
- [compiler-apis.md](compiler-apis.md) — BallerinaCompilerApi boundary (ADR-061), resolution-first pipeline (ADR-049), compiler immutability, cancellation model.
- [testing-constraints.md](testing-constraints.md) — txtar fixture loader, injectable WorkspaceCompiler/FileContentProvider/FileWatcherService, DocumentUri migration phases, facade boundary.
- [dead-ends.md](dead-ends.md) — ALWAYS read: confirmed absences (questions the docs don't answer) + superseded ADRs.

Maintenance: merge into topic headings, never ticket/date sections (ticket-numbered
findings go in `tickets.md` under their own heading, not a new top-level file per
ticket). Split files >~150 lines and update this index.
