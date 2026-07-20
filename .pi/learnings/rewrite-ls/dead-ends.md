# Dead ends — searches and assumptions that waste time

- No ADR directory exists in `ballerina-lang/language-server/docs/` — ADRs are referenced by number in Go rewrite comments but not stored locally.
- No "dual-snapshot" string anywhere in the Java LS or the Go rewrite — the concept lives only in Go rewrite comments as "Ticket 09" work.
- The TypeScript PoC (`ballerina-language-server-poc/`) is a single-file diagnostics-only prototype — no relevance to the dual-snapshot engine or feature architecture.
