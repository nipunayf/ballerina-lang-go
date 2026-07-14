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

// Package event provides the synchronous core event bus carrying
// project-lifecycle events for the language server core. The bus is a leaf
// package (stdlib-only) with inline dispatch: Publish calls each matching
// subscriber's handler synchronously on the calling goroutine, in
// registration order, before returning. No goroutines, no queues, no tiers.
//
// Ticket 08 ships the bus infrastructure and three workspace-manager (WM)
// lifecycle event types: ProjectRegistered, ProjectEvicted, and
// ProjectKindTransitioned. Ticket 09 extends the same bus with ProjectUpdated
// (WM-E4), compilation-engine (CE) lifecycle events, and async delivery.
package event

import (
	"log"
	"sync"
)

// Kind discriminates event types on the bus. Subscribers filter by kind.
type Kind uint8

const (
	// ProjectRegistered is published when a project is first loaded into the
	// index (WM-E1).
	ProjectRegistered Kind = iota
	// ProjectEvicted is published when a project is removed from the index,
	// either by LRU eviction or a kind transition (WM-E2).
	ProjectEvicted
	// ProjectKindTransitioned is published when a project's kind changes
	// atomically (e.g. SingleFile→Build on Ballerina.toml creation) (WM-E3).
	ProjectKindTransitioned
	// ProjectUpdated and CE lifecycle events are added in ticket 09.
)

// Event is the interface every bus event implements. SourceRoot is the
// core-internal source-root key (ADR-053), never a DocumentURI.
type Event interface {
	Kind() Kind
	SourceRoot() string
}

// EvictionReason describes why a project was evicted.
type EvictionReason uint8

const (
	// EvictionLRU is the count-bounded background-preferred eviction.
	EvictionLRU EvictionReason = iota
	// EvictionKindTransition is eviction as part of an atomic kind transition.
	EvictionKindTransition
)

// ProjectRegisteredEvent is published when a project enters the index.
type ProjectRegisteredEvent struct {
	root string
}

// NewProjectRegisteredEvent constructs a ProjectRegisteredEvent.
func NewProjectRegisteredEvent(sourceRoot string) ProjectRegisteredEvent {
	return ProjectRegisteredEvent{root: sourceRoot}
}

func (e ProjectRegisteredEvent) Kind() Kind         { return ProjectRegistered }
func (e ProjectRegisteredEvent) SourceRoot() string { return e.root }

// ProjectEvictedEvent is published when a project leaves the index.
type ProjectEvictedEvent struct {
	root   string
	reason EvictionReason
}

// NewProjectEvictedEvent constructs a ProjectEvictedEvent.
func NewProjectEvictedEvent(sourceRoot string, reason EvictionReason) ProjectEvictedEvent {
	return ProjectEvictedEvent{root: sourceRoot, reason: reason}
}

// Reason returns why the project was evicted.
func (e ProjectEvictedEvent) Reason() EvictionReason { return e.reason }

func (e ProjectEvictedEvent) Kind() Kind         { return ProjectEvicted }
func (e ProjectEvictedEvent) SourceRoot() string { return e.root }

// ProjectKindTransitionedEvent is published when a project's kind changes
// atomically (e.g. SingleFile→Build). The old and new source roots may differ
// when the project root identity changes.
type ProjectKindTransitionedEvent struct {
	oldRoot string
	newRoot string
}

// NewProjectKindTransitionedEvent constructs a ProjectKindTransitionedEvent.
func NewProjectKindTransitionedEvent(oldRoot, newRoot string) ProjectKindTransitionedEvent {
	return ProjectKindTransitionedEvent{oldRoot: oldRoot, newRoot: newRoot}
}

// OldRoot returns the source root before the transition.
func (e ProjectKindTransitionedEvent) OldRoot() string { return e.oldRoot }

// NewRoot returns the source root after the transition.
func (e ProjectKindTransitionedEvent) NewRoot() string { return e.newRoot }

func (e ProjectKindTransitionedEvent) Kind() Kind         { return ProjectKindTransitioned }
func (e ProjectKindTransitionedEvent) SourceRoot() string { return e.newRoot }

// Bus is the synchronous event bus. It dispatches events inline to matching
// subscribers in registration order. A panicking handler is recovered and
// logged; Publish continues to the next subscriber and does not return an
// error — a publisher must not be blocked by a subscriber failure.
type Bus struct {
	mu          sync.Mutex
	closed      bool
	subscribers []subscriber
}

type subscriber struct {
	kinds   []Kind
	handler func(Event)
}

// New creates an empty event bus.
func New() *Bus {
	return &Bus{}
}

// Subscribe registers handler for the given kinds. Kinds filters by
// membership; an empty slice subscribes to all kinds. Subscribers are called
// in registration order. Subscribe after Close is a no-op.
func (b *Bus) Subscribe(kinds []Kind, handler func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.subscribers = append(b.subscribers, subscriber{kinds: kinds, handler: handler})
}

// Publish dispatches e synchronously to every subscriber whose kind filter
// matches, in registration order. Subscriber panics are recovered and logged;
// delivery to subsequent subscribers continues. Publish after Close is a
// no-op.
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	subs := make([]subscriber, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.Unlock()

	for _, sub := range subs {
		if !matches(sub.kinds, e.Kind()) {
			continue
		}
		deliver(sub.handler, e)
	}
}

// Close stops accepting new subscribers and drops further publishes. It is
// idempotent.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
}

func matches(kinds []Kind, k Kind) bool {
	if len(kinds) == 0 {
		return true
	}
	for _, want := range kinds {
		if want == k {
			return true
		}
	}
	return false
}

func deliver(handler func(Event), e Event) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("event: subscriber panic for %T: %v", e, r)
		}
	}()
	handler(e)
}
