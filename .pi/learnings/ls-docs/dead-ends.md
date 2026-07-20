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
