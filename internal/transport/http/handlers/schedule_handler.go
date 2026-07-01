package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/KolManis/uni-scheduler/internal/importer"
	pgRepo "github.com/KolManis/uni-scheduler/internal/repository/postgres"
	"github.com/KolManis/uni-scheduler/internal/usecase/generator"
	"github.com/gorilla/mux"
)

// scheduleService — интерфейс для работы с расписаниями.
type scheduleService interface {
	Generate(ctx context.Context, in generator.GenerateInput) (*schedule.Schedule, error)
	GetByID(ctx context.Context, id int64) (*schedule.Schedule, error)
	List(ctx context.Context) ([]pgRepo.ScheduleSummary, error)
	Delete(ctx context.Context, id int64) error
	PatchAssignment(ctx context.Context, schedID int64, idx int, req generator.PatchRequest) (*schedule.Schedule, error)
	ImportExcel(ctx context.Context, data *importer.ImportedData) (*importer.ImportResult, error)
}

// ScheduleHandler обрабатывает HTTP-запросы к расписаниям.
type ScheduleHandler struct {
	svc scheduleService
}

func NewScheduleHandler(svc scheduleService) *ScheduleHandler {
	return &ScheduleHandler{svc: svc}
}

// POST /api/v1/schedules/generate
func (h *ScheduleHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	result, err := h.svc.Generate(r.Context(), generator.GenerateInput{
		Name:          req.Name,
		MaxIterations: req.MaxIterations,
		SolverType:    req.SolverType,
		TimeoutSec:    req.TimeoutSec,
	})
	if err != nil {
		if errors.Is(err, schedule.ErrNoSolution) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// GET /api/v1/schedules
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []pgRepo.ScheduleSummary{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// GET /api/v1/schedules/{id}
func (h *ScheduleHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	result, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, schedule.ErrNotFound) {
			writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// DELETE /api/v1/schedules/{id}
func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, schedule.ErrNotFound) {
			writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PATCH /api/v1/schedules/{id}/assignments/{idx}
func (h *ScheduleHandler) PatchAssignment(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schedule id")
		return
	}

	idxStr := mux.Vars(r)["idx"]
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 {
		writeError(w, http.StatusBadRequest, "invalid assignment index")
		return
	}

	var req PatchAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	updated, err := h.svc.PatchAssignment(r.Context(), id, idx, generator.PatchRequest{
		TimeSlot: req.TimeSlot,
		RoomID:   req.RoomID,
		Parity:   schedule.Parity(req.Parity),
	})
	if err != nil {
		var conflict *generator.ConflictError
		if errors.As(err, &conflict) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(conflict)
			return
		}
		if errors.Is(err, schedule.ErrNotFound) {
			writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// --- helpers ---

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
