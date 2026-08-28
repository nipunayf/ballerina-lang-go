# Go Patterns & Best Practices in ballerina-lang-go

This document catalogs the Go coding patterns, idioms, and best practices used across the
codebase. It is intended as a reference for contributors: which conventions to follow, which
are deliberate design choices, and which are Java-inherited legacy from the port.

**Context:** This is a Java-to-Go port of the Ballerina compiler + interpreter, organized as a
staged pipeline (parse → AST → symbol/type resolution → semantic analysis → desugar → BIR →
interpret). The architecture is cleanly layered with one-directional dependencies:
`semtypes` (leaf) ← `model` ← `context` ← `semantics`/`desugar` ← `bir` ← `runtime`, with
`values` and `decimal` as runtime leaves.

## 1. Type-system design patterns

**Sealed interfaces via unexported marker methods** — the Go substitute for Java's
`sealed`/abstract hierarchies, used everywhere:

- `TopLevelNode` requires unexported `isTopLevel()` (`ast/interfaces.go:63`); same for
  `StatementNode`, `BindingPatternNode`, `LExpr`
- `semtypes` seals `atom` via `index()`/`canonicalKey()` (`semtypes/atom.go:19`) and the BDD
  hierarchy (`semtypes/bdd.go:19`)
- Since only same-package types can implement, this restricts the hierarchy exactly like Java
  `sealed`

**Compile-time interface conformance assertions** — `var _ Symbol = &TypeSymbol{}` blocks
(`model/symbol.go:439-471`, `bir/non_terminator.go:206-231`) replace Java's `implements`
checks.

**Private `*Base` struct + embedding** (mandated in AGENTS.md) — the dominant composition
mechanism replacing Java inheritance. Bases are unexported, carry shared fields, and implement
shared methods once: `bLangNodeBase` (`ast/ast.go:64`), `symbolBase` (`model/symbol.go:232`),
`BIRNodeBase → BIRInstructionBase → BIRTerminatorBase` (`bir/model.go:44-167`), `analyzerBase`
embedded by all five semantic analyzers with a parent-chain for context access
(`semantics/semantic_analyzer.go:47`).

**Interface + `impl` suffix for hidden concrete types** — constructors return the interface:
`CharReader`/`charReaderImpl` (`tools/text/char_reader.go:25`),
`DiagnosticInfo`/`diagnosticInfoImpl`.

## 2. Identity by index, not pointer (the SymbolRef pattern)

The most emphasized project rule: never use `model.Symbol` as a map key — use
`SymbolRef{Index, SpaceIndex}`, a comparable two-int value struct (`model/symbol.go:185`)
resolving into a `SymbolSpace.symbols []Symbol` slice.

Rationale: map keys must be comparable and stable across serialization; pointer identity is
neither (symbols get copied during type narrowing). Enforcement is by deliberate panic —
`SymbolRef` implements `Symbol` but every method panics, and `AddSymbol` panics if handed a
ref (`model/symbol.go:474, 703-725`).

The same index-handle idea recurs throughout:

- Hash-consed type atoms (`semtypes/env.go:178`)
- `TypePoolIndex` for serialization
- Interned `*PackageID` so pointers are safe map keys (`model/package_id.go:57`)
- `Location` storing a compact `fileIndex int` instead of a filename
  (`tools/diagnostics/location.go:24`)

## 3. Error handling — three cleanly separated channels

**User-facing compiler errors are accumulated as diagnostics**, not returned:
`SemanticError`/`SyntaxError` append to a mutex-guarded slice on `CompilerContext`
(`context/context.go:159-172`); pipeline stages check `HasErrors()` to stop between phases.
Semantic-analysis functions almost never return `error`.

**Go `error` values only at genuinely fallible boundaries**: native functions return
`(BalValue, error)` (`runtime/extern/extern.go:31`), I/O utilities, and the `decimal` package
with typed errors and an `ErrorKind` enum (`decimal/decimal.go:45`). `%w` wrapping in the CLI.

**`panic` for invariant violations (compiler bugs)** — Java-`RuntimeException`-flavored but
consistently applied. Two idiomatic refinements:

- **Recover-to-error at package boundaries**: the BIR serializer/deserializer panic internally
  and convert to `error` at the top (`bir/codec/deserializer.go:48`), keeping parser-style
  happy paths branch-free.
- **Ballerina panics = Go panics carrying `*values.Error`**, and Ballerina `trap` = `recover`
  (`runtime/internal/exec/executor.go:147`); anything recovered that isn't a `*values.Error`
  is re-panicked as an interpreter bug.

## 4. Concurrency

- **Two-phase compilation**: stages 1–4 run sequentially in topological module order;
  stages 5–10 run as one goroutine per module with `sync.WaitGroup` plus a hand-rolled
  panic-collection bridge (recover in each goroutine, re-panic after `Wait()` —
  `projects/package_compilation.go:149-176`). No `errgroup`.
- **Ballerina strands map to goroutines** with a buffered result channel; the parent call
  stack is deep-copied so frames aren't aliased across goroutines
  (`runtime/internal/exec/strand.go`).
- **Strand-aware re-entrant mutexes** (`sync.Mutex` + `sync.Cond` tracking owner strand ID)
  implement Ballerina `lock` (`runtime/internal/locks/locks.go:45`).
- Shared state is explicitly guarded (`sync.RWMutex` on `SymbolSpace`, `sync.Map` for narrowed
  symbols), while `semtypes.Context` is documented as a per-goroutine thread-local cache,
  never shared.
- A **nightly CI run with `-race`** backs all of this (`.github/workflows/nightly-race.yml`).

## 5. Performance idioms

- **Size-classed `sync.Pool` frame allocator** — five pools of fixed-size
  `[8|16|32|64|128]BalValue` arrays for interpreter call frames; frames captured by closures
  are marked `escaped` and skip the pool (`runtime/internal/frame/frame.go`). The standout
  optimization.
- **Hash-consing / interning / memoization** throughout `semtypes`: atom tables,
  `SemtypeInterner` producing comparable `InternHandle`s usable as map keys, per-context memo
  caches for recursive types (`semtypes/env.go`, `semtype_interner.go`, `core.go`).
- **Env freezing**: after `Freeze()` the atom table becomes read-only for lock-free lookup,
  with new runtime atoms going to a `weak.Pointer` ephemeral store — modern Go 1.24
  (`semtypes/env.go:83-229`).
- Targeted `unsafe`: zero-copy `unsafe.String` for template strings, `unsafe.Pointer` identity
  keys for cycle detection.
- Precomputed buffer sizes (`EvalTemplateExpr` precomputes total literal length at BIR
  construction), cached dispatch targets on BIR instructions (`bir/terminator.go:46-51`),
  slice-as-stack tricks for call stacks.

## 6. Build tags & tooling

- **Debug/release two-file pairs**: `common/assert_debug.go` vs `assert_release.go` —
  `Assert(cond func() bool)` takes a *closure* so the condition isn't even evaluated in
  release (zero-cost assertions). Same pattern for lazy debug logging, detailed type-error
  printing (`semtypes/sem_type_printer_debug.go`), and the pprof profiler
  (`cli/cmd/prof_debug.go` wiring `net/http/pprof`, no-op in release).
- **golangci-lint v2** with a **depguard rule** forbidding `test_util` imports outside
  `_test.go` files — architectural boundaries enforced by lint, plus `internal/` packages
  enforcing them via the compiler (`runtime/internal/`, `common/tomlparser/internal/`).
- **Custom code generation**: `tree-gen` (in `compiler-tools/`) generates the red/green
  syntax-tree node files from `parser/nodes.json` via `//go:generate` + templates
  (`parser/tree/node.go:19`, `st_node.go:72`). Note: `bir/bir_gen.go` is hand-written —
  "gen" means BIR *generation*, not generated code.
- **CI**: benchmark time/memory regression comments on PRs, WASM (`GOOS=js GOARCH=wasm`) +
  native builds, nightly race detection.

## 7. Testing conventions

- **Golden-file corpus tests with `-update`** as the primary strategy (AGENTS.md: prefer
  corpus tests over unit tests): per-stage expected outputs under `corpus/<stage>/`, shared
  `UpdateIfNeeded` helpers (`test_util/update.go`), `txtar` archives for multi-stream outputs
  (stdout/stderr/exitcode), `go-diff` for readable failures, `t.Parallel()` everywhere, modern
  `for b.Loop()` benchmarks.
- **Test double for the whole platform**: the PAL (below) lets tests inject an in-memory
  platform (`test_util/testharness/`) — the runtime is fully platform-agnostic.
- Custom minimal assertion helpers instead of testify in the main module (`test_util/assert.go`);
  `go:embed` for CLI templates and test bala bundles.

## 8. Dependency injection — the PAL

All platform interaction (io, fs, http, time, signals) goes through the Platform Adaptation
Layer:

- `pal.Platform` is a **struct of function fields** (`FS{ReadFile func(...)...}`) rather than
  interfaces (`platform/pal/platform.go:50-73`) — lightweight DI, with an interface
  (`HTTPClient`) only where statefulness demands it.
- Production wires `os`/`time` (`platform/palnative/`); tests wire in-memory buffers.
- Elsewhere, `fs.FS` is injected into project loading (`projects.Load(projectFs fs.FS, ...)`).
- Dependency inversion via `any` "opaque pointer" fields breaks import cycles between
  `runtime/extern` and the interpreter core (`runtime/extern/extern.go:36,48`).

## 9. Modern Go features in use

Go 1.26 module. **Generics used sparingly and deliberately** — `common.Optional[T]`,
`Set[T]`/`OrderedMap[K,V]`, `DependencyGraph[T comparable]`, generic subtype algebra in
`semtypes` (`enumerable_subtype.go`) — while the hot interpreter path deliberately sticks to
`any` + type switches. **`iter.Seq`/`iter.Seq2` range-over-func iterators** for node/symbol
traversal, **`weak.Pointer`**, typed `sync/atomic` values, `slices`/`maps` for defensive
copies at API boundaries.

## Java-inherited vs idiomatic Go

Because this is a port, two styles coexist — know which is which before imitating:

| Java-inherited (tolerated legacy) | Idiomatic Go (the direction of travel) |
|---|---|
| `BLang*`/`ST*` prefixes, `SCREAMING_SNAKE` enum members like `NodeKind_ANNOTATION` (staticcheck naming rules are explicitly disabled for this) | `iota` enums + hand-written `String()` |
| ~90 interface-per-node contracts with `GetX`/`SetX` pairs (`ast/interfaces.go`) | Small composed interfaces (`Node` is 2 methods) |
| Fluent `WithX().Build()` builders, tri-state `optionalBool`, `Optional.Get()` that panics | Options structs, `fs.FS`/PAL injection |
| Base-struct methods that panic "should be implemented by child types" (abstract-method emulation) | Sealed interfaces via unexported markers |
| `IllegalArgumentError`/`IndexOutOfBoundsError` exception classes, error hierarchies via embedding + type switch | Diagnostics accumulation, `%w` wrapping, recover-to-error |
| Explicit bit-position `Flag uint64` and `iota+N` opcode ranges (kept for wire stability) | Array-indexed op vtable (`ops[code]`), bitset `SemType` |
| One-type-per-file granularity, `// Java source:` provenance comments | `internal/` packages, depguard-enforced boundaries |

## Patterns most worth emulating

- Index-handles instead of pointer keys (`SymbolRef`, atoms, interned handles)
- Closure-based build-tag-gated assertions (zero cost in release)
- The PAL struct-of-functions DI with a full in-memory test double
- Recover-to-error at codec boundaries
- Size-classed `sync.Pool` allocators for hot-path objects
- Hash-consing with env freezing for lock-free reads
- Golden-file corpus testing with `-update`
