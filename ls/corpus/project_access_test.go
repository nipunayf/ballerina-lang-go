// Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an "AS IS"
// BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing permissions
// and limitations under the License.

package corpus

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ballerina-nutcracker/ballerina/ls/core/compile"
	"github.com/ballerina-nutcracker/ballerina/ls/core/event"
	"github.com/ballerina-nutcracker/ballerina/ls/core/workspace"
	"github.com/ballerina-nutcracker/ballerina/ls/protocol"
	"github.com/ballerina-nutcracker/ballerina/ls/server"
	"github.com/ballerina-nutcracker/ballerina/projects"
)

func TestProjectAccessHotPathDoesNotOpenDirectories(t *testing.T) {
	data, err := fs.ReadFile(os.DirFS("sync/testdata"), "access-hot-path.bal")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	callLine := uint32(strings.Count(source[:strings.Index(source, "    _ =")], "\n"))
	var calls atomic.Int64
	opener := func(dir string) fs.FS { calls.Add(1); return os.DirFS(dir) }
	bus := event.New()
	defer bus.Close()
	service := workspace.New(opener, bus)
	compiler := compile.New(service, bus, compile.WithDebounce(0))
	defer compiler.Shutdown()
	transport := &stepTransport{}
	srv := server.New(transport, service, compiler, bus)
	defer srv.Flush()
	uri, err := workspace.FileURIFromPath(filepath.Join(t.TempDir(), "main.bal"))
	if err != nil {
		t.Fatal(err)
	}
	batch := func(messages ...protocol.Message) []json.RawMessage {
		t.Helper()
		var input bytes.Buffer
		for _, message := range messages {
			if err := protocol.WriteMessage(&input, message); err != nil {
				t.Fatal(err)
			}
		}
		transport.reader = bytes.NewReader(input.Bytes())
		transport.writer.Reset()
		if err := srv.Serve(); err != nil {
			t.Fatal(err)
		}
		out, err := readMessages(transport.writer.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	batch(completionConfigMessage(1, "initialize", map[string]any{}),
		completionConfigMessage(nil, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri.String(), "languageId": "ballerina", "version": 1, "text": source}}),
		completionConfigMessage(nil, "$pal/flush", map[string]any{}))
	if calls.Load() == 0 {
		t.Fatal("opener was never used for initial load")
	}
	before := calls.Load()
	batch(completionConfigMessage(nil, "textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": uri.String(), "version": 2}, "contentChanges": []any{map[string]any{"range": map[string]any{"start": protocol.Position{}, "end": protocol.Position{}}, "text": " "}}}),
		completionConfigMessage(nil, "$pal/flush", map[string]any{}))
	if calls.Load() != before {
		t.Fatal("didChange reopened source directories")
	}
	for _, request := range []struct {
		method    string
		character uint32
	}{{"textDocument/completion", 4}, {"textDocument/signatureHelp", 18}} {
		responses := batch(completionConfigMessage(2, request.method, map[string]any{"textDocument": map[string]any{"uri": uri.String()}, "position": protocol.Position{Line: callLine, Character: request.character}}),
			completionConfigMessage(nil, "$pal/flushRequests", map[string]any{}))
		if calls.Load() != before {
			t.Fatalf("%s reopened source directories", request.method)
		}
		if len(responses) != 1 || !strings.Contains(string(responses[0]), "identify") {
			t.Fatalf("%s returned no semantic result: %s", request.method, formatMessages(responses))
		}
	}
}

func TestProjectAccessCorpus(t *testing.T) {
	t.Run("independent-single-files", func(t *testing.T) {
		runTranscriptWithCheck(t, "sync/testdata/independent-single-files.json", func(service *workspace.ProjectService, compiler *compile.CompilationService, root string) {
			first := assertProjectDocument(t, service, compiler, root, "first.bal", "first.bal", "latest", "unusedA")
			second := assertProjectDocument(t, service, compiler, root, "second.bal", "second.bal", "second", "unusedB")
			if first == second {
				t.Fatal("single-file projects share workspace identity")
			}
			for _, name := range []string{"first.bal", "second.bal"} {
				if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
					t.Fatalf("overlay document exists on disk: %s, %v", name, err)
				}
			}
		})
	})
	t.Run("sibling-prefix-packages", func(t *testing.T) {
		runTranscriptWithCheck(t, "sync/testdata/sibling-prefix-packages.json", func(service *workspace.ProjectService, compiler *compile.CompilationService, root string) {
			a := assertProjectDocument(t, service, compiler, root, "shop/main.bal", "shop", "first", "unusedA")
			b := assertProjectDocument(t, service, compiler, root, "shop2/main.bal", "shop2", "second", "unusedB")
			if a == b {
				t.Fatal("sibling packages share workspace identity")
			}
		})
	})
	t.Run("manifest-create-delete", func(t *testing.T) {
		runTranscriptWithCheck(t, "sync/testdata/manifest-create-delete.json", func(service *workspace.ProjectService, compiler *compile.CompilationService, root string) {
			assertProjectDocument(t, service, compiler, root, "main.bal", "main.bal", "buffer", "unused")
			if project, _, ok := service.CurrentProject(root); ok || project != nil {
				t.Fatal("build project remains current after manifest deletion")
			}
			u, err := workspace.FileURIFromPath(filepath.Join(root, "main.bal"))
			if err != nil {
				t.Fatal(err)
			}
			project, err := service.Project(u)
			if err != nil || project == nil || project.Kind() != projects.ProjectKindSingleFile {
				t.Fatalf("final project=%v err=%v", project, err)
			}
		})
	})
}

func TestNestedManifestCorpus(t *testing.T) {
	for _, transition := range []string{"create", "delete"} {
		t.Run(transition, func(t *testing.T) {
			runTranscriptWithCheck(t, "sync/testdata/nested-manifest-"+transition+".json", func(service *workspace.ProjectService, compiler *compile.CompilationService, root string) {
				innerRoot, innerName := root, "modules/util/foo.bal"
				if transition == "create" {
					innerRoot = filepath.Join(root, "modules", "util")
					innerName = "foo.bal"
				}
				var loaded []projects.Project
				for _, doc := range []struct{ relative, key, name, buffer, diagnostic string }{
					{"main.bal", root, "main.bal", "outerBuffer", "unusedOuter"},
					{"modules/util/foo.bal", innerRoot, innerName, "innerBuffer", "unusedInner"},
				} {
					uri, err := workspace.FileURIFromPath(filepath.Join(root, filepath.FromSlash(doc.relative)))
					if err != nil {
						t.Fatal(err)
					}
					key, err := service.ProjectKey(uri)
					if err != nil || key != doc.key {
						t.Fatalf("%s: project key=%q, %v; want %q", doc.relative, key, err, doc.key)
					}
					project, gen, ok := service.CurrentProject(key)
					if !ok || project == nil {
						t.Fatalf("%s: owning project was lost during nested manifest %s", doc.relative, transition)
					}
					if project.Kind() != projects.ProjectKindBuild {
						t.Fatalf("%s: project kind=%v", doc.relative, project.Kind())
					}
					loaded = append(loaded, project)
					snap, ok := service.Snapshot(uri)
					if !ok || snap.SourceRoot != key || snap.Generation != gen {
						t.Fatalf("%s: snapshot identity=%q/%d, current identity=%q/%d", doc.relative, snap.SourceRoot, snap.Generation, key, gen)
					}
					name, ok := service.Paths(project).NameOf(uri)
					if !ok || name != doc.name {
						t.Fatalf("%s: compiler name=%q, %v", doc.relative, name, ok)
					}
					sm, ok := compiler.SealedModuleFor(context.Background(), uri)
					if !ok {
						t.Fatalf("%s: sealed module unavailable", doc.relative)
					}
					text, ok := sm.SourceText(uri)
					if !ok || text != snap.Text || !strings.Contains(text, doc.buffer) {
						t.Fatalf("%s: buffer lost in semantic result: %q, %v", doc.relative, text, ok)
					}
					diags, diagGen, ok := compiler.DiagnosticsFor(key)
					if !ok || diagGen != gen || len(diags[uri]) != 1 || !strings.Contains(diags[uri][0].Message, doc.diagnostic) {
						t.Fatalf("%s: diagnostics=%+v, generation=%d, ok=%v", doc.relative, diags, diagGen, ok)
					}
				}
				if (loaded[0] == loaded[1]) != (transition == "delete") {
					t.Fatalf("wrong project split/merge after nested manifest %s", transition)
				}
			})
		})
	}
}

func assertProjectDocument(t *testing.T, service *workspace.ProjectService, compiler *compile.CompilationService, root, relative, keyRelative, parameter, diagnostic string) string {
	t.Helper()
	u, err := workspace.FileURIFromPath(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	key, err := service.ProjectKey(u)
	if err != nil || key != filepath.Join(root, filepath.FromSlash(keyRelative)) {
		t.Fatalf("project key=%q, %v", key, err)
	}
	project, _, ok := service.CurrentProject(key)
	if !ok || project == nil {
		t.Fatalf("no current project for %s", u)
	}
	paths := service.Paths(project)
	name, ok := paths.NameOf(u)
	if !ok {
		t.Fatalf("no compiler name for %s", u)
	}
	if name != filepath.Base(relative) {
		t.Fatalf("name=%q, want basename", name)
	}
	if back, ok := paths.URIOf(name); !ok || back != u {
		t.Fatalf("reverse=%v, %v", back, ok)
	}
	sm, ok := compiler.SealedModuleFor(context.Background(), u)
	if !ok {
		t.Fatalf("no sealed module for %s", u)
	}
	text, ok := sm.SourceText(u)
	if !ok || !strings.Contains(text, parameter) {
		t.Fatalf("sealed source=%q, %v", text, ok)
	}
	snap, ok := service.Snapshot(u)
	if !ok || snap.Text != text {
		t.Fatalf("buffer/sealed mismatch: %+v, %q", snap, text)
	}
	diags, _, ok := compiler.DiagnosticsFor(key)
	if !ok || len(diags) != 1 || len(diags[u]) != 1 || !strings.Contains(diags[u][0].Message, diagnostic) {
		t.Fatalf("diagnostics for %s: %+v, %v", key, diags, ok)
	}
	return key
}
