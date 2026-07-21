# Dead ends — searches and assumptions that waste time

- `context/` package is the compiler's own context, NOT Go's `context.Context`. Don't confuse them.
- `main/` is a mirror of `ls-ref/` — always use `ls-ref/` as canonical.
- `model.SymbolRef` must never be used as a map key (per AGENTS.md). Use it as a value, not a key.
- Stale: `ExpectedType` concept now exists — `context.ExpectedSlotRecord`/`ExpectedSlotKind`, `projects.ExpectedTypeIndex`/`ExpectedTypeFact` were added for ticket 20. Don't assume it's absent; see `completion-infrastructure.md` and `gaps.md` for current state.
