// ============================================================
// controller/internal/api/sse.go — Real-time event streaming
//
// SSE stands for Server-Sent Events. It's a simple way for the
// server to push messages to the browser without the browser
// having to keep asking "anything new?".
//
// How it works:
//   1. Browser opens a long-lived HTTP connection to /api/events
//   2. When a scan runs, we push events down that connection
//   3. The browser receives them instantly and updates the UI
//
// The Broker is the "post office" — it keeps a list of all open
// browser connections and delivers each event to all of them.
// ============================================================

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Broker manages a list of connected browser clients and delivers events to all of them.
type Broker struct {
	mu      sync.RWMutex            // protects `clients` from concurrent access
	clients map[chan []byte]struct{} // one channel per connected browser tab
}

// NewBroker creates an empty broker ready to accept clients.
func NewBroker() *Broker {
	return &Broker{clients: make(map[chan []byte]struct{})}
}

// Publish converts `v` to JSON and sends it to every connected browser tab.
// If a browser tab is too slow to receive, its message is dropped (non-blocking).
func (b *Broker) Publish(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- data: // deliver the message
		default:         // tab is too slow — skip it rather than block everything
		}
	}
}

// subscribe registers a new browser connection and returns:
//   - ch: a channel that receives event bytes
//   - unsub: a cleanup function to call when the browser disconnects
func (b *Broker) subscribe() (ch chan []byte, unsub func()) {
	ch = make(chan []byte, 16) // buffer up to 16 messages before dropping
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.clients, ch) // remove this client from the list
		b.mu.Unlock()
		close(ch) // signal the SSE handler loop to exit
	}
}

// serveSSE handles the /api/events HTTP endpoint.
// It keeps the connection open and streams events to the browser as they happen.
func (b *Broker) serveSSE(w http.ResponseWriter, r *http.Request) {
	// Check that the response writer supports streaming (flushing partial responses)
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Tell the browser this is a streaming response that stays open
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // tell nginx not to buffer (if behind a proxy)

	// Tell the browser: if the connection drops, retry after 1 second
	fmt.Fprintf(w, "retry: 1000\n")
	// Send a "connected" event so the browser knows the stream is working
	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush() // push these bytes to the browser immediately

	// Register this browser tab as a client
	ch, unsub := b.subscribe()
	defer unsub() // unregister when the function exits (browser disconnected)

	// Loop forever: wait for events and send them to the browser
	for {
		select {
		case <-r.Context().Done():
			// Browser closed the tab or navigated away — stop the loop
			return
		case data, ok := <-ch:
			if !ok {
				// Channel was closed (broker is shutting down)
				return
			}
			// SSE format: each message starts with "data: " and ends with two newlines
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush() // push immediately — don't wait to fill a buffer
		}
	}
}
