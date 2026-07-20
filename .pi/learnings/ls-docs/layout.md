# Doc tree layout

- `architecture/adrs/` — 61 ADRs (ADR-001 through ADR-061), numbered by phase/iteration
- `architecture/domain-model/` — bounded-contexts, commands, domain-events, aggregates, context-map, subdomains, ubiquitous-language
- `architecture/structural/` — api-contracts, communication-patterns, component-responsibility-cohesion, data-storage-strategy, folder-structure, package-structure
- `architecture/scenarios/` — Gherkin .feature files + stress-test markdown
- `architecture/inputs/` — competitive analysis (DIM1–DIM18), LS analysis reports, dependency docs, research
- `architecture/inputs/competitive-anlaysis/` — note the typo in directory name ("anlaysis" not "analysis")
- Go LS backlog lives at `ballerina-go/docs/raw/ls-backlog/` (separate repo from `ls/` worktree, under `ballerina-go/docs/`). Contains map.md, 25 tickets (01–25), design/ and research/ subdirs.
- Go LS decisions live at `ballerina-go/docs/raw/decisions/` — one .md per resolved ticket.
- Go LS research lives at `ballerina-go/docs/raw/ls-backlog/research/` — one .md per researched ticket.
- Go LS design lives at `ballerina-go/docs/raw/ls-backlog/design/` — one .md per designed ticket.
