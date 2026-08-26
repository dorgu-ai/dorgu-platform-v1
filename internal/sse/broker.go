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

// Package sse fans server-side state changes out to browser clients over
// text/event-stream.
//
// # Why a mailbox and not a queue
//
// Every event this broker carries is a complete snapshot of one view, so an
// older event for a topic is worthless the moment a newer one exists. Each
// subscriber therefore holds a mailbox of the latest event per topic rather than
// a queue of every event, which gives three properties a bounded queue cannot:
//
//   - a slow client never blocks an informer, because publish never waits
//   - a slow client never loses meaning, because it always gets the newest
//     state rather than a truncated history of stale ones
//   - memory is bounded by the number of topics, not by how far behind a client
//     has fallen
//
// This is what removes the need for a manual refresh. There is no path where the
// browser ends up holding state older than the server's without a subsequent
// event arriving to correct it.
package sse

import (
	"sync"
)

// Event is one server-sent event: a topic and its encoded payload.
type Event struct {
	// Topic is the SSE event name the browser listens for.
	Topic string
	// Data is the already-encoded payload. Encoding happens once per publish
	// rather than once per client, which is the whole reason the broker deals
	// in bytes instead of values.
	Data []byte
	// Seq is a monotonic sequence number used as the SSE id field. The browser
	// sends it back as Last-Event-ID on reconnect; the server does not replay
	// from it, because every event is a full snapshot and the client is sent a
	// fresh one on connect anyway.
	Seq uint64
}

// Broker fans events out to subscribers. It is safe for concurrent use.
type Broker struct {
	mu     sync.Mutex
	subs   map[uint64]*Subscription
	nextID uint64
	seq    uint64
}

// NewBroker returns an empty broker.
func NewBroker() *Broker {
	return &Broker{subs: map[uint64]*Subscription{}}
}

// Subscribe registers a new subscriber. The caller must Close it.
func (b *Broker) Subscribe() *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	sub := &Subscription{
		broker:  b,
		id:      b.nextID,
		pending: map[string]Event{},
		signal:  make(chan struct{}, 1),
	}
	b.subs[sub.id] = sub
	return sub
}

// Publish delivers an event to every subscriber, replacing any undelivered
// event for the same topic. It never blocks.
func (b *Broker) Publish(topic string, data []byte) {
	b.mu.Lock()
	b.seq++
	event := Event{Topic: topic, Data: data, Seq: b.seq}
	subs := make([]*Subscription, 0, len(b.subs))
	for _, sub := range b.subs {
		subs = append(subs, sub)
	}
	b.mu.Unlock()

	for _, sub := range subs {
		sub.deliver(event)
	}
}

// Subscribers reports how many clients are connected. Used by the meta endpoint
// and by tests.
func (b *Broker) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

func (b *Broker) remove(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, id)
}

// Subscription is one client's mailbox.
type Subscription struct {
	broker *Broker
	id     uint64

	mu      sync.Mutex
	pending map[string]Event
	// order preserves first-pending-first delivery across topics, so a topic
	// that keeps changing cannot starve one that changed earlier.
	order  []string
	closed bool
	signal chan struct{}
}

// Signal fires when at least one event is waiting. It is coalesced: several
// publishes produce one wake-up, and the handler drains everything pending.
func (s *Subscription) Signal() <-chan struct{} {
	return s.signal
}

// Drain returns every waiting event and empties the mailbox. It returns nil
// when nothing is waiting, which makes a spurious wake-up harmless.
func (s *Subscription) Drain() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.order) == 0 {
		return nil
	}
	out := make([]Event, 0, len(s.order))
	for _, topic := range s.order {
		out = append(out, s.pending[topic])
	}
	s.pending = map[string]Event{}
	s.order = s.order[:0]
	return out
}

// Close removes the subscription from its broker. It is idempotent.
func (s *Subscription) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	s.broker.remove(s.id)
}

func (s *Subscription) deliver(event Event) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if _, waiting := s.pending[event.Topic]; !waiting {
		s.order = append(s.order, event.Topic)
	}
	s.pending[event.Topic] = event
	s.mu.Unlock()

	select {
	case s.signal <- struct{}{}:
	default:
		// A wake-up is already pending. The handler has not drained yet, and
		// when it does it will see this event too.
	}
}
