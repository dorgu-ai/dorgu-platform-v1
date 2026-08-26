/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sse

import (
	"context"
	"sync"
	"time"
)

// DefaultWindow is how long changes are gathered before a snapshot is built.
//
// It is a latency budget, not a throttle. A rollout marks the Apps topic dirty
// once per Pod transition, which on a fifty-pod Deployment is a few hundred
// notifications in a second or two; without a window that would be a few hundred
// full snapshot builds and encodes. A quarter second is below the threshold
// where a person reads the UI as lagging, and it turns that burst into a handful
// of pushes.
const DefaultWindow = 250 * time.Millisecond

// Coalescer collapses a burst of change notifications into at most one flush per
// topic per window.
//
// Marking is cheap and never blocks, which matters because the callers are
// informer event handlers: a slow flush must not become back-pressure on the
// watch.
type Coalescer struct {
	window time.Duration
	flush  func(topic string)

	mu    sync.Mutex
	dirty map[string]bool
	order []string

	wake chan struct{}
}

// NewCoalescer returns a Coalescer that calls flush for each dirty topic. A
// window of zero flushes on the next loop iteration with no delay, which is what
// tests want.
func NewCoalescer(window time.Duration, flush func(topic string)) *Coalescer {
	return &Coalescer{
		window: window,
		flush:  flush,
		dirty:  map[string]bool{},
		wake:   make(chan struct{}, 1),
	}
}

// Mark records that a topic changed. It never blocks.
func (c *Coalescer) Mark(topic string) {
	c.mu.Lock()
	if !c.dirty[topic] {
		c.dirty[topic] = true
		c.order = append(c.order, topic)
	}
	c.mu.Unlock()

	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Run flushes dirty topics until ctx is cancelled. It flushes whatever is
// outstanding before returning, so a shutdown does not leave a client holding
// state one change behind.
func (c *Coalescer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.drain()
			return
		case <-c.wake:
		}

		if c.window > 0 {
			timer := time.NewTimer(c.window)
			select {
			case <-ctx.Done():
				timer.Stop()
				c.drain()
				return
			case <-timer.C:
			}
		}
		c.drain()
	}
}

// drain flushes and clears the dirty set. flush is called outside the lock:
// building a snapshot reads the store, and holding this lock across that would
// serialise it against the informers marking new changes.
func (c *Coalescer) drain() {
	c.mu.Lock()
	if len(c.order) == 0 {
		c.mu.Unlock()
		return
	}
	topics := c.order
	c.order = nil
	c.dirty = map[string]bool{}
	c.mu.Unlock()

	for _, topic := range topics {
		c.flush(topic)
	}
}
