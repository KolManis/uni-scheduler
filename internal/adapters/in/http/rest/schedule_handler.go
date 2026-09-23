package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/KolManis/uni-scheduler/internal/core/app"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/gorilla/mux"
)

// scheduleService — интерфейс для работы с расписаниями.
type scheduleService interface {
	Generate(ctx context.Context, in app.GenerateInput) (*domain.Schedule, error)
	GetByID(ctx context.Context, id int64) (*domain.Schedule, error)
	List(ctx context.Context) ([]domain.ScheduleSummary, error)
	Delete(ctx context.Context, id int64) error
	PatchAssignment(ctx context.Context, schedID int64, idx int, req app.PatchRequest) (*domain.Schedule, error)
	ImportExcel(ctx context.Context, data *domain.ImportedData) (*domain.ImportResult, error)
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

	result, err := h.svc.Generate(r.Context(), app.GenerateInput{
		Name:           req.Name,
		MaxIterations:  req.MaxIterations,
		SolverType:     req.SolverType,
		TimeoutSec:     req.TimeoutSec,
		SemesterHalf:   req.SemesterHalf,
		ImproveAlgo:    req.ImproveAlgo,
		ParallelStarts: req.ParallelStarts,
		Preferences: domain.SolverPreferences{
			LectureBeforePractice:  req.LectureBeforePractice,
			LecturePracticeSameDay: req.LecturePracticeSameDay,
			SameSubjectSameDay:     req.SameSubjectSameDay,
		},
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, domain.ErrNoSolution) {
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
		list = []domain.ScheduleSummary{}
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
		if errors.Is(err, domain.ErrNotFound) {
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
		if errors.Is(err, domain.ErrNotFound) {
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

	updated, err := h.svc.PatchAssignment(r.Context(), id, idx, app.PatchRequest{
		TimeSlot: req.TimeSlot,
		RoomID:   req.RoomID,
		Parity:   domain.Parity(req.Parity),
	})
	if err != nil {
		var conflict *app.ConflictError
		if errors.As(err, &conflict) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(conflict)
			return
		}
		if errors.Is(err, domain.ErrNotFound) {
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
