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
	"github.com/vyanawatch/vyanawatch/internal/config"
	"github.com/vyanawatch/vyanawatch/internal/engine"
	"github.com/vyanawatch/vyanawatch/internal/repo"
	"github.com/vyanawatch/vyanawatch/ui"
)

type Server struct {
	router    *chi.Mux
	server    *http.Server
	repos     *Repositories
	engine    *engine.Engine
	sse       *SSEBroker
	ctx       context.Context
	cfg       *config.Provider
	notifiers *NotifierManager
}

type Repositories struct {
	Monitors    repo.MonitorRepository
	Heartbeats  repo.HeartbeatRepository
	Incidents   repo.IncidentRepository
	StatusPages repo.StatusPageRepository
	EventLogs   repo.EventLogRepository
	Tags        repo.TagRepository
	Maintenance repo.MaintenanceRepository
}

type NotifierManager struct {
	Dispatch func(event engine.Event)
}

func NewServer(
	repos *Repositories,
	eng *engine.Engine,
	cfg *config.Provider,
	sseCtx context.Context,
	notifiers *NotifierManager,
) *Server {
	s := &Server{
		repos:     repos,
		engine:    eng,
		sse:       NewSSEBroker(),
		ctx:       sseCtx,
		cfg:       cfg,
		notifiers: notifiers,
	}

	go s.sse.Run(sseCtx)

	s.router = s.setupRoutes()
	return s
}

func (s *Server) Start(addr string) error {
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Info().Str("addr", addr).Msg("HTTP server starting")
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) Broadcast(event SSEEvent) {
	s.sse.Broadcast(event)
}

func (s *Server) setupRoutes() *chi.Mux {
	r := chi.NewRouter()

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

	authMW := newAuthMiddleware(s.cfg)

	r.HandleFunc("/login", s.handleLogin)
	r.Get("/logout", s.handleLogout)
	r.Get("/health", s.handleHealth)
	r.Post("/api/v1/push/{token}", s.handlePush)
	r.Get("/status/{slug}", s.handleStatusPageHTML)
	r.Get("/api/v1/status/{slug}", s.handlePublicStatusPage)

	r.Group(func(r chi.Router) {
		r.Use(authMW.Middleware)

		r.Get("/", s.handleDashboard)

		r.Route("/api/v1", func(r chi.Router) {
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

			r.Route("/tags", func(r chi.Router) {
				r.Get("/", s.handleListTags)
				r.Post("/", s.handleCreateTag)
				r.Route("/{id}", func(r chi.Router) {
					r.Put("/", s.handleUpdateTag)
					r.Delete("/", s.handleDeleteTag)
				})
			})

			r.Route("/maintenance", func(r chi.Router) {
				r.Get("/", s.handleListMaintenance)
				r.Post("/", s.handleCreateMaintenance)
				r.Route("/{id}", func(r chi.Router) {
					r.Put("/", s.handleUpdateMaintenance)
					r.Delete("/", s.handleDeleteMaintenance)
				})
			})

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

			r.Get("/events", s.handleSSE)

			r.Get("/events/log", s.handleListEventLogs)
			r.Delete("/events/log", s.handleClearEventLogs)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handleUpdateSettings)
			r.Post("/settings/test-email", s.handleSendTestEmail)
			r.Post("/settings/test-telegram", s.handleSendTestTelegram)
			r.Post("/settings/test-discord", s.handleSendTestDiscord)
			r.Post("/settings/test-webhook", s.handleSendTestWebhook)
			r.Get("/settings/email-preview", s.handlePreviewEmailTemplate)

			r.Get("/auth/me", s.handleAuthMe)
		})
	})

	return r
}

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

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Error().Err(err).Msg("Failed to write JSON response")
		}
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func parseUintParam(r *http.Request, key string) (uint, error) {
	str := chi.URLParam(r, key)
	var id uint
	if _, err := fmt.Sscanf(str, "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid %s: %s", key, str)
	}
	return id, nil
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(ui.DashboardHTML)
}

func (s *Server) handleStatusPageHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(ui.StatusHTML)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": "dev",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}
