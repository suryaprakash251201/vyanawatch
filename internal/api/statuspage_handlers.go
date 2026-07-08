package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/vyanawatch/vyanawatch/internal/model"
)

func (s *Server) handleListStatusPages(w http.ResponseWriter, r *http.Request) {
	pages, err := s.repos.StatusPages.GetAll()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch status pages")
		return
	}
	respondJSON(w, http.StatusOK, pages)
}

func (s *Server) handleGetStatusPage(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := s.repos.StatusPages.GetByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Status page not found")
		return
	}
	respondJSON(w, http.StatusOK, page)
}

func (s *Server) handleCreateStatusPage(w http.ResponseWriter, r *http.Request) {
	var sp model.StatusPage
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if sp.Name == "" {
		respondError(w, http.StatusBadRequest, "Name is required")
		return
	}

	if err := s.repos.StatusPages.Create(&sp); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create status page")
		return
	}
	respondJSON(w, http.StatusCreated, sp)
}

func (s *Server) handleUpdateStatusPage(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := s.repos.StatusPages.GetByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Status page not found")
		return
	}

	var sp model.StatusPage
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	sp.ID = existing.ID
	sp.CreatedAt = existing.CreatedAt
	if err := s.repos.StatusPages.Update(&sp); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update status page")
		return
	}
	respondJSON(w, http.StatusOK, sp)
}

func (s *Server) handleDeleteStatusPage(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := s.repos.StatusPages.GetByID(id); err != nil {
		respondError(w, http.StatusNotFound, "Status page not found")
		return
	}

	if err := s.repos.StatusPages.Delete(id); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete status page")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Status page deleted"})
}

func (s *Server) handleAddMonitorToStatusPage(w http.ResponseWriter, r *http.Request) {
	pageID, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	monitorID, err := parseUintParam(r, "monitorId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := s.repos.StatusPages.GetByID(pageID); err != nil {
		respondError(w, http.StatusNotFound, "Status page not found")
		return
	}
	if _, err := s.repos.Monitors.GetByID(monitorID); err != nil {
		respondError(w, http.StatusNotFound, "Monitor not found")
		return
	}

	if err := s.repos.StatusPages.AddMonitor(pageID, monitorID); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to add monitor to status page")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Monitor added to status page"})
}

func (s *Server) handleRemoveMonitorFromStatusPage(w http.ResponseWriter, r *http.Request) {
	monitorID, err := parseUintParam(r, "monitorId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.repos.StatusPages.RemoveMonitor(monitorID); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to remove monitor from status page")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Monitor removed from status page"})
}

func (s *Server) handlePublicStatusPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	page, err := s.repos.StatusPages.GetBySlug(slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Status page not found")
		return
	}

	type monitorStatus struct {
		model.Monitor
		UptimeStats      model.UptimeStats `json:"uptime_stats"`
		RecentHeartbeats []model.Heartbeat `json:"recent_heartbeats"`
	}

	result := struct {
		*model.StatusPage
		Monitors []monitorStatus `json:"monitors"`
	}{
		StatusPage: page,
		Monitors:   make([]monitorStatus, 0, len(page.Monitors)),
	}

	for _, m := range page.Monitors {
		stats, _ := s.repos.Heartbeats.GetUptimeStats(m.ID)
		since := time.Now().Add(-1 * time.Hour)
		heartbeats, _ := s.repos.Heartbeats.GetResponseTimeHistory(m.ID, since, 50)
		result.Monitors = append(result.Monitors, monitorStatus{
			Monitor:          m,
			UptimeStats:      stats,
			RecentHeartbeats: heartbeats,
		})
	}

	respondJSON(w, http.StatusOK, result)
}
