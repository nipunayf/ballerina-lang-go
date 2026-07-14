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

package workspace

import (
	"time"

	"ballerina-lang-go/ls/core/event"
	"ballerina-lang-go/projects"
)

// indexEntry is one slot in the source-root-keyed project index.
type indexEntry struct {
	project     projects.Project
	sourceRoot  string
	lastTouched time.Time // LRU stamp, updated on every resolve/publish hit
	openDocs    int       // count of open DocumentURIs resolved to this source root
}

// projectIndex is a count-bounded, source-root-keyed LRU cache of loaded
// projects, with a filePath→sourceRoot memo for the ADR-048 root-walk. It is
// unexported and behind ProjectService.
type projectIndex struct {
	entries          map[string]*indexEntry // sourceRoot → entry
	pathToSourceRoot map[string]string      // filePath → sourceRoot (memo)
	maxProjects      int
	now              func() time.Time
}

func newProjectIndex(maxProjects int, now func() time.Time) *projectIndex {
	return &projectIndex{
		entries:          make(map[string]*indexEntry),
		pathToSourceRoot: make(map[string]string),
		maxProjects:      maxProjects,
		now:              now,
	}
}

// get returns the entry for sourceRoot, updating its LRU stamp.
func (i *projectIndex) get(sourceRoot string) (*indexEntry, bool) {
	entry, ok := i.entries[sourceRoot]
	if !ok {
		return nil, false
	}
	entry.lastTouched = i.now()
	return entry, true
}

// putExisting stores an entry, replacing any existing entry for the same
// source root. If the index is full and this is a new source root, it evicts
// the least-recently-touched background entry first. Eviction publishes
// ProjectEvicted on the bus.
func (i *projectIndex) putExisting(entry *indexEntry, bus *event.Bus) {
	if _, ok := i.entries[entry.sourceRoot]; ok {
		entry.lastTouched = i.now()
		i.entries[entry.sourceRoot] = entry
		return
	}
	if len(i.entries) >= i.maxProjects {
		i.evictLRU(bus)
	}
	i.entries[entry.sourceRoot] = entry
}

// put stores a new entry. If the index is full, it evicts the
// least-recently-touched background entry (openDocs == 0) first; only if every
// entry is active does it evict the LRU active entry. Eviction publishes
// ProjectEvicted on the bus. Returns the stored entry and the evicted root
// (empty if none).
func (i *projectIndex) put(entry *indexEntry, bus *event.Bus) (*indexEntry, string) {
	if existing, ok := i.entries[entry.sourceRoot]; ok {
		existing.project = entry.project
		existing.lastTouched = i.now()
		return existing, ""
	}
	evictedRoot := ""
	if len(i.entries) >= i.maxProjects {
		evictedRoot = i.evictLRU(bus)
	}
	i.entries[entry.sourceRoot] = entry
	return entry, evictedRoot
}

// evictLRU removes the least-recently-touched background entry; if all entries
// are active, removes the LRU active entry. Returns the evicted source root
// (empty if the index was empty).
func (i *projectIndex) evictLRU(bus *event.Bus) string {
	var victim *indexEntry
	for _, entry := range i.entries {
		if entry.openDocs == 0 {
			if victim == nil || entry.lastTouched.Before(victim.lastTouched) {
				victim = entry
			}
		}
	}
	if victim == nil {
		for _, entry := range i.entries {
			if victim == nil || entry.lastTouched.Before(victim.lastTouched) {
				victim = entry
			}
		}
	}
	if victim == nil {
		return ""
	}
	i.remove(victim.sourceRoot)
	if bus != nil {
		bus.Publish(event.NewProjectEvictedEvent(victim.sourceRoot, event.EvictionLRU))
	}
	return victim.sourceRoot
}

// remove deletes an entry and invalidates the filePath→sourceRoot memo entries
// that pointed to it.
func (i *projectIndex) remove(sourceRoot string) {
	delete(i.entries, sourceRoot)
	for filePath, root := range i.pathToSourceRoot {
		if root == sourceRoot {
			delete(i.pathToSourceRoot, filePath)
		}
	}
}

// lookupSourceRoot returns the memoized source root for filePath, if present.
func (i *projectIndex) lookupSourceRoot(filePath string) (string, bool) {
	root, ok := i.pathToSourceRoot[filePath]
	return root, ok
}

// memoSourceRoot records filePath → sourceRoot.
func (i *projectIndex) memoSourceRoot(filePath, sourceRoot string) {
	i.pathToSourceRoot[filePath] = sourceRoot
}

// invalidateUnder removes memo entries whose file path is under dir (or equals
// dir) — used when Ballerina.toml is created/deleted/changed under dir.
func (i *projectIndex) invalidateUnder(dir string) {
	for filePath := range i.pathToSourceRoot {
		if filePath == dir || isAncestor(dir, filePath) {
			delete(i.pathToSourceRoot, filePath)
		}
	}
}

// isAncestor reports whether dir is an ancestor of path (dir == path or path is
// under dir), using forward-slash semantics.
func isAncestor(dir, path string) bool {
	if dir == "" {
		return false
	}
	if path == dir {
		return true
	}
	prefix := dir
	if prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	return len(path) > len(prefix) && path[:len(prefix)] == prefix
}
