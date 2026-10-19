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
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ballerina-nutcracker/ballerina/ls/core/compile"
	"github.com/ballerina-nutcracker/ballerina/ls/core/event"
	"github.com/ballerina-nutcracker/ballerina/ls/core/observability"
	"github.com/ballerina-nutcracker/ballerina/ls/core/workspace"
	"github.com/ballerina-nutcracker/ballerina/ls/server"
	"github.com/ballerina-nutcracker/ballerina/projects"
)

type nestedDiscoveryFailureFS struct{}

func (nestedDiscoveryFailureFS) Open(string) (fs.File, error) { return nil, fs.ErrPermission }

func TestNestedManifestWatchRetryRecoversAllDocuments(t *testing.T) {
	for _, transition := range []string{"create", "delete"} {
		t.Run(transition, func(t *testing.T) {
			content, err := fs.ReadFile(os.DirFS("sync/testdata"), "nested-manifest-"+transition+".json")
			if err != nil {
				t.Fatal(err)
			}
			var fixture transcript
			if err := json.Unmarshal(content, &fixture); err != nil {
				t.Fatal(err)
			}
			root := materializeSources(t, fixture.Sources)
			rootURI, err := workspace.FileURIFromPath(root)
			if err != nil {
				t.Fatal(err)
			}
			messages, err := fixtureMessages(fixture.Messages, "", strings.TrimPrefix(rootURI.String(), "file://"))
			if err != nil {
				t.Fatal(err)
			}
			segments := buildSegments(messages)
			if len(segments) != 3 || segments[1].diskWrite == nil {
				t.Fatal("nested-manifest fixture must contain one disk action between two framed batches")
			}
			nested := filepath.Join(root, "modules", "util")
			var failDiscovery atomic.Bool
			opener := func(dir string) fs.FS {
				if dir == nested && failDiscovery.Load() {
					return nestedDiscoveryFailureFS{}
				}
				return os.DirFS(dir)
			}
			bus := event.New()
			defer bus.Close()
			service := workspace.New(opener, bus)
			compiler := compile.New(service, bus, compile.WithDebounce(0))
			defer compiler.Shutdown()
			transport := &stepTransport{}
			var logs bytes.Buffer
			srv := server.New(transport, service, compiler, bus, server.WithLogger(observability.New(&logs)))
			defer srv.Flush()
			serve := func(input []byte) []json.RawMessage {
				t.Helper()
				transport.reader = bytes.NewReader(input)
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
			serve(segments[0].input.Bytes())
			before := make(map[workspace.DocumentURI]workspace.Snapshot)
			for _, relative := range []string{"main.bal", "modules/util/foo.bal"} {
				uri, err := workspace.FileURIFromPath(filepath.Join(root, filepath.FromSlash(relative)))
				if err != nil {
					t.Fatal(err)
				}
				snap, ok := service.Snapshot(uri)
				if !ok || snap.SourceRoot == "" || snap.Generation == 0 {
					t.Fatalf("document never published: %s, %+v", uri, snap)
				}
				before[uri] = snap
			}
			applyDiskWrite(t, root, segments[1].diskWrite)
			failDiscovery.Store(true)
			failed := serve(segments[2].input.Bytes())
			if len(failed) != 2 {
				t.Fatalf("failed watch added client notifications: %s", formatMessages(failed))
			}
			for _, reply := range failed {
				var envelope struct {
					Method string          `json:"method"`
					Error  json.RawMessage `json:"error"`
				}
				if err := json.Unmarshal(reply, &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Method != "" || len(envelope.Error) != 0 || strings.Contains(string(reply), "Buffer") {
					t.Fatalf("failed watch published a notification, error, or stale semantics: %s", reply)
				}
			}
			if !strings.Contains(logs.String(), "permission denied") {
				t.Fatalf("discovery failure was not logged: %s", logs.String())
			}
			for _, key := range []string{root, nested} {
				if project, _, ok := service.CurrentProject(key); ok || project != nil {
					t.Fatalf("stale project remains current: %s", key)
				}
				if _, _, ok := compiler.DiagnosticsFor(key); ok {
					t.Fatalf("stale diagnostics remain current: %s", key)
				}
			}
			for uri, old := range before {
				snap, ok := service.Snapshot(uri)
				if !ok || snap.Text != old.Text || snap.Version != old.Version || snap.SourceRoot != "" || snap.Generation != 0 {
					t.Fatalf("failed watch lost buffer or exposed current ownership: %s, %+v", uri, snap)
				}
				if _, ok := compiler.SealedModuleFor(context.Background(), uri); ok {
					t.Fatalf("failed watch exposed stale semantics: %s", uri)
				}
			}
			failDiscovery.Store(false)
			recovered := serve(segments[2].input.Bytes())
			for uri, old := range before {
				key, name, diagnostic := root, "main.bal", "unusedOuter"
				if uri.Path() != filepath.Join(root, "main.bal") {
					key, name, diagnostic = root, "modules/util/foo.bal", "unusedInner"
					if transition == "create" {
						key, name = nested, "foo.bal"
					}
				}
				gotKey, err := service.ProjectKey(uri)
				if err != nil || gotKey != key {
					t.Fatalf("%s: project key=%q, %v; want %q", uri, gotKey, err, key)
				}
				project, gen, ok := service.CurrentProject(key)
				if !ok || project == nil {
					t.Fatalf("%s: affected document was not reloaded by watch retry without edits", uri)
				}
				if project.Kind() != projects.ProjectKindBuild {
					t.Fatalf("%s: wrong recovered project kind: %v", uri, project.Kind())
				}
				snap, ok := service.Snapshot(uri)
				if !ok || snap.Text != old.Text || snap.Version != old.Version || snap.SourceRoot != key || snap.Generation != gen || gen <= old.Generation {
					t.Fatalf("%s: wrong recovered buffer/identity: %+v; current generation=%d", uri, snap, gen)
				}
				gotName, ok := service.Paths(project).NameOf(uri)
				if !ok || gotName != name {
					t.Fatalf("%s: recovered compiler name=%q, %v; want %q", uri, gotName, ok, name)
				}
				sm, ok := compiler.SealedModuleFor(context.Background(), uri)
				if !ok {
					t.Fatalf("%s: recovered semantic result unavailable", uri)
				}
				if text, ok := sm.SourceText(uri); !ok || text != old.Text {
					t.Fatalf("%s: recovered semantic buffer=%q, %v", uri, text, ok)
				}
				diags, diagGen, ok := compiler.DiagnosticsFor(key)
				if !ok || diagGen != gen || len(diags[uri]) != 1 || !strings.Contains(diags[uri][0].Message, diagnostic) {
					t.Fatalf("%s: wrong recovered diagnostics: %+v, %d, %v", uri, diags, diagGen, ok)
				}
			}
			published := make(map[string]bool)
			for _, message := range recovered {
				var envelope struct {
					Method string `json:"method"`
					Params struct {
						URI         string `json:"uri"`
						Diagnostics []struct {
							Message string `json:"message"`
						} `json:"diagnostics"`
					} `json:"params"`
				}
				if err := json.Unmarshal(message, &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Method == "textDocument/publishDiagnostics" && len(envelope.Params.Diagnostics) == 1 && strings.Contains(envelope.Params.Diagnostics[0].Message, "unused") {
					published[envelope.Params.URI] = true
				}
			}
			for uri := range before {
				if !published[uri.String()] {
					t.Errorf("watch retry did not republish diagnostics for %s", uri)
				}
			}
			actualReplies, wantReplies := nestedSemanticReplies(t, recovered), nestedSemanticReplies(t, fixture.Expected)
			if !reflect.DeepEqual(normalizeMessages(actualReplies), normalizeMessages(wantReplies)) {
				t.Fatalf("recovered framed signature results mismatch\nactual: %s\nexpected: %s", formatMessages(actualReplies), formatMessages(wantReplies))
			}
		})
	}
}

func nestedSemanticReplies(t *testing.T, messages []json.RawMessage) []json.RawMessage {
	t.Helper()
	var replies []json.RawMessage
	for _, message := range messages {
		var envelope struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(message, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.ID == 2 || envelope.ID == 3 {
			replies = append(replies, message)
		}
	}
	return replies
}
