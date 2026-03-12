package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

// SSEEvent represents a Server-Sent Event.
type SSEEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// SSEClient is a connected SSE client.
type SSEClient struct {
	ch chan SSEEvent
}

// SSEBroker manages SSE client connections and event broadcasting.
type SSEBroker struct {
	clients    map[*SSEClient]bool
	register   chan *SSEClient
	unregister chan *SSEClient
	broadcast  chan SSEEvent
	mu         sync.RWMutex
}

// NewSSEBroker creates a new SSE broker.
func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEEvent, 64),
	}
}

// Run starts the SSE broker event loop. Should be run in a goroutine.
func (b *SSEBroker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			b.mu.Lock()
			for client := range b.clients {
				close(client.ch)
				delete(b.clients, client)
			}
			b.mu.Unlock()
			return

		case client := <-b.register:
			b.mu.Lock()
			b.clients[client] = true
			b.mu.Unlock()
			log.Debug().Int("total", len(b.clients)).Msg("SSE client connected")

		case client := <-b.unregister:
			b.mu.Lock()
			if _, ok := b.clients[client]; ok {
				close(client.ch)
				delete(b.clients, client)
			}
			b.mu.Unlock()
			log.Debug().Int("total", len(b.clients)).Msg("SSE client disconnected")

		case event := <-b.broadcast:
			b.mu.RLock()
			for client := range b.clients {
				select {
				case client.ch <- event:
				default:
					// Client buffer full, skip
				}
			}
			b.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to all connected SSE clients.
func (b *SSEBroker) Broadcast(event SSEEvent) {
	select {
	case b.broadcast <- event:
	default:
		// Broadcast channel full, drop event
		log.Warn().Str("event", event.Event).Msg("SSE broadcast channel full, dropping event")
	}
}

// handleSSE handles SSE connections from clients.
// GET /api/v1/events
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client
	client := &SSEClient{
		ch: make(chan SSEEvent, 32),
	}

	// Register client
	s.sse.register <- client

	// Ensure cleanup on disconnect
	ctx := r.Context()

	// Send initial connection event
	writeSSE(w, "connected", `{"message":"Connected to VyanaWatch SSE"}`)
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			s.sse.unregister <- client
			return
		case event, ok := <-client.ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event.Data)
			if err != nil {
				continue
			}
			writeSSE(w, event.Event, string(data))
			flusher.Flush()
		}
	}
}

// writeSSE writes a single SSE message to the response writer.
func writeSSE(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}
