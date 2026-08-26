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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscribeAndPublish(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	defer sub.Close()

	assert.Equal(t, 1, b.Subscribers())

	b.Publish("apps", []byte(`{"apps":[]}`))

	select {
	case <-sub.Signal():
	case <-time.After(time.Second):
		t.Fatal("no signal after publish")
	}

	events := sub.Drain()
	require.Len(t, events, 1)
	assert.Equal(t, "apps", events[0].Topic)
	assert.JSONEq(t, `{"apps":[]}`, string(events[0].Data))
	assert.Equal(t, uint64(1), events[0].Seq)
}

func TestPublishReachesEverySubscriber(t *testing.T) {
	b := NewBroker()
	first, second := b.Subscribe(), b.Subscribe()
	defer first.Close()
	defer second.Close()

	b.Publish("apps", []byte("x"))

	assert.Len(t, first.Drain(), 1)
	assert.Len(t, second.Drain(), 1)
}

// This is the property that makes a slow client harmless. Every event is a
// complete snapshot of one view, so the older one is worthless and keeping it
// would only grow memory.
func TestOnlyTheNewestEventPerTopicSurvives(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	defer sub.Close()

	b.Publish("apps", []byte("stale"))
	b.Publish("apps", []byte("staler-still"))
	b.Publish("apps", []byte("current"))

	events := sub.Drain()
	require.Len(t, events, 1, "three publishes to one topic collapse to one delivery")
	assert.Equal(t, "current", string(events[0].Data))
	assert.Equal(t, uint64(3), events[0].Seq, "the sequence keeps counting")
}

// A topic that keeps changing must not starve one that changed earlier.
func TestDrainPreservesFirstPendingFirstOrder(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	defer sub.Close()

	b.Publish("meta", []byte("m1"))
	b.Publish("apps", []byte("a1"))
	b.Publish("meta", []byte("m2"))
	b.Publish("incidents", []byte("i1"))

	events := sub.Drain()
	require.Len(t, events, 3)
	assert.Equal(t, "meta", events[0].Topic)
	assert.Equal(t, "m2", string(events[0].Data), "the newest value under the earliest slot")
	assert.Equal(t, "apps", events[1].Topic)
	assert.Equal(t, "incidents", events[2].Topic)
}

func TestDrainOnEmptyMailboxIsNil(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	defer sub.Close()

	assert.Nil(t, sub.Drain(), "a spurious wake-up must be harmless")
}

func TestDrainEmptiesTheMailbox(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	defer sub.Close()

	b.Publish("apps", []byte("x"))
	require.Len(t, sub.Drain(), 1)
	assert.Nil(t, sub.Drain())
}

func TestSignalIsCoalescedToOneWakeUp(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	defer sub.Close()

	for range 50 {
		b.Publish("apps", []byte("x"))
	}

	// One wake-up is pending; the handler drains everything when it acts on it.
	select {
	case <-sub.Signal():
	default:
		t.Fatal("expected a pending wake-up")
	}
	select {
	case <-sub.Signal():
		t.Fatal("wake-ups must coalesce, not queue")
	default:
	}
	assert.Len(t, sub.Drain(), 1)
}

func TestPublishNeverBlocksOnAClientThatNeverReads(t *testing.T) {
	b := NewBroker()
	silent := b.Subscribe()
	defer silent.Close()

	done := make(chan struct{})
	go func() {
		for n := range 10_000 {
			b.Publish("apps", []byte{byte(n)})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("publish blocked on a client that never drained")
	}
}

func TestCloseRemovesTheSubscriberAndIsIdempotent(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	require.Equal(t, 1, b.Subscribers())

	sub.Close()
	assert.Equal(t, 0, b.Subscribers())
	assert.NotPanics(t, sub.Close)
	assert.Equal(t, 0, b.Subscribers())
}

func TestPublishToAClosedSubscriberIsDropped(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	sub.Close()

	b.Publish("apps", []byte("x"))
	assert.Nil(t, sub.Drain())
}

func TestPublishWithNoSubscribersIsFine(t *testing.T) {
	b := NewBroker()
	assert.NotPanics(t, func() { b.Publish("apps", []byte("x")) })
	assert.Equal(t, 0, b.Subscribers())
}

func TestConcurrentSubscribePublishAndClose(t *testing.T) {
	b := NewBroker()

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub := b.Subscribe()
			for range 50 {
				b.Publish("apps", []byte("x"))
				sub.Drain()
			}
			sub.Close()
		}()
	}
	wg.Wait()

	assert.Equal(t, 0, b.Subscribers())
}

func TestSequenceNumbersAreMonotonic(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	defer sub.Close()

	var last uint64
	for range 5 {
		b.Publish("apps", []byte("x"))
		events := sub.Drain()
		require.Len(t, events, 1)
		assert.Greater(t, events[0].Seq, last)
		last = events[0].Seq
	}
}
