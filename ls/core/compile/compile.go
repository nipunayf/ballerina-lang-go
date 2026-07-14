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

// Package compile provides CompilationService, the core service that reads
// the published CurrentPackage of a resolved project (via a direct
// *workspace.ProjectService reference) and returns core-defined
// CompilerDiagnostic values with byte-offset-derived positions. The server
// converts CompilerDiagnostic to protocol.Diagnostic at the boundary,
// including UTF-16 position conversion and severity mapping.
//
// Ticket 08 retires the Phase-A private overlayFS: loading moved to the
// workspace, and Compile reads the already-published package rather than
// constructing a synthetic filesystem. Content authority moved from
// CompileRequest.Text to the published CurrentPackage(). CompilationService
// subscribes to the event bus to maintain a known-roots set so it does not
// attempt to compile for a source root that has been evicted.
package compile

import (
	"context"
	"sync"

	"ballerina-lang-go/ls/core/event"
	"ballerina-lang-go/ls/core/uri"
	"ballerina-lang-go/ls/core/workspace"
	"ballerina-lang-go/tools/diagnostics"
)

// CompileRequest carries the document URI to compile. Content authority moved
// to the published CurrentPackage; the former Text field is removed.
type CompileRequest struct {
	URI uri.DocumentURI
}

// CompileResult holds the core-defined diagnostics from a compilation.
type CompileResult struct {
	Diagnostics []CompilerDiagnostic
}

// Severity is a core-defined diagnostic severity. The compile package
// converts from diagnostics.DiagnosticSeverity to this type so that the
// server never needs to import tools/diagnostics directly.
type Severity uint8

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityInformation
	SeverityHint
)

// CompilerDiagnostic is a core-defined diagnostic with byte-offset-derived
// positions. StartLine/StartChar/EndLine/EndChar use byte character offsets
// within each line (not UTF-16 code units). The server converts these to
// protocol.Position with UTF-16 character counts at the boundary.
type CompilerDiagnostic struct {
	StartLine uint32
	StartChar uint32
	EndLine   uint32
	EndChar   uint32
	Severity  Severity
	Code      string
	Message   string
}

// CompilationService reads the published CurrentPackage of a resolved project
// and compiles it. It holds a direct *ProjectService reference (the Java
// CompilationActionImpl(projectService) shape) and subscribes to the event bus
// to maintain a known-roots set.
type CompilationService struct {
	projects   *workspace.ProjectService
	bus        *event.Bus
	mu         sync.Mutex
	knownRoots map[string]struct{}
}

// New creates a CompilationService wired to the project service (direct read
// reference) and the event bus. It subscribes to ProjectRegistered/
// ProjectEvicted/ProjectKindTransitioned to maintain the known-roots set.
func New(projects *workspace.ProjectService, bus *event.Bus) *CompilationService {
	svc := &CompilationService{
		projects:   projects,
		bus:        bus,
		knownRoots: make(map[string]struct{}),
	}
	if bus != nil {
		bus.Subscribe([]event.Kind{
			event.ProjectRegistered,
			event.ProjectEvicted,
			event.ProjectKindTransitioned,
		}, svc.handleEvent)
	}
	return svc
}

// handleEvent updates the known-roots set in response to lifecycle events.
func (s *CompilationService) handleEvent(e event.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch e.Kind() {
	case event.ProjectRegistered:
		s.knownRoots[e.SourceRoot()] = struct{}{}
	case event.ProjectEvicted:
		delete(s.knownRoots, e.SourceRoot())
	case event.ProjectKindTransitioned:
		if te, ok := e.(event.ProjectKindTransitionedEvent); ok {
			delete(s.knownRoots, te.OldRoot())
		}
		s.knownRoots[e.SourceRoot()] = struct{}{}
	}
}

// isKnown reports whether the source root has an active project.
func (s *CompilationService) isKnown(root string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.knownRoots[root]
	return ok
}

// Compile resolves the project via the direct ProjectService reference, reads
// the published CurrentPackage, and compiles it. The document text used for
// computeLineStarts is read from the published package — the same text Apply
// published — so diagnostics and positions are identical to the workspace's
// view. If the source root is unknown (evicted between Apply and Compile),
// Compile returns an empty result. context.Context is threaded for ADR-018
// cancellation ownership, even though Phase B's calls are synchronous.
func (s *CompilationService) Compile(ctx context.Context, req CompileRequest) (CompileResult, error) {
	_ = ctx
	project, err := s.projects.Project(req.URI)
	if err != nil || project == nil {
		return CompileResult{}, nil
	}
	if !s.isKnown(project.SourceRoot()) {
		return CompileResult{}, nil
	}
	pkg := project.CurrentPackage()
	if pkg == nil {
		return CompileResult{}, nil
	}
	docID, ok := project.DocumentID(req.URI.Path())
	if !ok {
		return CompileResult{}, nil
	}
	module := pkg.Module(docID.ModuleID())
	if module == nil {
		return CompileResult{}, nil
	}
	doc := module.Document(docID)
	if doc == nil {
		return CompileResult{}, nil
	}
	text := doc.TextDocument().String()
	fileName := req.URI.Path()

	compilation := pkg.Compilation()
	env := compilation.DiagnosticEnv()
	lineStarts := computeLineStarts(text)

	var diags []CompilerDiagnostic
	for _, diag := range compilation.DiagnosticResult().Diagnostics() {
		location := diag.Location()
		if !diagnostics.LocationHasSource(location) {
			continue
		}
		if env.FileName(location) != fileName {
			continue
		}
		startLine, startChar, ok := byteOffsetToLineChar(text, lineStarts, location.StartOffset())
		if !ok {
			continue
		}
		endLine, endChar, ok := byteOffsetToLineChar(text, lineStarts, location.EndOffset())
		if !ok {
			continue
		}
		info := diag.DiagnosticInfo()
		diags = append(diags, CompilerDiagnostic{
			StartLine: startLine,
			StartChar: startChar,
			EndLine:   endLine,
			EndChar:   endChar,
			Severity:  toCoreSeverity(info.Severity()),
			Code:      info.Code(),
			Message:   diag.Message(),
		})
	}
	return CompileResult{Diagnostics: diags}, nil
}

func byteOffsetToLineChar(text string, lineStarts []int, offset int) (uint32, uint32, bool) {
	if offset < 0 || offset > len(text) {
		return 0, 0, false
	}
	line := findLine(lineStarts, offset)
	lineStart := lineStarts[line]
	contentEnd := lineContentEnd(text, lineStart)
	column := offset - lineStart
	if column > contentEnd-lineStart {
		column = contentEnd - lineStart
	}
	if column < 0 {
		column = 0
	}
	return uint32(line), uint32(column), true
}

func toCoreSeverity(sev diagnostics.DiagnosticSeverity) Severity {
	switch sev {
	case diagnostics.Error, diagnostics.Fatal:
		return SeverityError
	case diagnostics.Warning:
		return SeverityWarning
	case diagnostics.Info:
		return SeverityInformation
	case diagnostics.Hint:
		return SeverityHint
	default:
		return SeverityError
	}
}

func findLine(lineStarts []int, offset int) int {
	for i := len(lineStarts) - 1; i >= 0; i-- {
		if offset >= lineStarts[i] {
			return i
		}
	}
	return 0
}

func lineContentEnd(text string, lineStart int) int {
	end := lineStart
	for end < len(text) && text[end] != '\r' && text[end] != '\n' {
		end++
	}
	return end
}

func computeLineStarts(text string) []int {
	starts := []int{0}
	for i := 0; i < len(text); {
		switch text[i] {
		case '\r':
			if i+1 < len(text) && text[i+1] == '\n' {
				starts = append(starts, i+2)
				i += 2
			} else {
				starts = append(starts, i+1)
				i++
			}
		case '\n':
			starts = append(starts, i+1)
			i++
		default:
			i++
		}
	}
	return starts
}
