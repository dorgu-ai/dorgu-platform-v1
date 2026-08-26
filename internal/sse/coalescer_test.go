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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flushLog records flushed topics safely under concurrency.
type flushLog struct {
	mu      sync.Mutex
	topics  []string
	flushed chan struct{}
}

func newFlushLog() *flushLog {
	return &flushLog{flushed: make(chan struct{}, 128)}
}

func (f *flushLog) record(topic string) {
	f.mu.Lock()
	f.topics = append(f.topics, topic)
	f.mu.Unlock()
	f.flushed <- struct{}{}
}

func (f *flushLog) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.topics))
	copy(out, f.topics)
	return out
}

func (f *flushLog) waitFor(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for range count {
		select {
		case <-f.flushed:
		case <-deadline:
			t.Fatalf("waited for %d flushes, saw %d: %v", count, len(f.seen()), f.seen())
		}
	}
}

func TestMarkFlushesTheTopic(t *testing.T) {
	log := newFlushLog()
	c := NewCoalescer(0, log.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	c.Mark("apps")
	log.waitFor(t, 1)
	assert.Equal(t, []string{"apps"}, log.seen())
}

// A rollout marks the Apps topic once per Pod transition. Without a window that
// would be one full snapshot build and encode per transition.
func TestABurstCollapsesToOneFlushPerTopic(t *testing.T) {
	log := newFlushLog()
	c := NewCoalescer(30*time.Millisecond, log.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	for range 500 {
		c.Mark("apps")
	}

	log.waitFor(t, 1)
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, []string{"apps"}, log.seen(),
		"500 marks inside one window is one flush")
}

func TestDistinctTopicsEachFlushOnce(t *testing.T) {
	log := newFlushLog()
	c := NewCoalescer(30*time.Millisecond, log.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	c.Mark("apps")
	c.Mark("incidents")
	c.Mark("apps")

	log.waitFor(t, 2)
	assert.Equal(t, []string{"apps", "incidents"}, log.seen(),
		"first-marked-first-flushed, one entry each")
}

func TestMarksAfterAFlushStartANewWindow(t *testing.T) {
	log := newFlushLog()
	c := NewCoalescer(10*time.Millisecond, log.record)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	c.Mark("apps")
	log.waitFor(t, 1)

	c.Mark("apps")
	log.waitFor(t, 1)

	assert.Equal(t, []string{"apps", "apps"}, log.seen())
}

// A shutdown must not leave a client holding state one change behind.
func TestOutstandingWorkIsFlushedOnCancel(t *testing.T) {
	log := newFlushLog()
	c := NewCoalescer(time.Hour, log.record)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(stopped)
	}()

	c.Mark("apps")
	// Give Run time to consume the wake-up and enter the long window.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	assert.Equal(t, []string{"apps"}, log.seen())
}

func TestCancelBeforeAnyMarkFlushesNothing(t *testing.T) {
	log := newFlushLog()
	c := NewCoalescer(time.Millisecond, log.record)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Run(ctx)

	assert.Empty(t, log.seen())
}

func TestMarkNeverBlocks(t *testing.T) {
	// No Run at all, so nothing is draining. Informer handlers call Mark and
	// must not become back-pressure on the watch.
	c := NewCoalescer(time.Hour, func(string) { t.Fatal("flush must not run") })

	done := make(chan struct{})
	go func() {
		for range 100_000 {
			c.Mark("apps")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Mark blocked")
	}
}

// The flush callback reads the store; holding the coalescer lock across it would
// serialise snapshot building against informers recording new changes.
func TestFlushRunsOutsideTheLock(t *testing.T) {
	var (
		c    *Coalescer
		once sync.Once
	)
	done := make(chan struct{})
	c = NewCoalescer(0, func(string) {
		// Marking from inside flush is the deadlock this pins. The recursive
		// mark also causes a second flush, hence the Once.
		c.Mark("recursive")
		once.Do(func() { close(done) })
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	c.Mark("apps")
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("flush deadlocked against Mark")
	}
}

func TestDefaultWindowIsBelowThePerceptibleThreshold(t *testing.T) {
	require.Equal(t, 250*time.Millisecond, DefaultWindow)
	assert.Less(t, DefaultWindow, 500*time.Millisecond,
		"a window a person can notice is a lag, not a budget")
}
