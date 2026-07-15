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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"ballerina-lang-go/ls/core/compile"
	"ballerina-lang-go/ls/core/event"
	"ballerina-lang-go/ls/core/uri"
	"ballerina-lang-go/ls/core/workspace"
	"ballerina-lang-go/ls/protocol"
)

const (
	publishDiagnosticsMethod = "textDocument/publishDiagnostics"
	// flushMethod is the corpus-only sentinel notification that triggers a
	// deterministic drain of the async compile engine + CE delivery so the
	// driver can read publishDiagnostics written out-of-band. It is never sent
	// by a real client.
	flushMethod = "$pal/flush"
	// cancelMethod is the standard LSP $/cancelRequest notification. Under
	// the design's branch-3 cancellation model it maps to superseding the
	// relevant source root's current generation (the running compile finishes
	// but its result is gated out → CE-E3); 09 has no cancellable per-document
	// request, so it applies to every active root (see CompilationService.Cancel).
	cancelMethod = "$/cancelRequest"
)

type Server struct {
	transport      protocol.Transport
	projects       *workspace.ProjectService
	compiler       *compile.CompilationService
	bus            *event.Bus
	versionSupport bool
	initialized    bool

	writeMu sync.Mutex // serializes framed writes (Serve + CE subscriber)

	ceDone        sync.WaitGroup // tracks in-flight CE→publish tasks (flush)
	lastPubMu     sync.Mutex
	lastPublished map[string]uint64 // per-root generation dedup for CE publishing
}

// New creates a Server with the given transport and core services. The bus is
// used to subscribe to CE-E5a/E5b for out-of-band diagnostic publication. The
// services are injected explicitly — the corpus driver wires
// workspace.New(platform) and compile.New(platform, bus).
func New(transport protocol.Transport, projects *workspace.ProjectService, compiler *compile.CompilationService, bus *event.Bus) *Server {
	s := &Server{
		transport:     transport,
		projects:      projects,
		compiler:      compiler,
		bus:           bus,
		lastPublished: make(map[string]uint64),
	}
	s.subscribeDiagnostics(bus)
	s.subscribeEvictions(bus)
	return s
}

// subscribeEvictions clears the per-root generation dedup mark when a source
// root is evicted (WM-E2), so a subsequent reload — which restarts the
// generation counter at 1 — is not suppressed behind the evicted root's stale
// high-water mark. Without this the dedup in publishRootDiagnostics could drop
// the reload's first diagnostics.
func (s *Server) subscribeEvictions(bus *event.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe([]event.Kind{event.ProjectEvicted}, func(e event.Event) {
		s.lastPubMu.Lock()
		delete(s.lastPublished, e.SourceRoot())
		s.lastPubMu.Unlock()
	})
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
	return s.write(protocol.Response{
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
	switch message.Method {
	case "initialized":
		return nil
	case flushMethod:
		return s.handleFlush()
	case cancelMethod:
		return s.handleCancelRequest(message.Params)
	case "textDocument/didOpen":
		return s.handleDidOpen(message.Params)
	case "textDocument/didChange":
		return s.handleDidChange(message.Params)
	case "textDocument/didClose":
		return s.handleDidClose(message.Params)
	case "textDocument/didSave":
		return nil
	case "workspace/didChangeWatchedFiles":
		return s.handleDidChangeWatchedFiles(message.Params)
	default:
		return nil
	}
}

// handleFlush is the corpus-only $pal/flush notification handler.
func (s *Server) handleFlush() error {
	s.Flush()
	return nil
}

// handleCancelRequest routes the standard LSP $/cancelRequest notification to
// the compile engine's Cancel, which supersedes the current generation of
// every active source root so in-flight compiles finish but their results are
// gated out (CE-E3). The CancelParams id is intentionally ignored (and not
// parsed): 09 has no cancellable per-document request to map an id to a source
// root, so the cancel applies to all active roots (per-root targeting is
// deferred to 10/11). Because the params are never decoded, a malformed
// payload cannot break the dispatch loop.
func (s *Server) handleCancelRequest(params json.RawMessage) error {
	_ = params
	if s.compiler != nil {
		s.compiler.Cancel()
	}
	return nil
}

// Flush is the deterministic drain point: it waits for the compile engine's
// in-flight cycles, the server's in-flight CE→publish tasks, and the tiered
// bus delivery, so all out-of-band publishDiagnostics have been written. It is
// the corpus driver's sync point (the $pal/flush sentinel); production code
// does not call it.
func (s *Server) Flush() {
	if s.compiler != nil {
		s.compiler.Flush()
	}
	s.ceDone.Wait()
	if s.bus != nil {
		s.bus.Flush()
	}
}

// handleDidOpen applies the open and returns nil — diagnostics arrive later via
// the CE subscriber (branch 5). No synchronous Compile or publishDiagnostics.
func (s *Server) handleDidOpen(params json.RawMessage) error {
	var didOpen protocol.DidOpenTextDocumentParams
	if json.Unmarshal(params, &didOpen) != nil {
		return nil
	}
	docURI, err := uri.NewFileURI(didOpen.TextDocument.URI)
	if err != nil {
		return nil
	}
	if _, err := s.projects.Apply(context.Background(), workspace.DocumentChange{
		Kind:       workspace.ChangeOpen,
		URI:        docURI,
		Text:       didOpen.TextDocument.Text,
		Version:    didOpen.TextDocument.Version,
		LanguageID: string(didOpen.TextDocument.LanguageID),
	}); err != nil {
		return nil
	}
	return nil
}

// handleDidChange applies the update and returns nil — diagnostics arrive later
// via the CE subscriber (branch 5).
func (s *Server) handleDidChange(params json.RawMessage) error {
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
	fullText, ok := applyChanges(current.Text, didChange.ContentChanges)
	if !ok {
		return nil
	}
	if _, err := s.projects.Apply(context.Background(), workspace.DocumentChange{
		Kind:    workspace.ChangeUpdate,
		URI:     docURI,
		Text:    fullText,
		Version: didChange.TextDocument.Version,
	}); err != nil {
		return nil
	}
	return nil
}

// handleDidClose applies the close and synchronously clears diagnostics for the
// document (LSP clients expect diagnostics cleared on close).
func (s *Server) handleDidClose(params json.RawMessage) error {
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
	note := s.publishDiagnostics(didClose.TextDocument.URI, 0, false, nil)
	if note == nil {
		return nil
	}
	return s.write(*note)
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

// write serializes a framed write: marshal, build the full framed message in a
// buffer (header+body), and Write once under writeMu so concurrent writes from
// the Serve goroutine and the CE-subscriber goroutine cannot interleave
// header/body bytes (gopls conn.writeMu precedent).
func (s *Server) write(message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if _, err := fmt.Fprintf(&buf, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	buf.Write(payload)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.transport.Write(buf.Bytes())
	return err
}

// subscribeDiagnostics wires the server's out-of-band diagnostic publication
// (branch 5). It subscribes to CE-E5a/CE-E5b (BEST_EFFORT) and, on each event,
// reads the stable snapshot via compile.DiagnosticsFor, applies the
// generation-staleness guard per document, converts to protocol.Diagnostic, and
// writes publishDiagnostics per open document via s.write. Each publish task
// increments s.ceDone so the flush sentinel can wait for it.
func (s *Server) subscribeDiagnostics(bus *event.Bus) {
	if bus == nil {
		return
	}
	bus.SubscribeWithTier([]event.Kind{
		event.ResolutionDiagnosticsReady,
		event.CompilationDiagnosticsReady,
	}, event.TierBestEffort, func(e event.Event) {
		s.publishRootDiagnostics(e.SourceRoot(), e.Generation())
	})
}

// publishRootDiagnostics publishes the current stable snapshot's diagnostics
// for every open document under root, gated by the event's generation. Under
// the no-mid-cycle-hooks model (branch 2) a clean cycle emits both CE-E5a and
// CE-E5b for the same generation, each carrying the complete post-cycle set;
// a resolution-error cycle emits CE-E5a only. The publish is therefore
// first-wins per generation: whichever of CE-E5a/CE-E5b delivers first
// publishes the complete set for the cycle and the other is a no-op. This is
// robust to a BEST_EFFORT drop of either event (the survivor still publishes)
// and to a source-root reload, which restarts the generation counter at 1 —
// an exact-equality guard (not <=) avoids suppressing the reload's diagnostics
// behind a stale high-water mark left by the previous incarnation.
func (s *Server) publishRootDiagnostics(root string, gen uint64) {
	if g, ok := s.projects.Generation(root); !ok || g != gen {
		return
	}
	s.lastPubMu.Lock()
	if gen == s.lastPublished[root] {
		s.lastPubMu.Unlock()
		return
	}
	s.lastPublished[root] = gen
	s.lastPubMu.Unlock()

	diags, snapGen, ok := s.compiler.DiagnosticsFor(root)
	if !ok || snapGen != gen {
		return
	}
	openDocs := s.projects.OpenDocumentsUnder(root)
	s.ceDone.Add(len(openDocs))
	for _, u := range openDocs {
		u := u
		go func() {
			defer s.ceDone.Done()
			if g, ok := s.projects.Generation(root); !ok || g != gen {
				return
			}
			snap, hasSnap := s.projects.Snapshot(u)
			version := int32(0)
			include := false
			var text string
			if hasSnap {
				version = snap.Version
				include = true
				text = snap.Text
			}
			note := s.publishDiagnostics(u.String(), version, include, convertDiagnostics(diags[u], text))
			if note == nil {
				return
			}
			s.write(*note)
		}()
	}
}

// handleDidChangeWatchedFiles routes workspace/didChangeWatchedFiles to the
// project service. Only file: URIs are admitted — non-file: events are ignored,
// matching the file:-only routing of 09. Watched-file republishes flow through
// ApplyWatchedFile (which bumps the generation and publishes ProjectUpdated);
// diagnostics arrive later via the CE subscriber.
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
