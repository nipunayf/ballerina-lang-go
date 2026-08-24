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
	"context"
	"encoding/json"

	"github.com/ballerina-nutcracker/ballerina/ls/core/query"
	"github.com/ballerina-nutcracker/ballerina/ls/core/uri"
	"github.com/ballerina-nutcracker/ballerina/ls/protocol"
)

func (s *Server) handleDocumentSymbol(ctx context.Context, params json.RawMessage) trackedResult {
	_ = ctx
	var request protocol.DocumentSymbolParams
	if json.Unmarshal(params, &request) != nil {
		return trackedResult{result: []any{}, handled: true}
	}
	documentURI, err := uri.NewFileURI(request.TextDocument.URI)
	if err != nil || s.query == nil {
		return trackedResult{result: []any{}, handled: true}
	}
	snapshot, ok := s.projects.Snapshot(documentURI)
	if !ok {
		return trackedResult{result: []any{}, handled: true}
	}
	symbols := s.query.DocumentSymbols(documentURI)
	if s.documentSymbolHierarchySupport {
		return trackedResult{result: documentSymbols(symbols, snapshot.Text, s.documentSymbolTagSupport), handled: true}
	}
	return trackedResult{result: flatDocumentSymbols(symbols, request.TextDocument.URI, snapshot.Text, s.documentSymbolTagSupport, ""), handled: true}
}

func documentSymbols(symbols []query.DocumentSymbol, text string, tagSupport bool) []protocol.DocumentSymbol {
	converted := make([]protocol.DocumentSymbol, 0, len(symbols))
	for _, symbol := range symbols {
		item := protocol.DocumentSymbol{
			Name:           symbol.Name,
			Kind:           symbol.Kind,
			Range:          toLSPRange(symbol.Range, text),
			SelectionRange: toLSPRange(symbol.Range, text),
		}
		if symbol.Deprecated && tagSupport {
			item.Tags = protocol.NewOptional([]protocol.SymbolTag{protocol.SymbolTagDeprecated})
		}
		if len(symbol.Children) > 0 {
			item.Children = protocol.NewOptional(documentSymbols(symbol.Children, text, tagSupport))
		}
		converted = append(converted, item)
	}
	return converted
}

func flatDocumentSymbols(symbols []query.DocumentSymbol, documentURI, text string, tagSupport bool, container string) []protocol.SymbolInformation {
	flattened := make([]protocol.SymbolInformation, 0, len(symbols))
	for _, symbol := range symbols {
		item := protocol.SymbolInformation{
			Name: symbol.Name,
			Kind: symbol.Kind,
			Location: protocol.Location{
				URI:   documentURI,
				Range: toLSPRange(symbol.Range, text),
			},
		}
		if container != "" {
			item.ContainerName = protocol.NewOptional(container)
		}
		if symbol.Deprecated && tagSupport {
			item.Tags = protocol.NewOptional([]protocol.SymbolTag{protocol.SymbolTagDeprecated})
		}
		flattened = append(flattened, item)
		flattened = append(flattened, flatDocumentSymbols(symbol.Children, documentURI, text, tagSupport, symbol.Name)...)
	}
	return flattened
}

func toLSPRange(value query.ByteRange, text string) protocol.Range {
	lineStarts := computeLineStarts(text)
	return protocol.Range{
		Start: lineCharToUTF16Position(text, lineStarts, value.StartLine, value.StartChar),
		End:   lineCharToUTF16Position(text, lineStarts, value.EndLine, value.EndChar),
	}
}
