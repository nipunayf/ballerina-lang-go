# Semtypes

Keep entries summarized and pointer-dense — `path` + symbol, one line each.

## Core type values & algebra

- `semtypes.SemType` — opaque type value (bitmask + optional subtype data) — `explore-codebase/semtypes/core.go:30-40`
- `semtypes.IsSubtype(cx, t1, t2)` — subtype check — `explore-codebase/semtypes/core.go:250-255`
- `semtypes.IsEmpty(cx, t)` — emptiness check — `explore-codebase/semtypes/core.go:232-250`
- `semtypes.IsNever(t)` — never check — `explore-codebase/semtypes/core.go:228-232`
- `semtypes.Union`, `Intersect`, `Diff` — type algebra — `explore-codebase/semtypes/core.go:53-230`

## Member-type queries

- `semtypes.MappingMemberTypeInnerVal(cx, t, k)` — member type of mapping T for key K — `explore-codebase/semtypes/core.go:522-526`
- `semtypes.MappingMemberTypeInnerValProj(cx, t, k)` — projection variant (excludes UNDEF) — `explore-codebase/semtypes/mapping_proj.go:19-22`
- `semtypes.ObjectMemberType(ctx, name, ty)` — type of object member — `explore-codebase/semtypes/member.go:118-125`
- `semtypes.ObjectMemberKind(ctx, name, ty)` — kind (field|method|remote-method|resource-method) — `explore-codebase/semtypes/member.go:98-105`
- `semtypes.ObjectMemberVisibility(ctx, name, ty)` — visibility of object member — `explore-codebase/semtypes/member.go:108-115`
- `semtypes.Member` struct — `Name`, `ValueTy`, `Kind`, `Visibility`, `Immutable` — `explore-codebase/semtypes/member.go:10-20`
- `semtypes.ListMemberType(context, t, key)` — member type of list T for index K — `explore-codebase/semtypes/sem_types.go:95-100`
- `semtypes.ListProj(context, t, key)` — list projection — `explore-codebase/semtypes/sem_types.go:90-95`

## Context & environment

- **`semtypes.Context` is thread-local — never use concurrently.** `explore-codebase/semtypes/context.go:10-15`
- `semtypes.ContextFrom(env)` — creates a fresh context from an Env — `explore-codebase/semtypes/context.go:150-170`
- `semtypes.Env` — the type environment, accessible via `CompilerEnvironment.GetTypeEnv()` — `explore-codebase/context/env.go:40-45`
