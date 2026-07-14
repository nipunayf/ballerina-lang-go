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

// Package workspace provides ProjectService, the core service that owns
// document lifecycle state (open/change/close). It absorbs the former
// server.documentStore with a DocumentURI-keyed map. DocumentChange carries
// resolved full text — the server resolves protocol.TextEdit ranges to full
// text before calling Apply, keeping this package protocol-free.
package workspace

import (
	"context"
	"fmt"

	"ballerina-lang-go/ls/core/uri"
	"ballerina-lang-go/platform/pal"
)

// ChangeKind discriminates the kind of DocumentChange.
type ChangeKind uint8

const (
	// ChangeOpen opens a new document, storing its full text and version.
	ChangeOpen ChangeKind = iota
	// ChangeUpdate replaces the text of an existing document with a new
	// resolved full text, subject to version monotonicity.
	ChangeUpdate
	// ChangeClose removes a document from the store.
	ChangeClose
)

// DocumentChange describes a single document lifecycle mutation. The server
// resolves protocol.TextEdit ranges to full Text before constructing a
// DocumentChange, so this type carries no protocol types.
type DocumentChange struct {
	Kind       ChangeKind
	URI        uri.DocumentURI
	Text       string // valid for ChangeOpen and ChangeUpdate
	Version    int32  // valid for ChangeOpen and ChangeUpdate
	LanguageID string // valid for ChangeOpen
}

// Snapshot is a plain-struct, value-type view of a document's current state.
// No refcount or Release method ships in Phase A — there is no refcount state.
// Ticket 09 wraps Snapshot with a release func() when the dual-snapshot engine
// adds refcounting.
type Snapshot struct {
	Text       string
	Version    int32
	LanguageID string
}

// ProjectService owns document lifecycle state. It absorbs the former
// server.documentStore, replacing the string-keyed map with a
// DocumentURI-keyed map.
type ProjectService struct {
	platform  pal.Platform
	documents map[uri.DocumentURI]Snapshot
}

// New creates a ProjectService wired to the given PAL platform. The platform
// is injected from the start to avoid a future constructor change.
func New(platform pal.Platform) *ProjectService {
	return &ProjectService{
		platform:  platform,
		documents: make(map[uri.DocumentURI]Snapshot),
	}
}

// Apply carries the exact logic from the former documentStore.open/change/close:
// version monotonicity checking and text accumulation. context.Context declares
// that cancellation flows through ctx, even though Phase A's calls are
// synchronous.
func (s *ProjectService) Apply(ctx context.Context, change DocumentChange) (Snapshot, error) {
	_ = ctx
	switch change.Kind {
	case ChangeOpen:
		snap := Snapshot{
			Text:       change.Text,
			Version:    change.Version,
			LanguageID: change.LanguageID,
		}
		s.documents[change.URI] = snap
		return snap, nil
	case ChangeUpdate:
		current, ok := s.documents[change.URI]
		if !ok {
			return Snapshot{}, fmt.Errorf("workspace: document not open: %s", change.URI)
		}
		if change.Version <= current.Version {
			return Snapshot{}, fmt.Errorf("workspace: stale version %d <= current %d", change.Version, current.Version)
		}
		snap := Snapshot{
			Text:       change.Text,
			Version:    change.Version,
			LanguageID: current.LanguageID,
		}
		s.documents[change.URI] = snap
		return snap, nil
	case ChangeClose:
		_, ok := s.documents[change.URI]
		delete(s.documents, change.URI)
		if !ok {
			return Snapshot{}, fmt.Errorf("workspace: document not open: %s", change.URI)
		}
		return Snapshot{}, nil
	default:
		return Snapshot{}, fmt.Errorf("workspace: unknown change kind %d", change.Kind)
	}
}

// Snapshot returns the current snapshot for the given URI, replacing the
// former documentStore.document lookup.
func (s *ProjectService) Snapshot(uri uri.DocumentURI) (Snapshot, bool) {
	snap, ok := s.documents[uri]
	return snap, ok
}

// Shutdown is the lifecycle contract that ticket 09 fills with real
// cancellation. Its Phase-A body is a no-op — there is no async work to
// cancel yet.
func (s *ProjectService) Shutdown(ctx context.Context) error {
	_ = ctx
	return nil
}
