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
	"io"
	"io/fs"
	"net/url"
	"path"
	"time"

	"ballerina-lang-go/ls/protocol"
	"ballerina-lang-go/projects"
	"ballerina-lang-go/tools/diagnostics"
)

const diagnosticSource = "ballerina"

func compileOverlayDiagnostics(uri string, text string) []protocol.Diagnostic {
	fileName, ok := overlayFileName(uri)
	if !ok {
		return nil
	}
	result, err := projects.Load(overlayFS{name: fileName, content: []byte(text)}, fileName)
	if err != nil {
		return nil
	}
	compilation := result.Project().CurrentPackage().Compilation()
	env := compilation.DiagnosticEnv()
	lineStarts := computeLineStarts(text)

	var converted []protocol.Diagnostic
	for _, diag := range compilation.DiagnosticResult().Diagnostics() {
		location := diag.Location()
		if !diagnostics.LocationHasSource(location) {
			continue
		}
		if env.FileName(location) != fileName {
			continue
		}
		start, ok := byteOffsetToPosition(text, lineStarts, location.StartOffset())
		if !ok {
			continue
		}
		end, ok := byteOffsetToPosition(text, lineStarts, location.EndOffset())
		if !ok {
			continue
		}
		converted = append(converted, protocol.Diagnostic{
			Range:    protocol.Range{Start: start, End: end},
			Severity: protocol.NewOptional(toLSPSeverity(diag.DiagnosticInfo().Severity())),
			Code:     protocol.NewOptional(protocol.NewOrDiagnosticCodeString(diag.DiagnosticInfo().Code())),
			Source:   protocol.NewOptional(diagnosticSource),
			Message:  protocol.NewOrDiagnosticMessageString(diag.Message()),
		})
	}
	return converted
}

func toLSPSeverity(severity diagnostics.DiagnosticSeverity) protocol.DiagnosticSeverity {
	switch severity {
	case diagnostics.Error, diagnostics.Fatal:
		return protocol.DiagnosticSeverityError
	case diagnostics.Warning:
		return protocol.DiagnosticSeverityWarning
	case diagnostics.Info:
		return protocol.DiagnosticSeverityInformation
	case diagnostics.Hint:
		return protocol.DiagnosticSeverityHint
	default:
		return protocol.DiagnosticSeverityError
	}
}

func overlayFileName(uri string) (string, bool) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return "", false
	}
	return path.Base(parsed.Path), true
}

func byteOffsetToPosition(text string, lineStarts []int, offset int) (protocol.Position, bool) {
	if offset < 0 || offset > len(text) {
		return protocol.Position{}, false
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
	return protocol.Position{
		Line:      uint32(line),
		Character: utf16CodeUnits(text[lineStart : lineStart+column]),
	}, true
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

func utf16CodeUnits(s string) uint32 {
	var count uint32
	for _, r := range s {
		count++
		if r >= 0x10000 {
			count++
		}
	}
	return count
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

func (info overlayFileInfo) Name() string      { return info.name }
func (info overlayFileInfo) Size() int64       { return info.size }
func (info overlayFileInfo) Mode() fs.FileMode { return 0o444 }
func (info overlayFileInfo) ModTime() time.Time { return time.Time{} }
func (info overlayFileInfo) IsDir() bool        { return false }
func (info overlayFileInfo) Sys() any          { return nil }

type overlayDirEntry struct {
	info overlayFileInfo
}

func (entry overlayDirEntry) Name() string               { return entry.info.name }
func (entry overlayDirEntry) IsDir() bool                 { return false }
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
