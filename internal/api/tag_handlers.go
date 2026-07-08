package api

import (
	"encoding/json"
	"net/http"

	"github.com/vyanawatch/vyanawatch/internal/model"
)

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.repos.Tags.GetAll()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch tags")
		return
	}
	respondJSON(w, http.StatusOK, tags)
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var t model.Tag
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if t.Name == "" {
		respondError(w, http.StatusBadRequest, "Tag name is required")
		return
	}

	if t.Color == "" {
		t.Color = "#6b7280"
	}

	if err := s.repos.Tags.Create(&t); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create tag")
		return
	}
	respondJSON(w, http.StatusCreated, t)
}

func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := s.repos.Tags.GetByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Tag not found")
		return
	}

	var t model.Tag
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	t.ID = existing.ID
	t.CreatedAt = existing.CreatedAt
	if err := s.repos.Tags.Update(&t); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update tag")
		return
	}
	respondJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.repos.Tags.Delete(id); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete tag")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Tag deleted"})
}
