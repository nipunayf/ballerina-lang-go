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

package corpus

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"reflect"
	"strings"
	"testing"

	"ballerina-lang-go/ls/protocol"
	"ballerina-lang-go/ls/server"
	"ballerina-lang-go/platform/pal"
	"ballerina-lang-go/platform/palnative"
)

var update = flag.Bool("update", false, "update LSP corpus golden files")

type transcript struct {
	Source   string            `json:"source"`
	Messages []json.RawMessage `json:"messages"`
	Expected []json.RawMessage `json:"expected"`
}

type memoryTransport struct {
	reader *bytes.Reader
	writer bytes.Buffer
}

func (t *memoryTransport) Read(p []byte) (int, error) {
	return t.reader.Read(p)
}

func (t *memoryTransport) Write(p []byte) (int, error) {
	return t.writer.Write(p)
}

func TestCorpus(t *testing.T) {
	platform, cleanup := palnative.NewPlatform()
	defer cleanup()
	runTranscript(t, platform.FS, "sync/testdata/incremental_edit.initialize.json")
}

func runTranscript(t *testing.T, filesystem pal.FS, path string) {
	t.Helper()
	content, err := filesystem.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture transcript
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	source, err := filesystem.ReadFile("sync/testdata/" + fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := fixtureMessages(fixture.Messages, string(source))
	if err != nil {
		t.Fatal(err)
	}
	var input bytes.Buffer
	for _, message := range messages {
		if err := protocol.WriteMessage(&input, message); err != nil {
			t.Fatal(err)
		}
	}
	transport := &memoryTransport{reader: bytes.NewReader(input.Bytes())}
	if err := server.New(transport).Serve(); err != nil {
		t.Fatal(err)
	}
	actual, err := readMessages(transport.writer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		fixture.Expected = actual
		updated, err := json.MarshalIndent(fixture, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := filesystem.WriteFile(path, append(updated, '\n')); err != nil {
			t.Fatal(err)
		}
		return
	}
	if !reflect.DeepEqual(normalizeMessages(actual), normalizeMessages(fixture.Expected)) {
		t.Fatalf("transcript output mismatch\nactual: %s\nexpected: %s", formatMessages(actual), formatMessages(fixture.Expected))
	}
}

func fixtureMessages(rawMessages []json.RawMessage, source string) ([]protocol.Message, error) {
	replacement, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	messages := make([]protocol.Message, 0, len(rawMessages))
	for _, raw := range rawMessages {
		raw = []byte(strings.ReplaceAll(string(raw), `"${source}"`, string(replacement)))
		var message protocol.Message
		if err := json.Unmarshal(raw, &message); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func readMessages(output []byte) ([]json.RawMessage, error) {
	reader := bufio.NewReader(bytes.NewReader(output))
	var messages []json.RawMessage
	for {
		message, err := protocol.ReadMessage(reader)
		if err == io.EOF {
			return messages, nil
		}
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}
		messages = append(messages, encoded)
	}
}

func normalizeMessages(messages []json.RawMessage) []any {
	normalized := make([]any, len(messages))
	for index, message := range messages {
		_ = json.Unmarshal(message, &normalized[index])
	}
	return normalized
}

func formatMessages(messages []json.RawMessage) string {
	formatted, _ := json.MarshalIndent(messages, "", "  ")
	return string(formatted)
}
