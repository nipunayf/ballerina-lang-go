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

package projects_test

import (
	"path/filepath"
	"testing"

	"ballerina-lang-go/test_util"
)

// TestRecompileSameSourceRootViaModifierChain is the ticket-09 prerequisite
// bar: load a project, compile it, then update a document through the ADR-042
// modifier chain on the SAME project (which keeps the persistent shared
// CompilerEnvironment/DiagnosticEnv) and recompile. Before the prerequisite
// this panicked (same fileName re-registered under a new doc pointer on the
// shared env); now the per-compile-instance identity allocates a new file
// index and the recompile succeeds with updated diagnostics.
func TestRecompileSameSourceRootViaModifierChain(t *testing.T) {
	require := test_util.NewRequire(t)
	assert := test_util.New(t)

	absPath, err := filepath.Abs(filepath.Join("testdata", "single-file", "main.bal"))
	require.NoError(err)

	result, err := loadProject(absPath)
	require.NoError(err)

	project := result.Project()

	// First compile on the persistent env.
	firstPkg := project.CurrentPackage()
	require.NotNil(firstPkg)
	firstDiags := firstPkg.Compilation().DiagnosticResult().Diagnostics()
	firstCount := len(firstDiags)

	// Modifier-chain update on the same project: a new documentContext with a
	// new doc pointer for the same fileName, on the shared env.
	module := firstPkg.DefaultModule()
	docIDs := module.DocumentIDs()
	require.NotEmpty(docIDs)
	updatedDoc := module.Document(docIDs[0]).Modify().WithContent("import ballerina/io;\n").Apply()

	// The modifier chain set a new current package on the same project/env.
	secondPkg := project.CurrentPackage()
	require.NotNil(secondPkg)
	assert.NotSame(firstPkg, secondPkg)
	assert.Equal("import ballerina/io;\n", updatedDoc.TextDocument().String())

	// Second compile on the SAME shared env: must not panic, and must produce
	// diagnostics that differ from the first compile (proving the recompile ran
	// against the new content rather than returning cached state). The first
	// compile (full main.bal with a used import) is clean; after the modifier
	// chain replaces the content with a bare unused import, the recompile must
	// surface the resulting "unused import" diagnostic.
	secondDiags := secondPkg.Compilation().DiagnosticResult().Diagnostics()
	assert.Equal(0, firstCount)
	assert.Equal(1, len(secondDiags))
	assert.Equal("unused import prefix 'io'", secondDiags[0].Message())
}
