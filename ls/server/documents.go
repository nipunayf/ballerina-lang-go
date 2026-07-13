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
	"net/url"
	"unicode/utf8"

	"ballerina-lang-go/ls/protocol"
)

type documentStore struct {
	documents map[string]document
}

type document struct {
	languageID protocol.LanguageKind
	version    int32
	text       string
}

func newDocumentStore() *documentStore {
	return &documentStore{documents: make(map[string]document)}
}

func (s *documentStore) open(item protocol.TextDocumentItem) (document, bool) {
	if !isFileURI(item.URI) {
		return document{}, false
	}
	doc := document{
		languageID: item.LanguageID,
		version:    item.Version,
		text:       item.Text,
	}
	s.documents[item.URI] = doc
	return doc, true
}

func (s *documentStore) change(params protocol.DidChangeTextDocumentParams) (document, bool) {
	if !isFileURI(params.TextDocument.URI) {
		return document{}, false
	}
	current, ok := s.documents[params.TextDocument.URI]
	if !ok || params.TextDocument.Version <= current.version {
		return document{}, false
	}
	text, ok := applyChanges(current.text, params.ContentChanges)
	if !ok {
		return document{}, false
	}
	current.version = params.TextDocument.Version
	current.text = text
	s.documents[params.TextDocument.URI] = current
	return current, true
}

func (s *documentStore) close(identifier protocol.TextDocumentIdentifier) bool {
	if !isFileURI(identifier.URI) {
		return false
	}
	_, ok := s.documents[identifier.URI]
	delete(s.documents, identifier.URI)
	return ok
}

func (s *documentStore) document(uri string) (document, bool) {
	document, ok := s.documents[uri]
	return document, ok
}

func isFileURI(uri string) bool {
	parsed, err := url.Parse(uri)
	return err == nil && parsed.Scheme == "file"
}

func applyChanges(text string, changes []protocol.TextDocumentContentChangeEvent) (string, bool) {
	if len(changes) == 0 {
		return "", false
	}
	for _, change := range changes {
		partial, ok := change.TextDocumentContentChangePartial()
		if !ok {
			return "", false
		}
		start, ok := utf16Offset(text, partial.Range.Start)
		if !ok {
			return "", false
		}
		end, ok := utf16Offset(text, partial.Range.End)
		if !ok || end < start {
			return "", false
		}
		if rangeLength, ok := partial.RangeLength.Value(); ok && utf16Length(text[start:end]) != rangeLength {
			return "", false
		}
		text = text[:start] + partial.Text + text[end:]
	}
	return text, true
}

func utf16Offset(text string, position protocol.Position) (int, bool) {
	lineStart := 0
	line := uint32(0)
	for line < position.Line {
		nextLineStart, ok := nextLineStart(text, lineStart)
		if !ok {
			return 0, false
		}
		lineStart = nextLineStart
		line++
	}
	if position.Character == 0 {
		return lineStart, true
	}
	lineEnd := lineStart
	for lineEnd < len(text) && text[lineEnd] != '\n' && text[lineEnd] != '\r' {
		_, size := utf8.DecodeRuneInString(text[lineEnd:])
		lineEnd += size
	}
	character := uint32(0)
	for offset := lineStart; offset < lineEnd; {
		r, size := utf8.DecodeRuneInString(text[offset:])
		width := uint32(1)
		if r >= 0x10000 {
			width = 2
		}
		if character+width > position.Character {
			return 0, false
		}
		character += width
		offset += size
		if character == position.Character {
			return offset, true
		}
	}
	if character == position.Character {
		return lineEnd, true
	}
	return 0, false
}

func nextLineStart(text string, start int) (int, bool) {
	if start >= len(text) {
		return start, true
	}
	for offset := start; offset < len(text); {
		switch text[offset] {
		case '\n':
			return offset + 1, true
		case '\r':
			if offset+1 < len(text) && text[offset+1] == '\n' {
				return offset + 2, true
			}
			return offset + 1, true
		default:
			_, size := utf8.DecodeRuneInString(text[offset:])
			offset += size
		}
	}
	return 0, false
}

func utf16Length(text string) uint32 {
	length := uint32(0)
	for _, r := range text {
		length++
		if r >= 0x10000 {
			length++
		}
	}
	return length
}
