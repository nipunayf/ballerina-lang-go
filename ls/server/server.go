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
	"bufio"
	"encoding/json"
	"errors"
	"io"

	"ballerina-lang-go/ls/protocol"
)

const publishDiagnosticsMethod = "textDocument/publishDiagnostics"

type Server struct {
	transport      protocol.Transport
	documents      *documentStore
	versionSupport bool
	initialized    bool
}

func New(transport protocol.Transport) *Server {
	return &Server{
		transport: transport,
		documents: newDocumentStore(),
	}
}

func (s *Server) Serve() error {
	reader := bufio.NewReader(s.transport)
	for {
		message, err := protocol.ReadMessage(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.handleMessage(message); err != nil {
			return err
		}
	}
}

func (s *Server) handleMessage(message protocol.Message) error {
	if len(message.ID) == 0 {
		return s.handleNotification(message)
	}
	return s.handleRequest(message)
}

func (s *Server) handleRequest(message protocol.Message) error {
	result, ok := s.dispatchRequest(message)
	if !ok {
		return nil
	}
	return protocol.WriteMessage(s.transport, protocol.Response{
		JSONRPC: "2.0",
		ID:      message.ID,
		Result:  result,
	})
}

func (s *Server) dispatchRequest(message protocol.Message) (any, bool) {
	switch message.Method {
	case "initialize":
		return s.handleInitialize(message.Params)
	}
	return nil, false
}

func (s *Server) handleInitialize(params json.RawMessage) (any, bool) {
	var initializeParams protocol.InitializeParams
	if json.Unmarshal(params, &initializeParams) != nil {
		return nil, false
	}
	s.initialized = true
	if caps, ok := initializeParams.Capabilities.TextDocument.Value(); ok {
		if diagCaps, ok := caps.PublishDiagnostics.Value(); ok {
			if versionSupport, ok := diagCaps.VersionSupport.Value(); ok {
				s.versionSupport = versionSupport
			}
		}
	}
	opts := protocol.TextDocumentSyncOptions{
		OpenClose: protocol.NewOptional(true),
		Change:    protocol.NewOptional(protocol.TextDocumentSyncKindIncremental),
		Save:      protocol.NewOptional(protocol.NewOrTextDocumentSyncOptionsSaveBoolean(true)),
	}
	return protocol.InitializeResult{Capabilities: protocol.ServerCapabilities{
		TextDocumentSync: protocol.NewOptional(protocol.NewOrServerCapabilitiesTextDocumentSyncTextDocumentSyncOptions(opts)),
	}}, true
}

func (s *Server) handleNotification(message protocol.Message) error {
	if !s.initialized {
		return nil
	}
	var notification *protocol.Notification
	switch message.Method {
	case "initialized":
		return nil
	case "textDocument/didOpen":
		notification = s.handleDidOpen(message.Params)
	case "textDocument/didChange":
		notification = s.handleDidChange(message.Params)
	case "textDocument/didClose":
		notification = s.handleDidClose(message.Params)
	case "textDocument/didSave":
		return nil
	default:
		return nil
	}
	if notification == nil {
		return nil
	}
	return protocol.WriteMessage(s.transport, *notification)
}

func (s *Server) handleDidOpen(params json.RawMessage) *protocol.Notification {
	var didOpen protocol.DidOpenTextDocumentParams
	if json.Unmarshal(params, &didOpen) != nil {
		return nil
	}
	document, ok := s.documents.open(didOpen.TextDocument)
	if !ok {
		return nil
	}
	return s.publishDiagnostics(didOpen.TextDocument.URI, document.version, true, compileOverlayDiagnostics(didOpen.TextDocument.URI, document.text))
}

func (s *Server) handleDidChange(params json.RawMessage) *protocol.Notification {
	var didChange protocol.DidChangeTextDocumentParams
	if json.Unmarshal(params, &didChange) != nil {
		return nil
	}
	document, ok := s.documents.change(didChange)
	if !ok {
		return nil
	}
	return s.publishDiagnostics(didChange.TextDocument.URI, document.version, true, compileOverlayDiagnostics(didChange.TextDocument.URI, document.text))
}

func (s *Server) handleDidClose(params json.RawMessage) *protocol.Notification {
	var didClose protocol.DidCloseTextDocumentParams
	if json.Unmarshal(params, &didClose) != nil {
		return nil
	}
	if !s.documents.close(didClose.TextDocument) {
		return nil
	}
	return s.publishDiagnostics(didClose.TextDocument.URI, 0, false, nil)
}

func (s *Server) publishDiagnostics(uri string, version int32, includeVersion bool, diagnostics []protocol.Diagnostic) *protocol.Notification {
	params := protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	}
	if includeVersion && s.versionSupport {
		params.Version = protocol.NewOptional(version)
	}
	return &protocol.Notification{
		JSONRPC: "2.0",
		Method:  publishDiagnosticsMethod,
		Params:  params,
	}
}
