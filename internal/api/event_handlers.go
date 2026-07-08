package api

import (
	"net/http"
	"strconv"
)

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

func (s *Server) handleClearEventLogs(w http.ResponseWriter, r *http.Request) {
	if err := s.repos.EventLogs.DeleteAll(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to clear event logs")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Event logs cleared"})
}
