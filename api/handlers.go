package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/vyanawatch/vyanawatch/config"
	"github.com/vyanawatch/vyanawatch/db"
)

// handleListMonitors returns all monitors with their uptime stats.
// GET /api/v1/monitors
func (s *Server) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	monitors, err := s.repos.Monitors.GetAllWithStats()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch monitors")
		return
	}
	respondJSON(w, http.StatusOK, monitors)
}

// handleGetMonitor returns a single monitor with uptime stats.
// GET /api/v1/monitors/{id}
func (s *Server) handleGetMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	m, err := s.repos.Monitors.GetByIDWithStats(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Monitor not found")
		return
	}
	respondJSON(w, http.StatusOK, m)
}

// handleCreateMonitor creates a new monitor and starts its worker.
// POST /api/v1/monitors
func (s *Server) handleCreateMonitor(w http.ResponseWriter, r *http.Request) {
	var m db.Monitor
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	// Set defaults if not provided
	if m.Interval == 0 {
		m.Interval = 60
	}
	if m.Timeout == 0 {
		m.Timeout = 10
	}
	if m.Retries == 0 {
		m.Retries = 3
	}
	if m.RetryInterval == 0 {
		m.RetryInterval = 10
	}

	if err := s.repos.Monitors.Create(&m); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Start monitoring if active
	if m.Active {
		s.engine.AddMonitor(s.ctx, m)
	}

	// Broadcast event
	s.sse.Broadcast(SSEEvent{
		Event: "monitor_created",
		Data:  m,
	})

	// Persist event log
	s.repos.EventLogs.Create(&db.EventLog{
		MonitorID:   m.ID,
		MonitorName: m.Name,
		Status:      string(m.Status),
		EventType:   "created",
		Message:     "Monitor created",
	})

	respondJSON(w, http.StatusCreated, m)
}

// handleUpdateMonitor updates an existing monitor and restarts its worker.
// PUT /api/v1/monitors/{id}
func (s *Server) handleUpdateMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := s.repos.Monitors.GetByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Monitor not found")
		return
	}

	// Decode updates into a map first to handle partial updates
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	// Apply updates to the existing monitor via re-marshal/unmarshal
	existingJSON, _ := json.Marshal(existing)
	if err := json.Unmarshal(existingJSON, &updates); err == nil {
		// Merge: decode the request body again onto the existing
	}

	// Simpler approach: decode body directly into the existing monitor
	// Re-read is not possible since body is consumed, so we re-encode updates
	updatedJSON, _ := json.Marshal(updates)
	var updated db.Monitor
	if err := json.Unmarshal(updatedJSON, &updated); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid update data")
		return
	}

	// Preserve immutable fields
	updated.ID = existing.ID
	updated.CreatedAt = existing.CreatedAt
	updated.PushToken = existing.PushToken
	updated.Type = existing.Type // Don't allow type change

	if err := s.repos.Monitors.Update(&updated); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Restart worker with new config
	s.engine.RestartMonitor(s.ctx, updated)

	// Broadcast event
	s.sse.Broadcast(SSEEvent{
		Event: "monitor_updated",
		Data:  updated,
	})

	respondJSON(w, http.StatusOK, updated)
}

// handleDeleteMonitor deletes a monitor and stops its worker.
// DELETE /api/v1/monitors/{id}
func (s *Server) handleDeleteMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := s.repos.Monitors.GetByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Monitor not found")
		return
	}

	// Stop worker first
	s.engine.RemoveMonitor(id)

	if err := s.repos.Monitors.Delete(id); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete monitor")
		return
	}

	// Broadcast event
	s.sse.Broadcast(SSEEvent{
		Event: "monitor_deleted",
		Data:  map[string]uint{"id": id},
	})

	// Persist event log
	s.repos.EventLogs.Create(&db.EventLog{
		MonitorID:   id,
		MonitorName: existing.Name,
		Status:      "deleted",
		EventType:   "deleted",
		Message:     "Monitor deleted",
	})

	respondJSON(w, http.StatusOK, map[string]string{"message": "Monitor deleted"})
}

// handlePauseMonitor pauses a monitor.
// POST /api/v1/monitors/{id}/pause
func (s *Server) handlePauseMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	m, err := s.repos.Monitors.GetByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Monitor not found")
		return
	}

	m.Active = false
	m.Status = db.StatusPaused
	if err := s.repos.Monitors.Update(m); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to pause monitor")
		return
	}

	s.engine.RemoveMonitor(id)

	s.sse.Broadcast(SSEEvent{
		Event: "monitor_paused",
		Data:  m,
	})

	s.repos.EventLogs.Create(&db.EventLog{
		MonitorID:   m.ID,
		MonitorName: m.Name,
		Status:      string(m.Status),
		EventType:   "paused",
		Message:     "Monitor paused",
	})

	respondJSON(w, http.StatusOK, m)
}

// handleResumeMonitor resumes a paused monitor.
// POST /api/v1/monitors/{id}/resume
func (s *Server) handleResumeMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	m, err := s.repos.Monitors.GetByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Monitor not found")
		return
	}

	m.Active = true
	m.Status = db.StatusPending
	if err := s.repos.Monitors.Update(m); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to resume monitor")
		return
	}

	s.engine.AddMonitor(s.ctx, *m)

	s.sse.Broadcast(SSEEvent{
		Event: "monitor_resumed",
		Data:  m,
	})

	s.repos.EventLogs.Create(&db.EventLog{
		MonitorID:   m.ID,
		MonitorName: m.Name,
		Status:      string(m.Status),
		EventType:   "resumed",
		Message:     "Monitor resumed",
	})

	respondJSON(w, http.StatusOK, m)
}

// handleMonitorHistory returns response time history for a monitor.
// GET /api/v1/monitors/{id}/history?hours=24&limit=500
func (s *Server) handleMonitorHistory(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate monitor exists
	if _, err := s.repos.Monitors.GetByID(id); err != nil {
		respondError(w, http.StatusNotFound, "Monitor not found")
		return
	}

	// Parse query params
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := strconv.Atoi(h); err == nil && v > 0 && v <= 720 {
			hours = v
		}
	}

	limit := 500
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 5000 {
			limit = v
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	heartbeats, err := s.repos.Heartbeats.GetResponseTimeHistory(id, since, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch history")
		return
	}

	respondJSON(w, http.StatusOK, heartbeats)
}

// handleSummary returns aggregate status counts.
// GET /api/v1/monitors/summary
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.repos.Monitors.GetSummary()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch summary")
		return
	}
	respondJSON(w, http.StatusOK, summary)
}

// handlePush processes an incoming push/heartbeat check-in.
// POST /api/v1/push/{token}
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		respondError(w, http.StatusBadRequest, "Token is required")
		return
	}

	m, err := s.repos.Monitors.GetByPushToken(token)
	if err != nil {
		respondError(w, http.StatusNotFound, "Push monitor not found for this token")
		return
	}

	if err := s.engine.HandlePush(m); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to process push")
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "monitor_push",
		Data:  m,
	})

	respondJSON(w, http.StatusOK, map[string]string{"message": "OK", "monitor": m.Name})
}

// handleHealth returns server health status.
// GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": "dev",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// handleCloneMonitor duplicates a monitor with a new name.
// POST /api/v1/monitors/{id}/clone
func (s *Server) handleCloneMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	orig, err := s.repos.Monitors.GetByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Monitor not found")
		return
	}

	clone := *orig
	clone.ID = 0
	clone.CreatedAt = time.Time{}
	clone.UpdatedAt = time.Time{}
	clone.PushToken = "" // will be auto-generated by BeforeCreate
	clone.Name = orig.Name + " (Copy)"
	clone.Status = db.StatusPending
	clone.LastCheckAt = nil
	clone.ResponseTime = 0
	clone.Heartbeats = nil
	clone.Incidents = nil

	if err := s.repos.Monitors.Create(&clone); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if clone.Active {
		s.engine.AddMonitor(s.ctx, clone)
	}

	s.sse.Broadcast(SSEEvent{
		Event: "monitor_created",
		Data:  clone,
	})

	respondJSON(w, http.StatusCreated, clone)
}

// handleAuthMe returns the current auth state.
// GET /api/v1/auth/me
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	authEnabled := cfg != nil && cfg.Auth.Enabled
	username := ""
	if authEnabled {
		if cookie, err := r.Cookie("vyanawatch_session"); err == nil {
			username, _ = validateSession(cookie.Value)
		}
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"auth_enabled": authEnabled,
		"username":     username,
	})
}

// handleListEventLogs returns recent event logs.
// GET /api/v1/events/log?limit=50
func (s *Server) handleListEventLogs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	logs, err := s.repos.EventLogs.GetRecent(limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch event logs")
		return
	}
	respondJSON(w, http.StatusOK, logs)
}

// handleClearEventLogs deletes all event logs.
// DELETE /api/v1/events/log
func (s *Server) handleClearEventLogs(w http.ResponseWriter, r *http.Request) {
	if err := s.repos.EventLogs.DeleteAll(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to clear event logs")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Event logs cleared"})
}
