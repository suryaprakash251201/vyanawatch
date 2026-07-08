package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/vyanawatch/vyanawatch/internal/model"
)

func (s *Server) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	monitors, err := s.repos.Monitors.GetAllWithStats()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch monitors")
		return
	}
	respondJSON(w, http.StatusOK, monitors)
}

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

func (s *Server) handleCreateMonitor(w http.ResponseWriter, r *http.Request) {
	var m model.Monitor
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

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

	if m.Active {
		s.engine.AddMonitor(s.ctx, m)
	}

	s.sse.Broadcast(SSEEvent{
		Event: "monitor_created",
		Data:  m,
	})

	s.repos.EventLogs.Create(&model.EventLog{
		MonitorID:   m.ID,
		MonitorName: m.Name,
		Status:      string(m.Status),
		EventType:   "created",
		Message:     "Monitor created",
	})

	respondJSON(w, http.StatusCreated, m)
}

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

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	updated := *existing

	if v, ok := raw["name"]; ok {
		if err := json.Unmarshal(v, &updated.Name); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: name")
			return
		}
	}
	if v, ok := raw["interval"]; ok {
		if err := json.Unmarshal(v, &updated.Interval); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: interval")
			return
		}
	}
	if v, ok := raw["timeout"]; ok {
		if err := json.Unmarshal(v, &updated.Timeout); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: timeout")
			return
		}
	}
	if v, ok := raw["retries"]; ok {
		if err := json.Unmarshal(v, &updated.Retries); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: retries")
			return
		}
	}
	if v, ok := raw["retry_interval"]; ok {
		if err := json.Unmarshal(v, &updated.RetryInterval); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: retry_interval")
			return
		}
	}
	if v, ok := raw["active"]; ok {
		if err := json.Unmarshal(v, &updated.Active); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: active")
			return
		}
	}

	if v, ok := raw["url"]; ok {
		if err := json.Unmarshal(v, &updated.URL); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: url")
			return
		}
	}
	if v, ok := raw["method"]; ok {
		if err := json.Unmarshal(v, &updated.Method); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: method")
			return
		}
	}
	if v, ok := raw["expected_status_code"]; ok {
		if err := json.Unmarshal(v, &updated.ExpectedStatusCode); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: expected_status_code")
			return
		}
	}
	if v, ok := raw["keyword_check"]; ok {
		if err := json.Unmarshal(v, &updated.KeywordCheck); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: keyword_check")
			return
		}
	}
	if v, ok := raw["hostname"]; ok {
		if err := json.Unmarshal(v, &updated.Hostname); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: hostname")
			return
		}
	}
	if v, ok := raw["port"]; ok {
		if err := json.Unmarshal(v, &updated.Port); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: port")
			return
		}
	}
	if v, ok := raw["dns_type"]; ok {
		if err := json.Unmarshal(v, &updated.DNSType); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: dns_type")
			return
		}
	}
	if v, ok := raw["alert_enabled"]; ok {
		if err := json.Unmarshal(v, &updated.AlertEnabled); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: alert_enabled")
			return
		}
	}
	if v, ok := raw["alert_channels"]; ok {
		if err := json.Unmarshal(v, &updated.AlertChannels); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: alert_channels")
			return
		}
	}
	if v, ok := raw["ssl_check"]; ok {
		if err := json.Unmarshal(v, &updated.SSLCheck); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: ssl_check")
			return
		}
	}
	if v, ok := raw["ssl_expiry_days"]; ok {
		if err := json.Unmarshal(v, &updated.SSLExpiryDays); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: ssl_expiry_days")
			return
		}
	}
	if v, ok := raw["keyword_present"]; ok {
		if err := json.Unmarshal(v, &updated.KeywordPresent); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: keyword_present")
			return
		}
	}
	if v, ok := raw["headers"]; ok {
		if err := json.Unmarshal(v, &updated.Headers); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: headers")
			return
		}
	}
	if v, ok := raw["body"]; ok {
		if err := json.Unmarshal(v, &updated.Body); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: body")
			return
		}
	}
	if v, ok := raw["tag_ids"]; ok {
		var tagIDs []uint
		if err := json.Unmarshal(v, &tagIDs); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid field: tag_ids")
			return
		}
		if err := s.repos.Tags.SetMonitorTags(id, tagIDs); err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to set tags")
			return
		}
	}

	updated.ID = existing.ID
	updated.CreatedAt = existing.CreatedAt
	updated.PushToken = existing.PushToken
	updated.Type = existing.Type

	if err := s.repos.Monitors.Update(&updated); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if updated.Active {
		s.engine.RestartMonitor(s.ctx, updated)
	} else {
		s.engine.RemoveMonitor(updated.ID)
	}

	s.sse.Broadcast(SSEEvent{
		Event: "monitor_updated",
		Data:  updated,
	})

	s.repos.EventLogs.Create(&model.EventLog{
		MonitorID:   updated.ID,
		MonitorName: updated.Name,
		Status:      string(updated.Status),
		EventType:   "updated",
		Message:     "Monitor updated",
	})

	respondJSON(w, http.StatusOK, updated)
}

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

	s.engine.RemoveMonitor(id)

	if err := s.repos.Monitors.Delete(id); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete monitor")
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "monitor_deleted",
		Data:  map[string]uint{"id": id},
	})

	s.repos.EventLogs.Create(&model.EventLog{
		MonitorID:   id,
		MonitorName: existing.Name,
		Status:      "deleted",
		EventType:   "deleted",
		Message:     "Monitor deleted",
	})

	respondJSON(w, http.StatusOK, map[string]string{"message": "Monitor deleted"})
}

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
	m.Status = model.StatusPaused
	if err := s.repos.Monitors.Update(m); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to pause monitor")
		return
	}

	s.engine.RemoveMonitor(id)

	s.sse.Broadcast(SSEEvent{
		Event: "monitor_paused",
		Data:  m,
	})

	s.repos.EventLogs.Create(&model.EventLog{
		MonitorID:   m.ID,
		MonitorName: m.Name,
		Status:      string(m.Status),
		EventType:   "paused",
		Message:     "Monitor paused",
	})

	respondJSON(w, http.StatusOK, m)
}

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
	m.Status = model.StatusPending
	if err := s.repos.Monitors.Update(m); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to resume monitor")
		return
	}

	s.engine.AddMonitor(s.ctx, *m)

	s.sse.Broadcast(SSEEvent{
		Event: "monitor_resumed",
		Data:  m,
	})

	s.repos.EventLogs.Create(&model.EventLog{
		MonitorID:   m.ID,
		MonitorName: m.Name,
		Status:      string(m.Status),
		EventType:   "resumed",
		Message:     "Monitor resumed",
	})

	respondJSON(w, http.StatusOK, m)
}

func (s *Server) handleMonitorHistory(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := s.repos.Monitors.GetByID(id); err != nil {
		respondError(w, http.StatusNotFound, "Monitor not found")
		return
	}

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

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.repos.Monitors.GetSummary()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch summary")
		return
	}
	respondJSON(w, http.StatusOK, summary)
}

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
	clone.PushToken = ""
	clone.Name = orig.Name + " (Copy)"
	clone.Status = model.StatusPending
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
