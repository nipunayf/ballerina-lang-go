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
	"context"
	"encoding/json"
	"errors"
	"io"

	"ballerina-lang-go/ls/core/compile"
	"ballerina-lang-go/ls/core/uri"
	"ballerina-lang-go/ls/core/workspace"
	"ballerina-lang-go/ls/protocol"
)

const publishDiagnosticsMethod = "textDocument/publishDiagnostics"

type Server struct {
	transport      protocol.Transport
	projects       *workspace.ProjectService
	compiler       *compile.CompilationService
	versionSupport bool
	initialized    bool
}

// New creates a Server with the given transport and core services. The
// services are injected explicitly — the corpus driver wires
// workspace.New(platform) and compile.New(platform).
func New(transport protocol.Transport, projects *workspace.ProjectService, compiler *compile.CompilationService) *Server {
	return &Server{
		transport: transport,
		projects:  projects,
		compiler:  compiler,
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
	case "workspace/didChangeWatchedFiles":
		return s.handleDidChangeWatchedFiles(message.Params)
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
	docURI, err := uri.NewFileURI(didOpen.TextDocument.URI)
	if err != nil {
		return nil
	}
	snapshot, err := s.projects.Apply(context.Background(), workspace.DocumentChange{
		Kind:       workspace.ChangeOpen,
		URI:        docURI,
		Text:       didOpen.TextDocument.Text,
		Version:    didOpen.TextDocument.Version,
		LanguageID: string(didOpen.TextDocument.LanguageID),
	})
	if err != nil {
		return nil
	}
	result, err := s.compiler.Compile(context.Background(), compile.CompileRequest{
		URI: docURI,
	})
	if err != nil {
		return nil
	}
	return s.publishDiagnostics(didOpen.TextDocument.URI, snapshot.Version, true, convertDiagnostics(result.Diagnostics, snapshot.Text))
}

func (s *Server) handleDidChange(params json.RawMessage) *protocol.Notification {
	var didChange protocol.DidChangeTextDocumentParams
	if json.Unmarshal(params, &didChange) != nil {
		return nil
	}
	docURI, err := uri.NewFileURI(didChange.TextDocument.URI)
	if err != nil {
		return nil
	}
	current, ok := s.projects.Snapshot(docURI)
	if !ok {
		return nil
	}
	// UTF-16 boundary: resolve protocol.TextEdit ranges to full text here,
	// before calling workspace.Apply with resolved Text.
	fullText, ok := applyChanges(current.Text, didChange.ContentChanges)
	if !ok {
		return nil
	}
	snapshot, err := s.projects.Apply(context.Background(), workspace.DocumentChange{
		Kind:    workspace.ChangeUpdate,
		URI:     docURI,
		Text:    fullText,
		Version: didChange.TextDocument.Version,
	})
	if err != nil {
		return nil
	}
	result, err := s.compiler.Compile(context.Background(), compile.CompileRequest{
		URI: docURI,
	})
	if err != nil {
		return nil
	}
	return s.publishDiagnostics(didChange.TextDocument.URI, snapshot.Version, true, convertDiagnostics(result.Diagnostics, snapshot.Text))
}

func (s *Server) handleDidClose(params json.RawMessage) *protocol.Notification {
	var didClose protocol.DidCloseTextDocumentParams
	if json.Unmarshal(params, &didClose) != nil {
		return nil
	}
	docURI, err := uri.NewFileURI(didClose.TextDocument.URI)
	if err != nil {
		return nil
	}
	if _, err := s.projects.Apply(context.Background(), workspace.DocumentChange{
		Kind: workspace.ChangeClose,
		URI:  docURI,
	}); err != nil {
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

// handleDidChangeWatchedFiles routes workspace/didChangeWatchedFiles to the
// project service. Only file: URIs are admitted — non-file: events are ignored,
// matching the file:-only routing of 08. Watched-file notifications produce no
// direct publishDiagnostics in 08; diagnostics flow on the next didOpen/didChange.
func (s *Server) handleDidChangeWatchedFiles(params json.RawMessage) error {
	var didChange protocol.DidChangeWatchedFilesParams
	if json.Unmarshal(params, &didChange) != nil {
		return nil
	}
	for _, fileEvent := range didChange.Changes {
		docURI, err := uri.NewFileURI(fileEvent.URI)
		if err != nil {
			continue
		}
		change := workspace.WatchedFileChange{URI: docURI}
		switch fileEvent.Type {
		case protocol.FileChangeTypeCreated:
			change.Kind = workspace.WatchedFileCreate
		case protocol.FileChangeTypeDeleted:
			change.Kind = workspace.WatchedFileDelete
		case protocol.FileChangeTypeChanged:
			change.Kind = workspace.WatchedFileModified
		default:
			continue
		}
		if err := s.projects.ApplyWatchedFile(context.Background(), change); err != nil {
			continue
		}
	}
	return nil
}
