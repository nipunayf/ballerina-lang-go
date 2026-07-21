# Dead ends

## Confirmed absences (questions the docs don't answer)

- No ADR or doc specifically addresses completion-item deduplication or sorting strategy for merged plugin+core completions (ADR-029 mentions it as a negative consequence but doesn't resolve it).
- No doc specifies the exact completion trigger characters or context classification (e.g., dot-completion vs. keyword completion).
- No doc specifies the semantic tokens classification or token types/modifiers list.
- No doc specifies the exact format or schema of the persistent symbol index (ADR-020 §4 mentions location and content scope but not schema).
- No doc specifies the exact `retryAfterMs` value for cold-start ContentModified error (ADR-020 §2).
- No doc specifies the exact staleness annotation format for stale-result serving (ADR-020 §2).
- No doc specifies the exact `isApplicable` check implementation for LanguageFeatureProvider (ADR-029 mentions O(1) requirement but no implementation).
- No doc specifies the exact `CancelChecker` interface or its integration with LSP4j cancellation (ADR-042 mentions it on InProgressSnapshot only).
- No ADR or decision doc specifically addresses expected-type completion projections (ticket 20 is a `grilling`-type ticket, status `claimed`, blocked by ticket 12 (resolved), with no research/design output yet). Ticket 20 is NOT the next completion-track ticket after the vertical slice (14): the charted order is 17→18→19→20/21→22→23→24/25. Tickets 17 (module/import/declaration) and 15 (semantic hover) are the actual frontier (open, unblocked, unclaimed).
- No ADR or decision doc specifically addresses the coupling of completion item kinds to the protocol layer — the decision is implicit in the architecture (protocol-free core DTOs, server-only LSP adaptation) and enforced in code, not separately documented as a design choice.
- **Ticket 18 has no research or design output yet:** `research/18*` and `design/18*` don't exist. The ticket is type=implement, status=claimed, blocked by none. The decision docs for 14 (completion vertical slice) and 17 (module/import/declaration) are the closest precedents.
- The compiler has no `ExpectedType` query for LS use — `setExpectedType` is a compiler-internal function that stores the expected type on AST nodes during type resolution, not a query API (`semantics/type_resolver.go`, `semantics/semantic_analyzer.go`).
- Ticket 20's question explicitly asks about "private compiler-owned semantic projection" for expected type, assignability, invocation signature, and argument-position facts — this is deferred from the completion vertical slice (14) and the semantic query surface (12). The HIL-selected initial coverage is the core contextual tier: typed assignment/initialization, returns, positional/named arguments, mapping/list fields, `new`/initializer arguments, conditions, and `panic`/`check`.
- The backlog directory lives at `/Users/wso2/projects/ballerina/ballerina-go/docs/raw/ls-backlog/` (a separate repo from the `ls/` worktree, under `ballerina-go/docs/`). Contains map.md, 25 tickets (01–25), design/ and research/ subdirs. Ticket 20 (`20-design-expected-type-completion-projections.md`) exists: type=grilling, status=claimed, blocked by 12 (resolved). No research/ or design/ output yet for ticket 20.
- The docs vault task T-020 (`integration-wiring`) is a different T-020 — it's about wiring cross-context event subscriptions, not about completion projections. No relation to ticket 20.
- jBallerina's `SemanticModel` does expose `expectedType(Document, LinePosition)` returning `Optional<TypeSymbol>` — this is the jBallerina API that the Go interpreter's LS would need to replicate. Documented in `architecture/inputs/dependencies/ballerina-lang.md:26`.
- **Ticket 15 (semantic hover) blocks no other ticket.** Its `Blocked by: none` and no other ticket lists 15 as a blocker. The dependency chains are: 17←14, 18←17, 19←18, 20←12, 21←20, 22←18+21, 23←18, 24←12, 25←24, 16←12, 11←10, 12←10. Ticket 15 is the open frontier (map.md: "Implement semantic hover (15) is the next frontier"). No design/ or research/ output exists for 15 yet. `docs/raw/ls-backlog/issues/15-implement-semantic-hover.md:1-4`, `docs/raw/ls-backlog/map.md:Not yet specified §M4`

## Dead ends

- `architecture/inputs/competitive-anlaysis/` — directory name has typo "anlaysis" not "analysis"
- ADR-034 is superseded by ADR-040 and reshaped by ADR-059 — do not cite as current constraint
- ADR-045 is superseded by ADR-061 — do not cite the strict "all compiler calls through BallerinaCompilerApi" rule
- ADR-042 storage mechanics superseded by ADR-058 — DualSnapshotStore is the current central store
- ADR-013 snapshot-memory mechanics superseded by ADR-058
- ADR-030 facade purity details superseded by ADR-059
- ADR-040 raw-Path boundary details superseded by ADR-059
- ADR-027 superseded by ADR-051 (TOML changes as document changes)
- ADR-035 superseded by ADR-049 (locking mode → recovery ladder)
- **Ticket 32 (completion reader/writer strategy) has no prior design comparison to Salsa-style incrementality:** the question is new to ticket 32 itself (ticket 32 is grilling, claimed, no research/design output yet). Current design rationale is avoiding ls-ref's sync-per-request pattern (2026-07-16-completion-vertical-slice.md:Rationale), not a documented comparison against genuinely-incremental-query alternatives.
- **No prior design decision on compile-generation granularity (whole-package vs per-file/per-function):** granularity is implicit in ADR-049 (resolution-first per package) and ADR-058 (dual snapshots per source root). Docs show *that* the compiler walks modules/packages via `PackageCompilation` (projects/package_compilation.go §sync.Once), not *why* finer granularities were rejected. Compiler per-file/per-function memoization deferred by monolithic walk-and-bind design (ticket 32 question 2).
- **No design note on lease-miss window shrinking:** term "lease-miss window" originates in ticket 32 itself. Current design (precomputed-index + generation-matched-lease + degrade fallback) documented in 2026-07-16-completion-vertical-slice.md (index published with snapshot at full-compile boundary). ADR-018 §2 downgrades mid-compile phase-boundary checkpoints to "future enhancement pending compiler API hooks" (2026-07-14-dual-snapshot-compiler-engine.md:41-44). Debounce tuning and cooperative compile cancellation not separately designed; debounce default 150ms tunable in 09 (2026-07-14-dual-snapshot-compiler-engine.md:396).
