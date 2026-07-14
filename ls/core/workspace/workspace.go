// Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 ( the "License"); you may not use this file except
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
// document lifecycle state (open/change/close) and the source-root-keyed
// project index. Apply carries resolved full text — the server resolves
// protocol.TextEdit ranges to full text before calling Apply, keeping this
// package protocol-free.
//
// Ticket 08 replaces the Phase-A single-file synthetic overlayFS (owned by
// compile) with a PAL-backed, overlay-augmented io/fs.FS owned here: the
// project index resolves file: URIs to source roots (ADR-048 three-step),
// projects.Load reads through palFS, and Apply publishes atomically through
// the compiler's immutable modifier chain. A synchronous event bus carries
// project-lifecycle events.
package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"time"

	"ballerina-lang-go/ls/core/event"
	"ballerina-lang-go/ls/core/uri"
	"ballerina-lang-go/platform/pal"
	"ballerina-lang-go/projects"
)

// defaultMaxProjects is the conservative Phase-B count bound for the project
// index. ADR-057's weighted 2048MB budget is deferred.
const defaultMaxProjects = 32

// Option configures a ProjectService at construction.
type Option func(*ProjectService)

// WithMaxProjects overrides the project index count bound.
func WithMaxProjects(n int) Option {
	return func(s *ProjectService) {
		if n > 0 {
			s.index.maxProjects = n
		}
	}
}

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
// No refcount or Release method ships in Phase B — there is no refcount state.
// Ticket 09 wraps Snapshot with a release func() when the dual-snapshot engine
// adds refcounting. SourceRoot is set on Apply for observability/testability.
type Snapshot struct {
	Text       string
	Version    int32
	LanguageID string
	SourceRoot string
}

// WatchedFileKind discriminates a watched-file change.
type WatchedFileKind uint8

const (
	// WatchedFileCreate is a file created on disk.
	WatchedFileCreate WatchedFileKind = iota
	// WatchedFileDelete is a file deleted from disk.
	WatchedFileDelete
	// WatchedFileModified is a file whose content changed on disk.
	WatchedFileModified
)

// WatchedFileChange describes a didChangeWatchedFiles event routed to the
// workspace.
type WatchedFileChange struct {
	Kind WatchedFileKind
	URI  uri.DocumentURI
}

// ProjectService owns document lifecycle state and the source-root-keyed
// project index. It absorbs the former server.documentStore, replacing the
// string-keyed map with a DocumentURI-keyed map, and replaces the compile-owned
// synthetic overlayFS with PAL-backed project loading and modifier-chain
// publication.
type ProjectService struct {
	platform  pal.Platform
	bus       *event.Bus
	documents map[uri.DocumentURI]Snapshot
	index     *projectIndex
	now       func() time.Time
	stat      func(string) (fs.FileInfo, error)
}

// New creates a ProjectService wired to the given PAL platform and synchronous
// event bus. The bus receives ProjectRegistered/ProjectEvicted/
// ProjectKindTransitioned events. Options configure the index bound.
func New(platform pal.Platform, bus *event.Bus, opts ...Option) *ProjectService {
	now := time.Now
	s := &ProjectService{
		platform:  platform,
		bus:       bus,
		documents: make(map[uri.DocumentURI]Snapshot),
		index:     newProjectIndex(defaultMaxProjects, now),
		now:       now,
		stat:      platform.FS.Stat,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// applyDocumentMap carries the 07 logic: version monotonicity checking and text
// accumulation in the documents map.
func (s *ProjectService) applyDocumentMap(change DocumentChange) (Snapshot, error) {
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
		if _, ok := s.documents[change.URI]; !ok {
			return Snapshot{}, fmt.Errorf("workspace: document not open: %s", change.URI)
		}
		delete(s.documents, change.URI)
		return Snapshot{}, nil
	default:
		return Snapshot{}, fmt.Errorf("workspace: unknown change kind %d", change.Kind)
	}
}

// Apply applies a document lifecycle change: update the documents map,
// resolve the source root (memoized), and — for open/update — load a fresh
// project through palFS so the open-buffer overlay is authoritative, then
// atomically replace the index entry. context.Context declares that
// cancellation flows through ctx; it is checked for Err() before publication.
// Phase B's calls are synchronous (no goroutines).
//
// The project is reloaded per publication rather than reusing a cached
// project's environment: the compiler's DiagnosticEnv is a per-Load singleton
// that panics on re-registration of a changed file (a documented latent
// limitation — see the skipped TestProjectDuplicate). The index still provides
// source-root memoization (the expensive ADR-048 walk) and count-bounding.
// The ADR-042 modifier chain (Document.Modify().WithContent().Apply() →
// setCurrentPackage) is the deferred publication model, restored by a later
// ticket that lifts DiagnosticEnv to per-instance file identity.
func (s *ProjectService) Apply(ctx context.Context, change DocumentChange) (Snapshot, error) {
	if change.Kind == ChangeClose {
		// Resolve first (the document is still open) to find the index entry,
		// then remove from the map and decrement the open-doc count. No
		// publication on close — the document stays in the package until
		// eviction or a watched-file delete.
		sourceRoot, err := s.findSourceRootMemoized(change.URI)
		if err == nil {
			if entry, ok := s.index.get(sourceRoot); ok && entry.openDocs > 0 {
				entry.openDocs--
			}
		}
		snap, err := s.applyDocumentMap(change)
		return snap, err
	}
	snap, err := s.applyDocumentMap(change)
	if err != nil {
		return snap, err
	}
	sourceRoot, err := s.findSourceRootMemoized(change.URI)
	if err != nil {
		return snap, nil // source root unresolved — document map still updated
	}
	if change.Kind == ChangeOpen {
		if entry, ok := s.index.get(sourceRoot); ok {
			entry.openDocs++
		}
	}
	snap.SourceRoot = sourceRoot
	if err := ctx.Err(); err != nil {
		return snap, fmt.Errorf("workspace: cancelled before publication: %w", err)
	}
	if err := s.publish(change, sourceRoot); err != nil {
		return snap, err
	}
	return snap, nil
}

// publish loads a fresh project through palFS and atomically replaces the
// index entry, then publishes ProjectRegistered. This is the 08 publication
// model — fresh projects.Load per publication, not cached-project modifier-
// chain reuse. The DiagnosticEnv same-name/different-TextDocument panic (see
// the decision's Phase-B correction) blocks the ADR-042 modifier chain
// (Document.Modify().WithContent().Apply() → setCurrentPackage) until a later
// ticket lifts DiagnosticEnv to per-instance file identity; until then 08 pays
// a full re-Load per content change. ProjectRegistered is published on each
// replacement (the source root stays known); ProjectEvicted is published only
// on real LRU/kind-transition eviction, never on a content republish.
func (s *ProjectService) publish(change DocumentChange, sourceRoot string) error {
	s.reloadAt(sourceRoot, change.URI)
	if s.bus != nil {
		s.bus.Publish(event.NewProjectRegisteredEvent(sourceRoot))
	}
	return nil
}

// reloadAt builds a fresh palFS for the source root, loads a brand-new project
// (fresh CompilerEnvironment and DiagnosticEnv), and atomically replaces the
// index entry. The previously indexed project is dropped; Compile resolves the
// freshly-indexed entry and reads its CurrentPackage(). The open-doc count is
// preserved from any existing entry at the same source root. A load failure is
// non-fatal: the documents map is still updated, and the next change retries.
func (s *ProjectService) reloadAt(sourceRoot string, u uri.DocumentURI) {
	fsys := s.buildPalFS(sourceRoot)
	project, err := s.loadProject(fsys, sourceRoot, u.Path())
	if err != nil || project == nil {
		return
	}
	entry := &indexEntry{
		project:     project,
		sourceRoot:  sourceRoot,
		lastTouched: s.now(),
	}
	if old, ok := s.index.get(sourceRoot); ok {
		entry.openDocs = old.openDocs
	}
	s.index.putExisting(entry, s.bus)
}

// findSourceRootMemoized is the ADR-048 step 1 for a file: URI: find the
// project source root, memoized by file path. It is file:-only in 08.
func (s *ProjectService) findSourceRootMemoized(u uri.DocumentURI) (string, error) {
	filePath := u.Path()
	if root, ok := s.index.lookupSourceRoot(filePath); ok {
		return root, nil
	}
	root, err := s.findSourceRoot(filePath)
	if err != nil {
		return "", err
	}
	s.index.memoSourceRoot(filePath, root)
	return root, nil
}

// findSourceRoot walks up from filePath looking for Ballerina.toml (build/
// workspace project); on no hit for a .bal file it returns the file's
// directory (single-file project root). It is the expensive step and is
// memoized by resolve.
func (s *ProjectService) findSourceRoot(filePath string) (string, error) {
	dir := path.Dir(filePath)
	for {
		tomlPath := path.Join(dir, projects.BallerinaTomlFile)
		if info, err := s.stat(tomlPath); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := path.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// No Ballerina.toml ancestor: single-file project rooted at the file's
	// directory (matching loadSingleFileProject's sourceDir = path.Dir).
	return path.Dir(filePath), nil
}

// buildPalFS constructs an overlay-augmented filesystem carrying the current
// open-buffer contents for files under sourceRoot. Open buffers win on
// ReadFile/Stat so a single-file non-disk URI (and overlay-over-disk edits)
// load from the buffer, not disk.
func (s *ProjectService) buildPalFS(sourceRoot string) palFS {
	overlays := make(map[string][]byte)
	for u, snap := range s.documents {
		if !u.IsFile() {
			continue
		}
		p := u.Path()
		if p == sourceRoot || isAncestor(sourceRoot, p) {
			overlays[p] = []byte(snap.Text)
		}
	}
	return palFS{pal: s.platform.FS, overlays: overlays, now: s.now}
}

// loadProject loads the project at sourceRoot through palFS. For a
// single-file source root (no Ballerina.toml), it loads the single .bal file;
// the overlay lets a non-disk file load from its buffer.
func (s *ProjectService) loadProject(fsys palFS, sourceRoot, filePath string) (projects.Project, error) {
	// A single-file root: no Ballerina.toml at sourceRoot → load the file
	// directly (palFS overlays the buffer for a non-disk URI).
	tomlPath := path.Join(sourceRoot, projects.BallerinaTomlFile)
	if info, err := fsys.Stat(tomlPath); err != nil || info.IsDir() {
		result, err := projects.Load(fsys, filePath)
		if err != nil {
			return nil, err
		}
		return result.Project(), nil
	}
	result, err := projects.Load(fsys, sourceRoot)
	if err != nil {
		return nil, err
	}
	return result.Project(), nil
}

// Project returns the resolved project for a file: URI from the index. It is
// the exported accessor CompilationService uses to read the published
// CurrentPackage via a direct reference. It does not auto-load: a source root
// that was never Apply'd (or was evicted) returns nil, so Compile returns an
// empty result.
func (s *ProjectService) Project(u uri.DocumentURI) (projects.Project, error) {
	sourceRoot, err := s.findSourceRootMemoized(u)
	if err != nil {
		return nil, err
	}
	entry, ok := s.index.get(sourceRoot)
	if !ok {
		return nil, nil
	}
	return entry.project, nil
}

// Snapshot returns the current snapshot for the given URI, replacing the
// former documentStore.document lookup.
func (s *ProjectService) Snapshot(u uri.DocumentURI) (Snapshot, bool) {
	snap, ok := s.documents[u]
	return snap, ok
}

// ApplyWatchedFile routes a didChangeWatchedFiles event. Ballerina.toml
// create/delete/change triggers an atomic ADR-024 kind transition (or reload);
// a source-file delete while not open drops the stale index entry so the next
// didOpen/didChange reloads fresh from disk (no modifier chain on the cached
// project, avoiding the DiagnosticEnv same-name/different-doc panic).
func (s *ProjectService) ApplyWatchedFile(ctx context.Context, change WatchedFileChange) error {
	_ = ctx
	if !change.URI.IsFile() {
		return nil
	}
	filePath := change.URI.Path()
	base := path.Base(filePath)
	switch base {
	case projects.BallerinaTomlFile:
		return s.applyBallerinaTomlChange(change.Kind, filePath)
	default:
		return s.applySourceFileChange(change.Kind, filePath)
	}
}

// applyBallerinaTomlChange handles create/delete/change of a Ballerina.toml:
// atomically invalidate the source-root memo, re-resolve, and reload the
// project fresh so the new kind (single-file ↔ build) or new manifest takes
// effect. Create/delete publish ProjectKindTransitioned when the source root
// identity is unchanged, or ProjectEvicted(oldRoot)+ProjectRegistered(newRoot)
// when the root identity changes (ADR-024). A content change publishes
// ProjectRegistered (a manifest reload is a replacement). With no open doc
// under dir, only the memo is invalidated; the next didOpen reloads the new kind.
func (s *ProjectService) applyBallerinaTomlChange(kind WatchedFileKind, tomlPath string) error {
	dir := path.Dir(tomlPath)
	anchor, oldRoot, found := s.findAnchorUnder(dir)
	s.index.invalidateUnder(dir)
	if !found {
		return nil
	}
	newRoot, err := s.findSourceRootMemoized(anchor)
	if err != nil {
		return nil
	}
	s.reloadAt(newRoot, anchor)
	if s.bus == nil {
		return nil
	}
	if kind == WatchedFileCreate || kind == WatchedFileDelete {
		if oldRoot == newRoot {
			s.bus.Publish(event.NewProjectKindTransitionedEvent(oldRoot, newRoot))
		} else {
			s.bus.Publish(event.NewProjectEvictedEvent(oldRoot, event.EvictionKindTransition))
			s.bus.Publish(event.NewProjectRegisteredEvent(newRoot))
		}
	} else {
		s.bus.Publish(event.NewProjectRegisteredEvent(newRoot))
	}
	return nil
}

// findAnchorUnder returns an open document under dir (or equal to dir) to
// anchor a kind-transition reload, plus the source root that doc resolved to
// before the transition. The anchor is the first matching open file: URI; the
// old root is taken from the filePath→sourceRoot memo (falling back to dir for
// a single-file doc that was never memoized).
func (s *ProjectService) findAnchorUnder(dir string) (uri.DocumentURI, string, bool) {
	for u := range s.documents {
		if !u.IsFile() {
			continue
		}
		p := u.Path()
		if p == dir || isAncestor(dir, p) {
			oldRoot, ok := s.index.lookupSourceRoot(p)
			if !ok {
				oldRoot = dir
			}
			return u, oldRoot, true
		}
	}
	return uri.DocumentURI{}, "", false
}

// applySourceFileChange handles create/change/delete of a .bal file. In 08:
// a deleted on-disk file that is not open drops the stale index entry so the
// next didOpen/didChange reloads fresh from disk (the fresh Load excludes the
// deleted file). Create/change on disk need no action — the next Load/Reload
// picks them up via ReadDir. A file that is still open keeps its overlay; the
// document stays in the package until close. This avoids the modifier chain
// (and the DiagnosticEnv same-name/different-doc panic) on the cached project.
func (s *ProjectService) applySourceFileChange(kind WatchedFileKind, filePath string) error {
	if kind != WatchedFileDelete {
		return nil
	}
	for u := range s.documents {
		if u.IsFile() && u.Path() == filePath {
			return nil // still open — overlay keeps content; no action in 08
		}
	}
	sourceRoot, ok := s.index.lookupSourceRoot(filePath)
	if !ok {
		return nil
	}
	s.index.remove(sourceRoot)
	return nil
}

// Shutdown is the lifecycle contract that ticket 09 fills with real
// cancellation. Its Phase-B body is a no-op — there is no async work to
// cancel yet.
func (s *ProjectService) Shutdown(ctx context.Context) error {
	_ = ctx
	if s.bus != nil {
		s.bus.Close()
	}
	return nil
}
