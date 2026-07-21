# gopls completion patterns

## CompletionItem internal struct
- `internal/golang/completion/completion.go:50-105` — `CompletionItem` struct: `Label`, `Detail`, `InsertText`, `Kind`, `Tags`, `Deprecated`, `AdditionalTextEdits`, `Depth`, `Score`, `snippet *snippet.Builder`, `Documentation`, `isSlice`. Invariant: does not refer to syntax or types.
- `internal/golang/completion/completion.go:68` — `Kind protocol.CompletionItemKind` — the internal struct uses the **protocol type directly**, not a local enum. No mapping layer exists.
- `internal/golang/completion/completion.go:130-133` — `Snippet()` method: returns `snippet.String()` if non-nil, else `InsertText`.
- `internal/golang/completion/completion.go:140-175` — `addConversion` wraps item in conversion expression: edits `InsertText` and `snippet`, adds `AdditionalTextEdits` for prefix before selector.
- `internal/golang/completion/completion.go:180-185` — Scoring constants: `lowScore=0.01`, `stdScore=1.0`, `highScore=10.0`.
- `internal/golang/completion/completion.go:188-196` — `matcher` interface: `Score(candidateLabel string) float32`. Implementations: `prefixMatcher` (case-sensitive prefix), `insensitivePrefixMatcher` (case-insensitive prefix), `fuzzy.Matcher`.
- `internal/golang/completion/completion.go:856-861` — `sortItems` sorts by `Score` descending using `sort.SliceStable` (stable sort preserves insertion order for ties).

## Candidate struct (internal scoring)
- `internal/golang/completion/completion.go:470-500` — `candidate` struct: `obj types.Object`, `score float64`, `name string`, `detail string`, `path []types.Object`, `pathInvokeMask uint16`, `mods []typeModKind`, `addressable bool`, `convertTo types.Type`, `imp *importInfo`. Used for deep search queue and scoring before conversion to `CompletionItem`.

## Deep completion state
- `internal/golang/completion/deep_completion.go:27` — `MaxDeepCompletions = 3` — limits deep completion results.
- `internal/golang/completion/deep_completion.go:30-55` — `deepCompletionState` struct: `enabled`, `queueClosed`, `thisQueue`, `nextQueue`, `highScores [MaxDeepCompletions]float64`, `candidateCount`.
- `internal/golang/completion/deep_completion.go:58-60` — `enqueue` adds to `nextQueue` (breadth-first).
- `internal/golang/completion/deep_completion.go:65-80` — `scorePenalty` penalizes deep candidates by depth/10, with slight bonus for unexported and penalty for functions.
- `internal/golang/completion/deep_completion.go:85-100` — `isHighScore` tracks top `MaxDeepCompletions` scores, returns false for low-scoring candidates (prunes search).
- `internal/golang/completion/deep_completion.go:122-131` — `stop()` closure: checks `ctx.Done()` first (user cancellation = immediate stop), then checks `deadline` (budget = stop only if past minDepth).
- `internal/golang/completion/deep_completion.go:190-200` — Every 100 candidates: checks `stop()`, and if `spent >= 0.85` of budget, closes the queue (no new sub-fields added) to finish current work.

## Snippet builder
- `internal/golang/completion/snippet/snippet_builder.go:24` — `Builder` struct: `currentTabStop int`, `sb strings.Builder`. Methods: `WriteText`, `WritePlaceholder(fn func(*Builder))`, `WriteFinalTabstop`, `WriteChoice`, `PrependText`, `Clone`, `String`.
- `internal/golang/completion/snippet/snippet_builder.go:62-68` — `WritePlaceholder` writes `${N:...}` with nested callback support. `WriteFinalTabstop` writes `$0`.
- `internal/golang/completion/snippet/snippet_builder.go:15-17` — Escape characters: `\`, `\}`, `\$` via `strings.NewReplacer`.
- `internal/golang/completion/snippet/snippet_builder.go:80-84` — `WriteChoice` writes `${N|a,b,c|}` with additional escaping of `|` and `,`.
- `internal/golang/completion/snippet.go:58-100` — `functionCallSnippet`: builds `name(${1:param1}, ${2:param2})` with placeholders. Skips snippet if already inside a call expression (reuses existing parens). Handles type parameters and regular parameters.

## Postfix snippets
- `internal/golang/completion/postfix_snippets.go:511` — `addPostfixSnippetCandidates`: adds artificial method snippets like `someSlice.sort!`. Gated by `ExperimentalPostfixCompletions` option.

## Statement and keyword completions
- `internal/golang/completion/statements.go:23` — `addStatementCandidates`: adds entire statement completions (e.g., `if`, `for`, `return`) in certain contexts. Runs last because it depends on other candidates.
- `internal/golang/completion/keywords.go:43` — `addKeywordCompletions`: adds keyword completions at file scope.
- `internal/golang/completion/keywords.go:189` — `addKeywordItems`: adds specific keyword items with given score.

## Capability negotiation
- `internal/server/general.go:150` — `CompletionProvider` declared with `TriggerCharacters: []string{"."}` only. No `ResolveProvider`, no `CompletionItem` options.
- `internal/settings/settings.go:1013-1017` — `ForClientCapabilities` reads `caps.TextDocument.Completion.CompletionItem.SnippetSupport` → sets `InsertTextFormat` to `SnippetTextFormat`; reads `InsertReplaceSupport` → sets `InsertReplaceSupported`.
- `internal/settings/settings.go:91-98` — `ClientOptions` stores `InsertTextFormat`, `InsertReplaceSupported`, `PreferredContentFormat`.
- `internal/settings/default.go:34` — Default `InsertTextFormat` is `PlainTextTextFormat`; default `PreferredContentFormat` is `Markdown`.

## Dispatch path
- `internal/protocol/tsserver.go:407-425` — Generated switch dispatches `"textDocument/completion"` → unmarshals `CompletionParams`, normalizes `Range` from `Position` if empty, validates position-in-range, calls `server.Completion`.
- `internal/server/completion.go:28` — `server.Completion` acquires snapshot via `s.session.FileOf`, dispatches on `snapshot.FileKind(fh)` to `golang.Completion`, `work.Completion`, or `template.Completion`.
- `internal/golang/completion/completion.go:514` — `func Completion(ctx, snapshot, fh, pos, context)` — the Go completion entry point. Calls `NarrowestPackageForFile` (triggers type-checking), then builds a `completer` struct.

## Result shape (`CompletionList`)
- `internal/protocol/tsprotocol.go:1223` — `IsIncomplete bool` field on `CompletionList`.
- `internal/server/completion.go:79` — `incompleteResults` set to `true` when `DeepCompletion` or `Matcher == Fuzzy`.
- `internal/server/completion.go:70-73` — Empty results (nil candidates) return `IsIncomplete: true` with empty items slice — signals client to keep requesting.
- `internal/server/completion.go:95-98` — Normal return: `CompletionList{IsIncomplete, Items}`.

## Snippet support
- `internal/golang/completion/snippet/snippet_builder.go:24` — `Builder` struct: `currentTabStop int`, `sb strings.Builder`. Methods: `WriteText`, `WritePlaceholder(fn func(*Builder))`, `WriteFinalTabstop`, `WriteChoice`, `PrependText`, `Clone`, `String`.
- `internal/golang/completion/snippet/snippet_builder.go:62-68` — `WritePlaceholder` writes `${N:...}` with nested callback support. `WriteFinalTabstop` writes `$0`.
- `internal/golang/completion/completion.go:99-102` — `CompletionItem.snippet *snippet.Builder` — internal field, not exported. `Snippet()` method returns snippet string or falls back to `InsertText`.
- `internal/server/completion.go:140-141` — `toProtocolCompletionItems` checks `options.InsertTextFormat == SnippetTextFormat` → uses `candidate.Snippet()` as `insertText`.
- `internal/golang/completion/completion.go:130-133` — `Snippet()` method: returns `snippet.String()` if non-nil, else `InsertText`.

## Insert vs Replace text edit
- `internal/server/completion.go:163-180` — If `InsertReplaceSupported`, uses `InsertReplaceEdit` with separate `Insert` (up to cursor) and `Replace` (full surrounding) ranges. Otherwise falls back to `TextEdit` with `replaceRng`.
- `internal/protocol/edits.go:166` — `SelectCompletionTextEdit` helper to pick insert vs replace mode from a `CompletionItem`.

## Sort order / determinism
- `internal/golang/completion/completion.go:856-861` — `sortItems` sorts by `Score` descending using `sort.SliceStable` (stable sort preserves insertion order for ties).
- `internal/server/completion.go:195-196` — `SortText: fmt.Sprintf("%05d", i)` — positional index as sort text, a hack for clients that don't respect server score ordering (LSP issue #348).
- `internal/golang/completion/unimported.go:261` — `slices.Sort(pkgs)` before iterating to avoid non-determinacy in unimported completions.

## Completion budget / cancellation
- `internal/golang/completion/completion.go:123` — `completionOptions.budget time.Duration`.
- `internal/golang/completion/completion.go:649-664` — Budget is a *soft* deadline: computed from `startTime + budget`, stored as `*time.Time` pointer (nil = unlimited). Deliberately NOT set on the context — user cancellation and budget are separate concerns.
- `internal/golang/completion/completion.go:686-689` — After collecting initial candidates, a `context.WithTimeout` is applied for the remaining work (expensive callbacks). This is acceptable because a minimal valid set already exists.
- `internal/golang/completion/deep_completion.go:122-131` — `stop()` closure: checks `ctx.Done()` first (user cancellation = immediate stop), then checks `deadline` (budget = stop only if past minDepth).
- `internal/golang/completion/deep_completion.go:190-200` — Every 100 candidates: checks `stop()`, and if `spent >= 0.85` of budget, closes the queue (no new sub-fields added) to finish current work.

## Selection / surrounding identifier
- `internal/golang/completion/completion.go:374-379` — `Selection` struct: `content`, `tokFile`, `start/end/cursor` (token.Pos), `mapper`. Methods: `Range()`, `PrefixRange()`, `Suffix()`.
- `internal/golang/completion/completion.go:412-443` — `setSurrounding` computes prefix from cursor position, sets the matcher (prefix, insensitive prefix, fuzzy).
- `internal/golang/completion/completion.go:445-455` — `getSurrounding()` returns a `Selection` with the computed prefix range.

## CompletionContext (trigger kind)
- `internal/golang/completion/completion.go:360-371` — `completionContext` struct stores `triggerCharacter` and `triggerKind` from `CompletionParams.Context`.
- `internal/protocol/tsprotocol.go:1320-1325` — `CompletionParams` has `Context CompletionContext` (only if client supports `contextSupport`).

## File kind dispatch
- `internal/server/completion.go:47-65` — Switch on `snapshot.FileKind(fh)`: `file.Go` → `golang.Completion`; `file.Mod` → nil; `file.Work` → `work.Completion`; `file.Tmpl` → `template.Completion`.
- Unsupported kinds return `IsIncomplete: true` with empty items (not an error).

## toProtocolCompletionItems conversion
- `internal/server/completion.go:80-200` — `toProtocolCompletionItems` converts internal `CompletionItem` to `protocol.CompletionItem`. Key steps: (1) filters deep completions beyond `MaxDeepCompletions` if not `DeepCompletion`, (2) selects `InsertText` vs `Snippet()` based on `InsertTextFormat`, (3) skips items with empty `insertText` (snippets disabled but candidate only supports snippet), (4) builds `Documentation` as Markdown or plain text, (5) builds `InsertReplaceEdit` or `TextEdit` based on `InsertReplaceSupported`, (6) sets `SortText: fmt.Sprintf("%05d", i)` as positional hack, (7) sets `FilterText: strings.TrimLeft(candidate.InsertText, "&*")`, (8) sets `Preselect: i == 0`.
- `internal/server/completion.go:191` — `Kind: candidate.Kind` — **direct passthrough**, no mapping. The internal `CompletionItem.Kind` (already `protocol.CompletionItemKind`) is copied verbatim to the protocol `CompletionItem.Kind`.
- `internal/server/completion.go:163-180` — Insert vs Replace: if `InsertReplaceSupported`, uses `InsertReplaceEdit` with separate `Insert` (up to cursor) and `Replace` (full surrounding) ranges. Otherwise falls back to `TextEdit` with `replaceRng`.
- `internal/server/completion.go:195-196` — `SortText: fmt.Sprintf("%05d", i)` — positional index as sort text, a hack for clients that don't respect server score ordering (LSP issue #348).
- `internal/server/completion.go:70-73` — Empty results (nil candidates) return `IsIncomplete: true` with empty items slice — signals client to keep requesting.
- `internal/server/completion.go:79` — `incompleteResults` set to `true` when `DeepCompletion` or `Matcher == Fuzzy`.

## CompletionItemKind: no local enum, direct protocol type
- gopls has **no local/internal CompletionItemKind enum**. The internal `CompletionItem` struct (`internal/golang/completion/completion.go:68`) uses `protocol.CompletionItemKind` directly.
- `internal/protocol/tsprotocol.go:1184` — `type CompletionItemKind uint32` — the protocol type definition.
- `internal/protocol/tsprotocol.go:6554-6575` — All 25 LSP constants (`TextCompletion=1` through `TypeParameterCompletion=25`).
- `internal/protocol/enums.go:24,60-84,161-162` — `namesCompletionItemKind` array and `Format()` method for string rendering.
- Kind is set directly to protocol constants at every creation site:
  - `internal/golang/completion/format.go:338-356` — `formatBuiltin()`: `protocol.ConstantCompletion`, `protocol.FunctionCompletion`, `protocol.InterfaceCompletion`, `protocol.ClassCompletion`, `protocol.VariableCompletion`.
  - `internal/golang/completion/format.go:100-130` — `item()` (main lexical path): `protocol.TextCompletion` (default), `protocol.ConstantCompletion`, `protocol.VariableCompletion`, `protocol.FieldCompletion`, `protocol.FunctionCompletion`, `protocol.MethodCompletion`, `protocol.ModuleCompletion`.
  - `internal/golang/completion/completion.go:1447-1454` — unimported selector path: `protocol.FunctionCompletion`, `protocol.VariableCompletion`, `protocol.ConstantCompletion`, `protocol.ClassCompletion`.
  - `internal/golang/completion/unimported.go:224-272` — unimported package symbols: `protocol.FunctionCompletion`, `protocol.VariableCompletion`, `protocol.ConstantCompletion`.
  - `internal/golang/completion/unimported.go:318` — module-indexed candidates: `protocol.FunctionCompletion`, `protocol.VariableCompletion`, `protocol.ConstantCompletion`.
  - `internal/golang/types_format.go:28-40` — `FormatType()`: `protocol.InterfaceCompletion`, `protocol.StructCompletion`, `protocol.ClassCompletion`.
- `internal/server/completion.go:191` — `toProtocolCompletionItems` passes `candidate.Kind` through verbatim — no mapping, no conversion.

## CompletionOptions settings
- `internal/settings/settings.go:369-395` — `CompletionOptions` struct: `UsePlaceholders bool`, `CompletionBudget time.Duration` (soft latency goal, 0=unlimited), `Matcher Matcher` (Fuzzy/CaseInsensitive/CaseSensitive), `ExperimentalPostfixCompletions bool`, `CompleteFunctionCalls bool`.
- `internal/settings/settings.go:866-871` — `Matcher` type: `Fuzzy="Fuzzy"`, `CaseInsensitive="CaseInsensitive"`, `CaseSensitive="CaseSensitive"`.
- `internal/settings/default.go:124-126` — Defaults: `CompletionBudget=100ms`, `CompleteFunctionCalls=true`, `Matcher=Fuzzy`.
- `internal/settings/default.go:141-143` — Internal defaults: `CompleteUnimported=true`, `DeepCompletion=true`, `CompletionDocumentation=true`.

## completer struct (per-request state)
- `internal/golang/completion/completion.go:200-280` — `completer` struct: `snapshot`, `pkg`, `qual` (types.Qualifier), `mq` (MetadataQualifier), `opts *completionOptions`, `completionContext`, `fh`, `filename`, `pgf`, `goversion`, `pos`, `path []ast.Node`, `seen map[types.Object]bool`, `items []CompletionItem`, `completionCallbacks []func(ctx, *imports.Options)`, `surrounding *Selection`, `inference candidateInference`, `enclosingFunc`, `enclosingCompositeLiteral`, `deepState`, `matcher`, `methodSetCache`, `tooNewSymbolsCache`, `mapper`, `startTime`, `scopes []*types.Scope`.
- `internal/golang/completion/completion.go:514-700` — `Completion()` entry point: (1) calls `NarrowestPackageForFile` (triggers type-checking), (2) finds enclosing path via `goastutil.PathEnclosingInterval`, (3) builds `completer` with all state, (4) computes deadline from `startTime + budget` (NOT set on context), (5) calls `collectCompletions` for initial candidates, (6) calls `deepSearch(ctx, 1, deadline)` for deep candidates, (7) applies `context.WithTimeout` for expensive callbacks, (8) runs `completionCallbacks` via `snapshot.RunProcessEnvFunc`, (9) calls `deepSearch(ctx, 0, deadline)` again for callback-populated candidates, (10) calls `addStatementCandidates`, (11) `sortItems`, returns.

## collectCompletions dispatch
- `internal/golang/completion/completion.go:720-790` — `collectCompletions` dispatches by context: (1) inside import spec → `populateImportCompletions`, (2) inside comment → `populateCommentCompletions`, (3) struct literal field → `structLiteralFieldName`, (4) label completion → `labels`, (5) empty switch → keywords only, (6) `*ast.Ident` → package name, selector, or `lexical`, (7) `*ast.TypeAssertExpr` → fake selector, (8) `*ast.SelectorExpr` → `selector`, (9) `*ast.BadDecl`/`*ast.File` → keywords, (10) default → `lexical`.

## Selection and surrounding identifier
- `internal/golang/completion/completion.go:374-379` — `Selection` struct: `content string`, `tokFile *token.File`, `start/end/cursor token.Pos`, `mapper *protocol.Mapper`. Methods: `Range()`, `PrefixRange()`, `Prefix()`, `Suffix()`, `check()`.
- `internal/golang/completion/completion.go:412-443` — `setSurrounding` computes prefix from cursor position, sets the matcher (prefix, insensitive prefix, fuzzy).
- `internal/golang/completion/completion.go:445-455` — `getSurrounding()` returns a `Selection` with the computed prefix range.
- `internal/golang/completion/completion.go:300-340` — `containingIdent` synthesizes a fake `*ast.Ident` for syntax errors (bad decls, empty switch, keyword-as-identifier, incomplete assign stmt). Uses `scanToken` to extract the token at position.

## CompletionContext (trigger kind)
- `internal/golang/completion/completion.go:360-371` — `completionContext` struct stores `triggerCharacter`, `triggerKind`, `commentCompletion bool`, `packageCompletion bool`.
- `internal/protocol/tsprotocol.go:1320-1325` — `CompletionParams` has `Context CompletionContext` (only if client supports `contextSupport`).

## File kind dispatch
- `internal/server/completion.go:47-65` — Switch on `snapshot.FileKind(fh)`: `file.Go` → `golang.Completion`; `file.Mod` → nil; `file.Work` → `work.Completion`; `file.Tmpl` → `template.Completion`.
- Unsupported kinds return `IsIncomplete: true` with empty items (not an error).
