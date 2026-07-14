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

// Package compile provides CompilationService, the core service that wraps
// the compiler pipeline (projects.Load → DiagnosticResult) and returns
// core-defined CompilerDiagnostic values with byte-offset-derived positions.
// The server converts CompilerDiagnostic to protocol.Diagnostic at the
// boundary, including UTF-16 position conversion and severity mapping.
//
// The overlayFS synthetic filesystem is a private implementation detail of
// this package — it is compilation infrastructure, not server logic. Ticket 08
// replaces it with real workspace management.
package compile

import (
	"context"
	"io"
	"io/fs"
	"path"
	"time"

	"ballerina-lang-go/ls/core/uri"
	"ballerina-lang-go/platform/pal"
	"ballerina-lang-go/projects"
	"ballerina-lang-go/tools/diagnostics"
)

// CompileRequest carries the document URI and full text to compile. The
// server resolves protocol.TextEdit ranges to full Text before calling
// Compile, keeping this package protocol-free.
type CompileRequest struct {
	URI  uri.DocumentURI
	Text string
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

// CompilationService wraps the compiler pipeline. It is constructed with a
// PAL platform to avoid a future constructor change.
type CompilationService struct {
	platform pal.Platform
}

// New creates a CompilationService wired to the given PAL platform.
func New(platform pal.Platform) *CompilationService {
	return &CompilationService{platform: platform}
}

// Compile carries the exact logic from the former compileOverlayDiagnostics:
// it constructs an overlayFS from the document text, calls projects.Load,
// iterates the compilation's DiagnosticResult, and maps each
// diagnostics.Diagnostic to a CompilerDiagnostic with byte-offset-derived
// positions. context.Context is threaded for ADR-018 cancellation ownership,
// even though Phase A's calls are synchronous.
func (s *CompilationService) Compile(ctx context.Context, req CompileRequest) (CompileResult, error) {
	_ = ctx
	_ = s.platform

	fileName := path.Base(req.URI.Path())
	result, err := projects.Load(overlayFS{name: fileName, content: []byte(req.Text)}, fileName)
	if err != nil {
		return CompileResult{}, nil
	}
	compilation := result.Project().CurrentPackage().Compilation()
	env := compilation.DiagnosticEnv()
	lineStarts := computeLineStarts(req.Text)

	var diags []CompilerDiagnostic
	for _, diag := range compilation.DiagnosticResult().Diagnostics() {
		location := diag.Location()
		if !diagnostics.LocationHasSource(location) {
			continue
		}
		if env.FileName(location) != fileName {
			continue
		}
		startLine, startChar, ok := byteOffsetToLineChar(req.Text, lineStarts, location.StartOffset())
		if !ok {
			continue
		}
		endLine, endChar, ok := byteOffsetToLineChar(req.Text, lineStarts, location.EndOffset())
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

type overlayFS struct {
	name    string
	content []byte
}

func (f overlayFS) Open(name string) (fs.File, error) {
	if name != f.name {
		return nil, fs.ErrNotExist
	}
	return overlayFile{info: f.fileInfo(), reader: newBytesReader(f.content)}, nil
}

func (f overlayFS) Stat(name string) (fs.FileInfo, error) {
	if name != f.name {
		return nil, fs.ErrNotExist
	}
	return f.fileInfo(), nil
}

func (f overlayFS) ReadFile(name string) ([]byte, error) {
	if name != f.name {
		return nil, fs.ErrNotExist
	}
	return f.content, nil
}

func (f overlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		return []fs.DirEntry{overlayDirEntry{info: f.fileInfo()}}, nil
	}
	return nil, fs.ErrNotExist
}

func (f overlayFS) fileInfo() overlayFileInfo {
	return overlayFileInfo{name: f.name, size: int64(len(f.content))}
}

type overlayFileInfo struct {
	name string
	size int64
}

func (info overlayFileInfo) Name() string       { return info.name }
func (info overlayFileInfo) Size() int64        { return info.size }
func (info overlayFileInfo) Mode() fs.FileMode  { return 0o444 }
func (info overlayFileInfo) ModTime() time.Time { return time.Time{} }
func (info overlayFileInfo) IsDir() bool        { return false }
func (info overlayFileInfo) Sys() any           { return nil }

type overlayDirEntry struct {
	info overlayFileInfo
}

func (entry overlayDirEntry) Name() string               { return entry.info.name }
func (entry overlayDirEntry) IsDir() bool                { return false }
func (entry overlayDirEntry) Type() fs.FileMode          { return 0 }
func (entry overlayDirEntry) Info() (fs.FileInfo, error) { return entry.info, nil }

type overlayFile struct {
	info   overlayFileInfo
	reader *bytesReader
}

func (f overlayFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f overlayFile) Read(p []byte) (int, error) { return f.reader.read(p) }
func (f overlayFile) Close() error               { return nil }

type bytesReader struct {
	data []byte
	off  int
}

func newBytesReader(data []byte) *bytesReader { return &bytesReader{data: data} }

func (r *bytesReader) read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}
