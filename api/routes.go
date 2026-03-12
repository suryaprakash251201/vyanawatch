package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog/log"
	"github.com/vyanawatch/vyanawatch/db"
	"github.com/vyanawatch/vyanawatch/monitor"
	"github.com/vyanawatch/vyanawatch/ui"
)

// Server holds the HTTP server and its dependencies.
type Server struct {
	router *chi.Mux
	server *http.Server
	repos  *db.Repos
	engine *monitor.Engine
	sse    *SSEBroker
	ctx    context.Context
}

// NewServer creates a new API server with all routes configured.
func NewServer(repos *db.Repos, engine *monitor.Engine, sseCtx context.Context) *Server {
	s := &Server{
		repos:  repos,
		engine: engine,
		sse:    NewSSEBroker(),
		ctx:    sseCtx,
	}

	// Start SSE broker
	go s.sse.Run(sseCtx)

	s.router = s.setupRoutes()
	return s
}

// Start begins listening on the given address.
func (s *Server) Start(addr string) error {
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second, // longer for SSE
		IdleTimeout:  60 * time.Second,
	}
	log.Info().Str("addr", addr).Msg("HTTP server starting")
	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Broadcast sends an SSE event to all connected clients.
func (s *Server) Broadcast(event SSEEvent) {
	s.sse.Broadcast(event)
}

// setupRoutes configures all HTTP routes.
func (s *Server) setupRoutes() *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))
	r.Use(requestLogger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Public routes (no auth required)
	r.HandleFunc("/login", s.handleLogin)
	r.Get("/logout", s.handleLogout)
	r.Get("/health", s.handleHealth)
	r.Post("/api/v1/push/{token}", s.handlePush)
	r.Get("/status/{slug}", s.handleStatusPageHTML)
	r.Get("/api/v1/status/{slug}", s.handlePublicStatusPage)

	// Protected routes (auth middleware applied)
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)

		// Dashboard UI
		r.Get("/", s.handleDashboard)

		// API v1
		r.Route("/api/v1", func(r chi.Router) {
			// Monitors CRUD
			r.Route("/monitors", func(r chi.Router) {
				r.Get("/", s.handleListMonitors)
				r.Post("/", s.handleCreateMonitor)
				r.Get("/summary", s.handleSummary)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", s.handleGetMonitor)
					r.Put("/", s.handleUpdateMonitor)
					r.Delete("/", s.handleDeleteMonitor)
					r.Get("/history", s.handleMonitorHistory)
					r.Post("/pause", s.handlePauseMonitor)
					r.Post("/resume", s.handleResumeMonitor)
					r.Post("/clone", s.handleCloneMonitor)
				})
			})

			// Status pages
			r.Route("/status-pages", func(r chi.Router) {
				r.Get("/", s.handleListStatusPages)
				r.Post("/", s.handleCreateStatusPage)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", s.handleGetStatusPage)
					r.Put("/", s.handleUpdateStatusPage)
					r.Delete("/", s.handleDeleteStatusPage)
					r.Post("/monitors/{monitorId}", s.handleAddMonitorToStatusPage)
					r.Delete("/monitors/{monitorId}", s.handleRemoveMonitorFromStatusPage)
				})
			})

			// SSE real-time events
			r.Get("/events", s.handleSSE)

			// Event Logs
			r.Get("/events/log", s.handleListEventLogs)
			r.Delete("/events/log", s.handleClearEventLogs)

			// Settings
			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handleUpdateSettings)
			r.Post("/settings/test-email", s.handleSendTestEmail)
			r.Get("/settings/email-preview", s.handlePreviewEmailTemplate)

			// Auth info
			r.Get("/auth/me", s.handleAuthMe)
		})
	})

	return r
}

// requestLogger is a lightweight zerolog-based request logger middleware.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		log.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", ww.Status()).
			Dur("latency", time.Since(start)).
			Msg("HTTP request")
	})
}

// respondJSON writes a JSON response.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Error().Err(err).Msg("Failed to write JSON response")
		}
	}
}

// respondError writes a JSON error response.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// parseUintParam extracts a uint URL parameter from Chi route.
func parseUintParam(r *http.Request, key string) (uint, error) {
	str := chi.URLParam(r, key)
	var id uint
	if _, err := fmt.Sscanf(str, "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid %s: %s", key, str)
	}
	return id, nil
}

// handleDashboard serves the main dashboard HTML page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(ui.DashboardHTML)
}

// handleStatusPageHTML serves the public status page HTML.
func (s *Server) handleStatusPageHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(ui.StatusHTML)
}
