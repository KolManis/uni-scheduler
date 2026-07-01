package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

type referenceInput interface {
	LoadInput(ctx context.Context) (*schedule.InputData, error)
}

// ReferenceHandler возвращает данные справочников.
type ReferenceHandler struct {
	repo referenceInput
}

func NewReferenceHandler(repo referenceInput) *ReferenceHandler {
	return &ReferenceHandler{repo: repo}
}

func (h *ReferenceHandler) GetTeachers(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.Teachers)
}

func (h *ReferenceHandler) GetGroups(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.Groups)
}

func (h *ReferenceHandler) GetRooms(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.Rooms)
}

func (h *ReferenceHandler) GetSubjectPlans(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.SubjectPlans)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
