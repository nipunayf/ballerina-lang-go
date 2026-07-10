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

func TestDocumentStoreAppliesUTF16Changes(t *testing.T) {
	store := newDocumentStore()
	store.open(protocol.TextDocumentItem{
		URI:     "file:///workspace/main.bal",
		Version: 1,
		Text:    "😀x\n",
	})

	store.change(protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: "file:///workspace/main.bal"},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{{
			Range: &protocol.Range{
				Start: protocol.Position{Line: 0, Character: 2},
				End:   protocol.Position{Line: 0, Character: 3},
			},
			Text: "y",
		}},
	})

	document, ok := store.document("file:///workspace/main.bal")
	if !ok {
		t.Fatal("document was removed")
	}
	if document.text != "😀y\n" || document.version != 2 {
		t.Fatalf("document = %#v, want updated text and version", document)
	}
}

func TestDocumentStoreRejectsInvalidBatchAtomically(t *testing.T) {
	store := newDocumentStore()
	store.open(protocol.TextDocumentItem{
		URI:     "file:///workspace/main.bal",
		Version: 1,
		Text:    "abc",
	})
	wrongLength := uint32(2)

	store.change(protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: "file:///workspace/main.bal"},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			{
				Range: &protocol.Range{
					Start: protocol.Position{Line: 0, Character: 0},
					End:   protocol.Position{Line: 0, Character: 1},
				},
				Text: "x",
			},
			{
				Range: &protocol.Range{
					Start: protocol.Position{Line: 0, Character: 1},
					End:   protocol.Position{Line: 0, Character: 2},
				},
				RangeLength: &wrongLength,
				Text:        "y",
			},
		},
	})

	document, ok := store.document("file:///workspace/main.bal")
	if !ok {
		t.Fatal("document was removed")
	}
	if document.text != "abc" || document.version != 1 {
		t.Fatalf("document = %#v, want original text and version", document)
	}
}

func TestDocumentStoreIgnoresStaleChangesAndNonFileURIs(t *testing.T) {
	store := newDocumentStore()
	store.open(protocol.TextDocumentItem{
		URI:     "file:///workspace/main.bal",
		Version: 2,
		Text:    "current",
	})
	store.change(protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: "file:///workspace/main.bal"},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{{
			Range: &protocol.Range{End: protocol.Position{Line: 0, Character: 7}},
			Text:  "stale",
		}},
	})
	store.open(protocol.TextDocumentItem{URI: "untitled:main.bal", Version: 1, Text: "ignored"})

	document, ok := store.document("file:///workspace/main.bal")
	if !ok || document.text != "current" || document.version != 2 {
		t.Fatalf("document = %#v, want current version", document)
	}
	if _, ok := store.document("untitled:main.bal"); ok {
		t.Fatal("non-file URI was retained")
	}
}
