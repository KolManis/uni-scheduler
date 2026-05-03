package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/KolManis/uni-scheduler/internal/usecase/generator"
	"github.com/gorilla/mux"
)

type ScheduleHandler struct {
	usecase generator.Usecase
}

func NewScheduleHandler(usecase generator.Usecase) *ScheduleHandler {
	return &ScheduleHandler{usecase: usecase}
}

// POST /api/v1/schedules/generate
func (h *ScheduleHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	result, err := h.usecase.Generate(r.Context(), generator.GenerateInput{
		Name:          req.Name,
		MaxIterations: req.MaxIterations,
	})
	if err != nil {
		if errors.Is(err, schedule.ErrNoSolution) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toScheduleResponse(result))
}

// GET /api/v1/schedules
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.usecase.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]ScheduleResponse, 0, len(schedules))
	for i := range schedules {
		response = append(response, toScheduleResponse(&schedules[i]))
	}

	writeJSON(w, http.StatusOK, response)
}

// GET /api/v1/schedules/{id}
func (h *ScheduleHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	result, err := h.usecase.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, schedule.ErrNotFound) {
			writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toScheduleResponse(result))
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}
