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

package server

import (
	"testing"

	"ballerina-lang-go/ls/protocol"
)

func TestCompileOverlayDiagnostics(t *testing.T) {
	diagnostics := compileOverlayDiagnostics("file:///workspace/main.bal", "public function main() {\n    int x = 1;\n}\n")
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1", len(diagnostics))
	}
	diag := diagnostics[0]
	severity, _ := diag.Severity.Value()
	if severity != protocol.DiagnosticSeverityError {
		t.Errorf("severity = %v, want SeverityError", severity)
	}
	source, _ := diag.Source.Value()
	if source != "ballerina" {
		t.Errorf("source = %q, want ballerina", source)
	}
	code, _ := diag.Code.Value()
	codeStr, _ := code.String()
	if codeStr != "SEMANTIC_ERROR" {
		t.Errorf("code = %q, want SEMANTIC_ERROR", codeStr)
	}
	wantStart := protocol.Position{Line: 1, Character: 4}
	wantEnd := protocol.Position{Line: 1, Character: 14}
	if diag.Range.Start != wantStart || diag.Range.End != wantEnd {
		t.Errorf("range = %+v..%+v, want %+v..%+v", diag.Range.Start, diag.Range.End, wantStart, wantEnd)
	}

	if got := compileOverlayDiagnostics("file:///workspace/main.bal", "public function main() {}\n"); len(got) != 0 {
		t.Fatalf("valid diagnostics = %d, want 0", len(got))
	}

	if got := compileOverlayDiagnostics("untitled:Untitled-1", "public function main() {\n    int x = 1;\n}\n"); len(got) != 0 {
		t.Fatalf("non-file diagnostics = %d, want 0", len(got))
	}
}

func TestByteOffsetToPositionUTF16(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		offset   int
		wantLine uint32
		wantChar uint32
		wantOK   bool
	}{
		{name: "ascii start", text: "abc\ndef\n", offset: 0, wantLine: 0, wantChar: 0, wantOK: true},
		{name: "ascii mid line0", text: "abc\ndef\n", offset: 2, wantLine: 0, wantChar: 2, wantOK: true},
		{name: "ascii at newline belongs to line0 end", text: "abc\ndef\n", offset: 3, wantLine: 0, wantChar: 3, wantOK: true},
		{name: "ascii line1 start", text: "abc\ndef\n", offset: 4, wantLine: 1, wantChar: 0, wantOK: true},
		{name: "ascii end of text trailing line", text: "abc\ndef\n", offset: 8, wantLine: 2, wantChar: 0, wantOK: true},
		{name: "crlf single break line1 start", text: "ab\r\ncd", offset: 4, wantLine: 1, wantChar: 0, wantOK: true},
		{name: "crlf at cr belongs to line0 end", text: "ab\r\ncd", offset: 2, wantLine: 0, wantChar: 2, wantOK: true},
		{name: "cr-only break", text: "ab\rcd", offset: 3, wantLine: 1, wantChar: 0, wantOK: true},
		{name: "surrogate pair counts two units", text: "😀x", offset: 4, wantLine: 0, wantChar: 2, wantOK: true},
		{name: "offset after surrogate at ascii", text: "😀x", offset: 5, wantLine: 0, wantChar: 3, wantOK: true},
		{name: "surrogate on later line", text: "😀\n😀z", offset: 9, wantLine: 1, wantChar: 2, wantOK: true},
		{name: "negative offset invalid", text: "abc", offset: -1, wantOK: false},
		{name: "offset past end invalid", text: "abc", offset: 4, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lineStarts := computeLineStarts(tc.text)
			pos, ok := byteOffsetToPosition(tc.text, lineStarts, tc.offset)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if pos.Line != tc.wantLine {
				t.Errorf("line = %d, want %d", pos.Line, tc.wantLine)
			}
			if pos.Character != tc.wantChar {
				t.Errorf("character = %d, want %d", pos.Character, tc.wantChar)
			}
		})
	}
}

func TestDocumentStoreReportsAcceptedMutations(t *testing.T) {
	store := newDocumentStore()

	if _, ok := store.open(protocol.TextDocumentItem{
		URI: "untitled:Untitled-1", Version: 1, Text: "x",
	}); ok {
		t.Fatal("non-file open reported accepted")
	}
	doc, ok := store.open(protocol.TextDocumentItem{
		URI: "file:///workspace/main.bal", Version: 1, Text: "abc",
	})
	if !ok {
		t.Fatal("file open reported rejected")
	}
	if doc.text != "abc" || doc.version != 1 {
		t.Fatalf("opened document = %#v", doc)
	}

	if _, ok := store.change(protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			URI:     "file:///workspace/main.bal",
			Version: 1,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			protocol.NewTextDocumentContentChangeEventTextDocumentContentChangePartial(protocol.TextDocumentContentChangePartial{
				Range: protocol.Range{End: protocol.Position{Line: 0, Character: 0}},
				Text:  "z",
			}),
		},
	}); ok {
		t.Fatal("stale change reported accepted")
	}

	doc, ok = store.change(protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			URI:     "file:///workspace/main.bal",
			Version: 2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			protocol.NewTextDocumentContentChangeEventTextDocumentContentChangePartial(protocol.TextDocumentContentChangePartial{
				Range: protocol.Range{
					Start: protocol.Position{Line: 0, Character: 0},
					End:   protocol.Position{Line: 0, Character: 3},
				},
				Text: "xyz",
			}),
		},
	})
	if !ok {
		t.Fatal("fresh change reported rejected")
	}
	if doc.text != "xyz" || doc.version != 2 {
		t.Fatalf("changed document = %#v", doc)
	}

	if store.close(protocol.TextDocumentIdentifier{URI: "file:///workspace/other.bal"}) {
		t.Fatal("close of unknown document reported accepted")
	}
	if !store.close(protocol.TextDocumentIdentifier{URI: "file:///workspace/main.bal"}) {
		t.Fatal("close of open document reported rejected")
	}
	if _, ok := store.document("file:///workspace/main.bal"); ok {
		t.Fatal("closed document was retained")
	}
}
