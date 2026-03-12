package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vyanawatch/vyanawatch/db"
)

// handleListStatusPages returns all status pages.
// GET /api/v1/status-pages
func (s *Server) handleListStatusPages(w http.ResponseWriter, r *http.Request) {
	pages, err := s.repos.StatusPages.GetAll()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch status pages")
		return
	}
	respondJSON(w, http.StatusOK, pages)
}

// handleGetStatusPage returns a single status page with its monitors.
// GET /api/v1/status-pages/{id}
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

// handleCreateStatusPage creates a new status page.
// POST /api/v1/status-pages
func (s *Server) handleCreateStatusPage(w http.ResponseWriter, r *http.Request) {
	var sp db.StatusPage
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

// handleUpdateStatusPage updates an existing status page.
// PUT /api/v1/status-pages/{id}
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

	var sp db.StatusPage
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

// handleDeleteStatusPage deletes a status page.
// DELETE /api/v1/status-pages/{id}
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

// handleAddMonitorToStatusPage assigns a monitor to a status page.
// POST /api/v1/status-pages/{id}/monitors/{monitorId}
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

	// Validate both exist
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

// handleRemoveMonitorFromStatusPage removes a monitor from a status page.
// DELETE /api/v1/status-pages/{id}/monitors/{monitorId}
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

// handlePublicStatusPage renders a public status page by slug.
// GET /status/{slug}
func (s *Server) handlePublicStatusPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	page, err := s.repos.StatusPages.GetBySlug(slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Status page not found")
		return
	}

	// Build response with uptime stats for each monitor
	type monitorStatus struct {
		db.Monitor
		UptimeStats db.UptimeStats `json:"uptime_stats"`
	}

	result := struct {
		*db.StatusPage
		Monitors []monitorStatus `json:"monitors"`
	}{
		StatusPage: page,
		Monitors:   make([]monitorStatus, 0, len(page.Monitors)),
	}

	hbRepo := db.NewHeartbeatRepo()
	for _, m := range page.Monitors {
		stats, _ := hbRepo.GetUptimeStats(m.ID)
		result.Monitors = append(result.Monitors, monitorStatus{
			Monitor:     m,
			UptimeStats: stats,
		})
	}

	respondJSON(w, http.StatusOK, result)
}
