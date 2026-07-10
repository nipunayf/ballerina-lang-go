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

type Server struct {
	transport   protocol.Transport
	documents   *documentStore
	initialized bool
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
		s.handleNotification(message)
		return nil
	}
	if message.Method != "initialize" {
		return nil
	}
	var params protocol.InitializeParams
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return nil
	}
	s.initialized = true
	return protocol.WriteMessage(s.transport, protocol.Response{
		JSONRPC: "2.0",
		ID:      message.ID,
		Result: protocol.InitializeResult{Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.TextDocumentSyncIncremental,
				Save:      true,
			},
		}},
	})
}

func (s *Server) handleNotification(message protocol.Message) {
	if !s.initialized {
		return
	}
	switch message.Method {
	case "initialized":
		return
	case "textDocument/didOpen":
		var params protocol.DidOpenTextDocumentParams
		if json.Unmarshal(message.Params, &params) == nil {
			s.documents.open(params.TextDocument)
		}
	case "textDocument/didChange":
		var params protocol.DidChangeTextDocumentParams
		if json.Unmarshal(message.Params, &params) == nil {
			s.documents.change(params)
		}
	case "textDocument/didClose":
		var params protocol.DidCloseTextDocumentParams
		if json.Unmarshal(message.Params, &params) == nil {
			s.documents.close(params.TextDocument)
		}
	case "textDocument/didSave":
		return
	}
}
