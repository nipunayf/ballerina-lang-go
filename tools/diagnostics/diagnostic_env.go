// Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package diagnostics

import (
	"fmt"
	"sync"
	"sync/atomic"

	"ballerina-lang-go/tools/text"
)

// compileInstance is a per-PackageCompilation identity token minted when a
// compile starts. DiagnosticEnv namespaces file registrations by instance so a
// shared (persistent per source root) env can be re-compiled across generations
// without the same-name/different-doc panic that blocked the ADR-042 modifier
// chain (ticket-09 prerequisite). The zero value is the default namespace used
// by standalone env owners (e.g. the TOML parser) that never call BeginCompile.
type compileInstance uint64

// Sentinel fileIndex values for synthetic sources that carry no TextDocument
// and are not registered in the env's slice. Real files use positive indices
// (1-based) assigned at RegisterFile time so the zero-value Location maps to
// UnknownFileIndex.
const (
	// UnknownFileIndex marks a Location with no associated source file.
	// Value 0 so a zero-value Location is treated as unknown.
	UnknownFileIndex = 0
	// BuiltinFileIndex marks a Location in the compiler's synthetic "<built-in>"
	// source. Built-in locations cannot carry user-visible errors.
	BuiltinFileIndex = -1
	// BallerinaTomlFileIndex marks a Location in Ballerina.toml. Manifest
	// validation diagnostics use this without resolving offsets to line/column.
	BallerinaTomlFileIndex = -2
)

const (
	builtinFileName       = "<built-in>"
	ballerinaTomlFileName = "Ballerina.toml"
)

// DiagnosticEnv resolves byte-offset-based Locations to line/column numbers.
// It maps file names to integer indices for compact storage in Location.
// Thread-safe via RWMutex since it is shared across compilation phases.
//
// Under the ticket-09 prerequisite the env is shared and persistent per source
// root, so fileIndex is globally stable across compile instances: an unchanged
// file reused by doc pointer across instances no-ops to its existing index
// (keeping symbol-Locations valid), while a changed file under a new instance
// allocates a new index instead of panicking. The default namespace (no
// BeginCompile) preserves the legacy same-name/different-doc panic for
// standalone env owners.
type DiagnosticEnv struct {
	mu          sync.RWMutex
	fileNames   []string
	docs        []text.TextDocument
	nameToIndex map[string]int // default namespace (instance 0): fileName -> fileIndex
	byInstance  map[compileInstance]map[string]int
	byDoc       map[text.TextDocument]int // doc pointer -> fileIndex (cross-instance reuse)
	active      compileInstance           // currently compiling instance; 0 = default
	nextInst    uint64
}

// NewDiagnosticEnv creates an empty DiagnosticEnv.
func NewDiagnosticEnv() *DiagnosticEnv {
	return &DiagnosticEnv{
		nameToIndex: make(map[string]int),
		byInstance:  make(map[compileInstance]map[string]int),
		byDoc:       make(map[text.TextDocument]int),
	}
}

// BeginCompile mints a new compile-instance token, marks it active on the env,
// and allocates its fileName namespace. It must be called before the compile
// registers files or resolves Locations. Under the LS single-flight-per-root
// rule at most one compile is active on a given env at a time. Returns the
// token to pass to EndCompile.
func (de *DiagnosticEnv) BeginCompile() compileInstance {
	inst := compileInstance(atomic.AddUint64(&de.nextInst, 1))
	de.mu.Lock()
	de.active = inst
	de.byInstance[inst] = make(map[string]int)
	de.mu.Unlock()
	return inst
}

// EndCompile clears the active instance. Call after the compile finishes so a
// later non-compile FileIndex/RegisterFile call falls back to the default
// namespace. The instance's fileName namespace is retained so Locations built
// during that compile keep resolving via their fileIndex.
func (de *DiagnosticEnv) EndCompile(inst compileInstance) {
	de.mu.Lock()
	if de.active == inst {
		de.active = 0
	}
	de.mu.Unlock()
}

// RegisterFile adds or updates a file in the environment. Assigns 1-based
// indices so zero-value Location (fileIndex=0) is unknown.
//
// In the default namespace (no active compile instance) it preserves the
// legacy contract: same-name/same-doc no-ops; same-name/different-doc panics.
// Under an active compile instance, a same-name registration under a different
// instance allocates a new file index (no panic), and a same doc pointer reused
// across instances no-ops to the existing index (symbol-Location stability).
func (de *DiagnosticEnv) RegisterFile(fileName string, doc text.TextDocument) {
	de.mu.Lock()
	defer de.mu.Unlock()
	if de.active == 0 {
		de.registerDefault(fileName, doc)
		return
	}
	names := de.byInstance[de.active]
	if existing, ok := names[fileName]; ok {
		// Same instance already registered this fileName. Same doc -> no-op;
		// different doc -> a genuine within-instance duplicate (shouldn't happen;
		// keep the safety panic).
		if de.docs[existing-1] == doc {
			return
		}
		panic(fmt.Sprintf("diagnostics: duplicate file declarations with same name: %q", fileName))
	}
	// Cross-instance reuse: an unchanged file re-registered with the same doc
	// pointer no-ops to its existing globally-stable index.
	if idx, ok := de.byDoc[doc]; ok {
		names[fileName] = idx
		return
	}
	de.fileNames = append(de.fileNames, fileName)
	de.docs = append(de.docs, doc)
	idx := len(de.fileNames)
	names[fileName] = idx
	de.byDoc[doc] = idx
}

// registerDefault implements the legacy default-namespace registration.
func (de *DiagnosticEnv) registerDefault(fileName string, doc text.TextDocument) {
	if idx, ok := de.nameToIndex[fileName]; ok {
		if de.docs[idx-1] == doc {
			return
		}
		panic(fmt.Sprintf("diagnostics: duplicte file declarations with same name: %q", fileName))
	}
	de.fileNames = append(de.fileNames, fileName)
	de.docs = append(de.docs, doc)
	idx := len(de.fileNames)
	de.nameToIndex[fileName] = idx
}

// FileIndex returns the index for a previously registered file name. Callers
// are expected to have invoked RegisterFile first; this panics otherwise so
// missing registrations surface immediately. Under an active compile instance
// it returns that instance's index for fileName.
func (de *DiagnosticEnv) FileIndex(fileName string) int {
	de.mu.RLock()
	defer de.mu.RUnlock()
	if de.active == 0 {
		idx, ok := de.nameToIndex[fileName]
		if !ok {
			panic(fmt.Sprintf("diagnostics: file not registered: %q", fileName))
		}
		return idx
	}
	idx, ok := de.byInstance[de.active][fileName]
	if !ok {
		panic(fmt.Sprintf("diagnostics: file not registered: %q", fileName))
	}
	return idx
}

// FileName returns the file name for a Location.
func (de *DiagnosticEnv) FileName(loc Location) string {
	switch loc.fileIndex {
	case UnknownFileIndex:
		return ""
	case BuiltinFileIndex:
		return builtinFileName
	case BallerinaTomlFileIndex:
		return ballerinaTomlFileName
	}
	de.mu.RLock()
	defer de.mu.RUnlock()
	slot := loc.fileIndex - 1
	if slot < 0 || slot >= len(de.fileNames) {
		panic(fmt.Sprintf("diagnostics: fileIndex %d out of range (have %d files)", loc.fileIndex, len(de.fileNames)))
	}
	return de.fileNames[slot]
}

func (de *DiagnosticEnv) getDoc(loc Location) text.TextDocument {
	de.mu.RLock()
	defer de.mu.RUnlock()
	slot := loc.fileIndex - 1
	return de.docs[slot]
}

// StartLine returns the 0-based start line for the given Location.
// Panics if the Location has no associated source. Callers must check
// IsLocationEmpty and synthetic sentinels (built-in, Ballerina.toml) first.
func (de *DiagnosticEnv) StartLine(loc Location) int {
	doc := de.requireDoc(loc, "StartLine")
	line, _, err := doc.LinePositionFromTextPosition(loc.startOffset)
	if err != nil {
		panic(fmt.Sprintf("diagnostics: StartLine: failed to resolve startOffset %d for loc %+v: %v", loc.startOffset, loc, err))
	}
	return line
}

// StartColumn returns the 0-based start column for the given Location.
// Panics if the Location has no associated source; see StartLine.
func (de *DiagnosticEnv) StartColumn(loc Location) int {
	doc := de.requireDoc(loc, "StartColumn")
	_, col, err := doc.LinePositionFromTextPosition(loc.startOffset)
	if err != nil {
		panic(fmt.Sprintf("diagnostics: StartColumn: failed to resolve startOffset %d for loc %+v: %v", loc.startOffset, loc, err))
	}
	return col
}

// EndLine returns the 0-based end line for the given Location.
// Panics if the Location has no associated source; see StartLine.
func (de *DiagnosticEnv) EndLine(loc Location) int {
	doc := de.requireDoc(loc, "EndLine")
	line, _, err := doc.LinePositionFromTextPosition(loc.endOffset)
	if err != nil {
		panic(fmt.Sprintf("diagnostics: EndLine: failed to resolve endOffset %d for loc %+v: %v", loc.endOffset, loc, err))
	}
	return line
}

// EndColumn returns the 0-based end column for the given Location.
// Panics if the Location has no associated source; see StartLine.
func (de *DiagnosticEnv) EndColumn(loc Location) int {
	doc := de.requireDoc(loc, "EndColumn")
	_, col, err := doc.LinePositionFromTextPosition(loc.endOffset)
	if err != nil {
		panic(fmt.Sprintf("diagnostics: EndColumn: failed to resolve endOffset %d for loc %+v: %v", loc.endOffset, loc, err))
	}
	return col
}

func (de *DiagnosticEnv) requireDoc(loc Location, caller string) text.TextDocument {
	doc := de.getDoc(loc)
	if doc == nil {
		panic(fmt.Sprintf("diagnostics: %s: no source for loc %+v (fileIndex=%d)", caller, loc, loc.fileIndex))
	}
	return doc
}
