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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package diagnostics

import (
	"testing"

	"ballerina-lang-go/tools/text"
)

// locForIndex builds a Location pointing at the given fileIndex, for FileName
// resolution. Fields are unexported; this test lives in-package.
func locForIndex(idx int) Location { return Location{fileIndex: idx} }

// TestDiagnosticEnvPerInstanceNoPanic verifies the ticket-09 prerequisite: a
// shared DiagnosticEnv (persistent per source root) must allow re-compiling
// the same source root across compile instances without panicking. A changed
// file (same name, different doc pointer) under a new instance allocates a new
// file index instead of panicking.
func TestDiagnosticEnvPerInstanceNoPanic(t *testing.T) {
	de := NewDiagnosticEnv()

	inst1 := de.BeginCompile()
	de.RegisterFile("root/a.bal", text.NewStringTextDocument("content a v1"))
	idxA1 := de.FileIndex("root/a.bal")
	de.EndCompile(inst1)

	// Recompile on the shared env with changed content for the same name.
	inst2 := de.BeginCompile()
	de.RegisterFile("root/a.bal", text.NewStringTextDocument("content a v2"))
	idxA2 := de.FileIndex("root/a.bal")
	de.EndCompile(inst2)

	if idxA2 == idxA1 {
		t.Fatalf("changed file under a new instance should get a new index; got same %d", idxA2)
	}
	if got := de.FileName(locForIndex(idxA1)); got != "root/a.bal" {
		t.Fatalf("FileName old index: got %q", got)
	}
	if got := de.FileName(locForIndex(idxA2)); got != "root/a.bal" {
		t.Fatalf("FileName new index: got %q", got)
	}
}

// TestDiagnosticEnvStructuralSharingReuse verifies that an unchanged file
// reused by pointer across instances no-ops to the EXISTING index (symbol
// stability across compiles), while a changed file allocates a new index.
func TestDiagnosticEnvStructuralSharingReuse(t *testing.T) {
	de := NewDiagnosticEnv()

	docA := text.NewStringTextDocument("content a")
	docB := text.NewStringTextDocument("content b")

	inst1 := de.BeginCompile()
	de.RegisterFile("root/a.bal", docA)
	de.RegisterFile("root/b.bal", docB)
	idxA1 := de.FileIndex("root/a.bal")
	idxB1 := de.FileIndex("root/b.bal")
	de.EndCompile(inst1)

	// Instance 2 reuses docA by pointer (unchanged), changes b.
	inst2 := de.BeginCompile()
	de.RegisterFile("root/a.bal", docA)                                // unchanged -> reuse idxA1
	de.RegisterFile("root/b.bal", text.NewStringTextDocument("new b")) // changed -> new index
	idxA2 := de.FileIndex("root/a.bal")
	idxB2 := de.FileIndex("root/b.bal")
	de.EndCompile(inst2)

	if idxA2 != idxA1 {
		t.Fatalf("unchanged reused doc should keep index %d; got %d", idxA1, idxA2)
	}
	if idxB2 == idxB1 {
		t.Fatalf("changed file should get a new index; got same %d", idxB1)
	}
	if got := de.FileName(locForIndex(idxA1)); got != "root/a.bal" {
		t.Fatalf("FileName idxA1: got %q", got)
	}
	if got := de.FileName(locForIndex(idxB2)); got != "root/b.bal" {
		t.Fatalf("FileName idxB2: got %q", got)
	}
}

// TestDiagnosticEnvDefaultNamespacePreserved verifies that a fresh env (no
// BeginCompile) keeps the legacy same-name/different-doc panic, so standalone
// users (e.g. the TOML parser) are unaffected by the prerequisite.
func TestDiagnosticEnvDefaultNamespacePreserved(t *testing.T) {
	de := NewDiagnosticEnv()
	de.RegisterFile("x", text.NewStringTextDocument("one"))
	// same name, different doc pointer -> panic (legacy behavior preserved).
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on same-name/different-doc in default namespace")
		}
	}()
	de.RegisterFile("x", text.NewStringTextDocument("two"))
}
