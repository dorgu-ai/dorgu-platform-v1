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

package httpapi

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/sse"
)

// handleStream serves the text/event-stream every view listens on.
//
// # Why SSE and not a WebSocket
//
// Updates are one-directional: the server pushes state and the browser never
// pushes back over this channel. SSE gets automatic reconnection from the
// browser for free, needs no upgrade handshake to survive a proxy, and is plain
// HTTP so it inherits the same Host guard and CSP as every other route. A
// WebSocket would add a framing protocol and a reconnect loop to write, in
// exchange for a direction of travel nothing uses.
//
// # Why every client gets a snapshot on connect
//
// The first thing a new or reconnecting client receives is a full snapshot of
// every view. That is what makes "no manual refresh" true rather than aspirational:
// after a laptop sleeps, a VPN drops or the process restarts, the browser
// reconnects on its own and is immediately correct, with no cache invalidation to
// reason about and no reload button to reach for. Last-Event-ID is deliberately
// ignored, because replaying a history of snapshots is strictly worse than
// sending the current one.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this server\n", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("Connection", "keep-alive")
	// Tell any proxy in the path not to buffer. nginx and its derivatives will
	// otherwise hold the stream until a buffer fills, which turns a live feed
	// into a batch one.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sub := s.cfg.Broker.Subscribe()
	defer sub.Close()

	buf := &bytes.Buffer{}

	// retry tells the browser how long to wait before reconnecting. Two seconds
	// is short enough that a restarted server feels instant and long enough not
	// to hammer a server that is genuinely down.
	buf.WriteString("retry: 2000\n\n")

	for _, topic := range StreamTopics {
		if data, encoded := s.EncodeTopic(topic); encoded {
			writeEvent(buf, string(topic), 0, data)
		}
	}
	if err := flush(w, flusher, buf); err != nil {
		return
	}

	heartbeat := time.NewTicker(s.heartbeat)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return

		case <-sub.Signal():
			events := sub.Drain()
			if len(events) == 0 {
				continue
			}
			for _, event := range events {
				writeEvent(buf, event.Topic, event.Seq, event.Data)
			}
			if err := flush(w, flusher, buf); err != nil {
				return
			}

		case <-heartbeat.C:
			// A comment line. The browser ignores it; the TCP connection and
			// any proxy in between do not.
			buf.WriteString(": keep-alive\n\n")
			if err := flush(w, flusher, buf); err != nil {
				return
			}
		}
	}
}

// writeEvent appends one SSE frame.
//
// Payloads are JSON, which never contains a raw newline, so a single data line
// is correct. Splitting on newlines anyway would be dead code defending against
// an encoder we control.
func writeEvent(buf *bytes.Buffer, topic string, seq uint64, data []byte) {
	if seq > 0 {
		fmt.Fprintf(buf, "id: %d\n", seq)
	}
	fmt.Fprintf(buf, "event: %s\n", topic)
	buf.WriteString("data: ")
	buf.Write(data)
	buf.WriteString("\n\n")
}

// flush writes the buffer to the client and empties it. A write error means the
// client is gone, which is ordinary and not logged as a failure.
func flush(w http.ResponseWriter, flusher http.Flusher, buf *bytes.Buffer) error {
	if buf.Len() == 0 {
		return nil
	}
	_, err := w.Write(buf.Bytes())
	buf.Reset()
	if err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// compile-time assertion that the broker's event shape is what writeEvent
// consumes, so a change to one has to visit the other.
var _ = func(e sse.Event) (string, uint64, []byte) { return e.Topic, e.Seq, e.Data }
