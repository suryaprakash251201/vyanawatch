package api

import (
	"encoding/json"
	"net/http"

	"github.com/vyanawatch/vyanawatch/internal/model"
)

func (s *Server) handleListMaintenance(w http.ResponseWriter, r *http.Request) {
	items, err := s.repos.Maintenance.GetAll()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch maintenance windows")
		return
	}
	respondJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateMaintenance(w http.ResponseWriter, r *http.Request) {
	var m model.Maintenance
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if m.Title == "" {
		respondError(w, http.StatusBadRequest, "Title is required")
		return
	}

	if err := s.repos.Maintenance.Create(&m); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create maintenance window")
		return
	}
	respondJSON(w, http.StatusCreated, m)
}

func (s *Server) handleUpdateMaintenance(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := s.repos.Maintenance.GetByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Maintenance window not found")
		return
	}

	var m model.Maintenance
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	m.ID = existing.ID
	m.CreatedAt = existing.CreatedAt
	if err := s.repos.Maintenance.Update(&m); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update maintenance window")
		return
	}
	respondJSON(w, http.StatusOK, m)
}

func (s *Server) handleDeleteMaintenance(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.repos.Maintenance.Delete(id); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete maintenance window")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Maintenance window deleted"})
}
