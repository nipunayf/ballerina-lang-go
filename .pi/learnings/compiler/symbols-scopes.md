# Symbols & scopes

Keep entries summarized and pointer-dense — `path` + symbol, one line each.

## CompilerContext symbol queries

- `CompilerContext.GetSymbol(ref)` → `model.Symbol` — delegates to `CompilerEnvironment.GetSymbol()`. `explore-codebase/context/context.go:80`
- `CompilerContext.SymbolName(ref)` → `string` — `explore-codebase/context/context.go:85`
- `CompilerContext.SymbolType(ref)` → `semtypes.SemType` — `explore-codebase/context/context.go:90`
- `CompilerContext.SetSymbolType(ref, ty)` — `explore-codebase/context/context.go:110-115`
- `CompilerContext.SymbolKind(ref)` → `model.SymbolKind` — `explore-codebase/context/context.go:100`
- `CompilerContext.SymbolIsPublic(ref)` → `bool` — `explore-codebase/context/context.go:105`
- `CompilerContext.UnnarrowedSymbol(ref)` → `model.SymbolRef` — `explore-codebase/context/context.go:70`
- `CompilerContext.CreateNarrowedSymbol(baseRef)` → `model.SymbolRef` — `explore-codebase/context/context.go:65`
- `CompilerContext.GetTypeEnv()` → `semtypes.Env` — `explore-codebase/context/context.go:175`
- `CompilerContext.NewSymbolSpace(packageID)` → `*model.SymbolSpace` — `explore-codebase/context/context.go:35`
- `CompilerContext.NewFunctionScope(parent, pkg)` → `*model.FunctionScope` — `explore-codebase/context/context.go:40`
- `CompilerContext.NewBlockScope(parent, pkg)` → `*model.BlockScope` — `explore-codebase/context/context.go:45`
- `CompilerContext.AddSymbolToSameSpace(ref, name, symbol)` → `model.SymbolRef` — `explore-codebase/context/context.go:50`
- `CompilerContext.CreateFunctionSymbol(space, name, sig, fnTy)` → `model.SymbolRef` — `explore-codebase/context/context.go:60`
- `CompilerContext.DiagnosticEnv()` → `*diagnostics.DiagnosticEnv` — `explore-codebase/context/context.go:30`
- `CompilerContext.Diagnostics()` → `[]diagnostics.Diagnostic` — `explore-codebase/context/context.go:170`
- `CompilerContext.HasErrors()` → `bool` — `explore-codebase/context/context.go:155`
- `CompilerEnvironment.FindSymbol(pkg, name)` — slow lookup for serialization, not for general use. `explore-codebase/context/env.go:100-115`
- `CompilerContext.SymbolLocation(ref)` → `diagnostics.Location` — exists. `explore-codebase/context/context.go:172`
- `CompilerContext.SymbolIsClass(ref)` → `bool` — exists. `explore-codebase/context/context.go:184`
- `CompilerContext.SymbolPackage(ref)` → `model.PackageIdentifier` — exists. `explore-codebase/context/context.go:146`
- `CompilerContext.ValueSymbolMetadata(ref)` → `(ValueSymbolMetadata, bool)` — exists. `explore-codebase/context/context.go:188`
- `CompilerContext.DistinctTypeID(ref)` → `int` / `CompilerContext.DistinctTypeSymbolRef(id)` → `(model.SymbolRef, bool)` — both exist. `explore-codebase/context/context.go:204-208`
- `CompilerContext.SymbolAnnotationValues(ref)` → `values.AnnotationValues` — exists. `explore-codebase/context/context.go:200`

## Symbol types (model package)

- `model.SymbolRef` — `{Package PackageIdentifier, Index int, SpaceIndex int}` — the canonical symbol handle; **must never be used as a map key**. `explore-codebase/model/symbol.go:80-85`
- `model.Symbol` interface — `Name()`, `Type()`, `Kind()`, `SetType()`, `IsPublic()`, `Copy()` — `explore-codebase/model/symbol.go:50-60`
- `model.ValueSymbol` — extends Symbol with `IsConst()`, `IsParameter()`, `IsIsolated()`, `IsFinal()`, `IsConfigurable()` — `explore-codebase/model/symbol.go:65-75`
- `model.FunctionSymbol` — extends Symbol with `Signature()`, `DefaultableParams()`, `IncludedRecordParams()`, `ParamNames()` — `explore-codebase/model/symbol.go:140-150`
- Concrete symbol types: `TypeSymbol`, `ValueSymbol`, `functionSymbol`, `classSymbol`, `NetworkClassSymbol`, `RecordSymbol`, `ObjectTypeSymbol`, `containerGenericFunctionSymbol`, `dependentlyTypedFunctionSymbol`, `monomorphicFunctionSymbol`, `ResourceMethodSymbol` — `explore-codebase/model/symbol.go:200-400`
- `model.SymbolKind` enum — `SymbolKindType`, `SymbolKindConstant`, `SymbolKindVariable`, `SymbolKindParemeter` (sic), `SymbolKindFunction` — `explore-codebase/model/symbol.go:200-210`
- `model.FunctionSignature` struct — `ParamTypes`, `ParamNames`, `ReturnType`, `RestParamType`, `Flags` — `explore-codebase/model/symbol.go:350-360`
- `model.InclusionMember` interface — `MemberName()`, `MemberKind()`, `MemberType()`, `SetMemberType()`; implemented by `FieldDescriptor`, `MethodDescriptor`, `RestTypeDescriptor` — `explore-codebase/model/symbol.go:300-310`
- `model.FieldDescriptor` — InclusionMember + `IsPublic()`, `IsReadonly()`, `IsOptional()`, `HasDefault()` — `explore-codebase/model/symbol.go:320-340`
- `model.MethodDescriptor` — InclusionMember + `MethodRef SymbolRef` — `explore-codebase/model/symbol.go:360-375`
- `model.MemberCarrier` interface — `Members() []InclusionMember`, `AddMember()`, `FieldDefaults()`; implemented by `RecordSymbol`, `ClassSymbol`, `ObjectTypeSymbol` — `explore-codebase/model/symbol.go:400-420`
- `model.ClassSymbol` interface — `Symbol`, `MemberCarrier`, `SetMethods(map[string]SymbolRef)`, `MethodSymbol(name) (SymbolRef, bool)` — `explore-codebase/model/symbol.go:400-410`
- `model.RecordSymbol.Fields()` — iterates field descriptors — `explore-codebase/model/symbol.go:420-430`
- `model.RecordSymbol.Field(name)` — looks up field by name — `explore-codebase/model/symbol.go:430-440`
- `model.RecordSymbol.RestField()` — returns rest type descriptor — `explore-codebase/model/symbol.go:440-450`
- `model.SymbolSpace.Symbols()` — returns `iter.Seq[SymbolRef]` over a point-in-time snapshot of refs, not `iter.Seq2[int, Symbol]` — `explore-codebase/model/symbol.go:600-614`
- `model.SymbolSpace.RefAt(index)` — returns `SymbolRef` at index — `explore-codebase/model/symbol.go:185-190`
- `model.SymbolSpace.Len()` — returns symbol count — `explore-codebase/model/symbol.go:175-180`
- `model.SymbolSpace.Pkg` — `PackageIdentifier` field — `explore-codebase/model/symbol.go:180-185`
- `model.PackageIdentifier` struct — `Organization`, `Package`, `Version` — `explore-codebase/model/symbol.go:80-85`

## Scopes & symbol spaces

- `model.Scope` interface — `GetSymbol(name)`, `GetPrefixedSymbol(prefix, name)`, `AddSymbol(name, symbol)`, `LookupXMLNS(prefix)`, `DefineXMLNS(prefix, uri)` — `explore-codebase/model/symbol.go:30-35`
- Scope hierarchy — `ModuleScope` (Main + Prefix + Annotation + XMLNS), `FunctionScope`, `BlockScope` — `explore-codebase/model/symbol.go:100-150`
- `model.ModuleScope` — `Main *SymbolSpace`, `Prefix map[string]ExportedSymbolSpace`, `Annotation *SymbolSpace`, `XMLNS map[string]string` — `explore-codebase/model/symbol.go:220-225`
- `ModuleScope.GetPrefixedSymbol(prefix, name)` — resolves `prefix:name` references, maps lang prefixes (int→lang.int, etc.) — `explore-codebase/model/symbol.go:350-360`
- `ModuleScope.Exports()` → `ExportedSymbolSpace` — `explore-codebase/model/symbol.go:280-285`
- `BlockScopeBase.GetSymbol(name)` — walks parent chain. `explore-codebase/model/symbol.go:470-480`
- `BlockScopeBase.GetPrefixedSymbol(prefix, name)` — delegates to parent. `explore-codebase/model/symbol.go:475-480`
- `model.SymbolSpace` — thread-safe (`mu sync.RWMutex` on lookup table + symbols slice); `GetSymbol(name)`, `SymbolAt(index)`, `AppendSymbol(sym)`, `RefAt(index)`, `Symbols() iter.Seq[SymbolRef]`, `Len()`, `SpaceIndex()`, `Pkg` — `explore-codebase/model/symbol.go:180-190`
- `model.ExportedSymbolSpace` — `Main *SymbolSpace`, `Annotation *SymbolSpace` — `explore-codebase/model/symbol.go:240-245`
- `ExportedSymbolSpace.PublicMainSymbols()` — iterates public symbols. `explore-codebase/model/symbol.go:450-460`
- `ExportedSymbolSpace.GetSymbol(name)` — looks up public symbol by name (only public). `explore-codebase/model/symbol.go:460-470`
- `model.BlockLevelScope` interface — combines `Scope` + `SymbolSpaceProvider` — `explore-codebase/model/symbol.go:40-45`
- `model.SymbolSpaceProvider` interface — `MainSpace() *SymbolSpace` — `explore-codebase/model/symbol.go:35-40`
