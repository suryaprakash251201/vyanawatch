package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

type SSEEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

type SSEClient struct {
	ch chan SSEEvent
}

type SSEBroker struct {
	clients    map[*SSEClient]bool
	register   chan *SSEClient
	unregister chan *SSEClient
	broadcast  chan SSEEvent
	mu         sync.RWMutex
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEEvent, 64),
	}
}

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
				}
			}
			b.mu.RUnlock()
		}
	}
}

func (b *SSEBroker) Broadcast(event SSEEvent) {
	select {
	case b.broadcast <- event:
	default:
		log.Warn().Str("event", event.Event).Msg("SSE broadcast channel full, dropping event")
	}
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client := &SSEClient{
		ch: make(chan SSEEvent, 32),
	}

	s.sse.register <- client

	ctx := r.Context()

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

func writeSSE(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}
