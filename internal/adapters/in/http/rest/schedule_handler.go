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
	GenerateSchedule(ctx context.Context, cmd app.GenerateCommand) (*domain.Schedule, error)
	GetSchedule(ctx context.Context, id int64) (*domain.Schedule, error)
	ListSchedules(ctx context.Context) ([]domain.ScheduleSummary, error)
	DeleteSchedule(ctx context.Context, id int64) error
	MoveAssignment(ctx context.Context, cmd app.MoveAssignmentCommand) (*domain.Schedule, error)
	PinAssignment(ctx context.Context, cmd app.PinAssignmentCommand) (*domain.Schedule, error)
	CheckInput(ctx context.Context) ([]app.InputProblem, error)
	MoveOptions(ctx context.Context, schedID int64, idx int) ([]app.MoveOption, error)
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

	result, err := h.svc.GenerateSchedule(r.Context(), app.GenerateCommand{
		Name:           req.Name,
		MaxIterations:  req.MaxIterations,
		SolverType:     req.SolverType,
		TimeoutSec:     req.TimeoutSec,
		SemesterHalf:   req.SemesterHalf,
		ImproveAlgo:    req.ImproveAlgo,
		ParallelStarts: req.ParallelStarts,
		BaseScheduleID: req.BaseScheduleID,
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
	list, err := h.svc.ListSchedules(r.Context())
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

	result, err := h.svc.GetSchedule(r.Context(), id)
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

	if err := h.svc.DeleteSchedule(r.Context(), id); err != nil {
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

	updated, err := h.svc.MoveAssignment(r.Context(), app.MoveAssignmentCommand{
		ScheduleID: id,
		Index:      idx,
		TimeSlot:   req.TimeSlot,
		RoomID:     req.RoomID,
		Parity:     domain.Parity(req.Parity),
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

// parseIdx — номер пары в расписании из пути.
func parseIdx(r *http.Request) (int, bool) {
	idx, err := strconv.Atoi(mux.Vars(r)["idx"])
	return idx, err == nil && idx >= 0
}

func (h *ScheduleHandler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "schedule not found")
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// GET /api/v1/schedules/{id}/assignments/{idx}/options — куда можно перенести пару.
func (h *ScheduleHandler) MoveOptions(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	idx, ok := parseIdx(r)
	if err != nil || !ok {
		writeError(w, http.StatusBadRequest, "invalid schedule id or assignment index")
		return
	}
	opts, err := h.svc.MoveOptions(r.Context(), id, idx)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(opts)
}

// PUT /api/v1/schedules/{id}/assignments/{idx}/pinned — {"pinned": true|false}.
func (h *ScheduleHandler) SetPinned(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	idx, ok := parseIdx(r)
	if err != nil || !ok {
		writeError(w, http.StatusBadRequest, "invalid schedule id or assignment index")
		return
	}
	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	updated, err := h.svc.PinAssignment(r.Context(), app.PinAssignmentCommand{ScheduleID: id, Index: idx, Pinned: req.Pinned})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// GET /api/v1/input/check — ошибки в справочниках и планах до генерации.
func (h *ScheduleHandler) CheckInput(w http.ResponseWriter, r *http.Request) {
	problems, err := h.svc.CheckInput(r.Context())
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	if problems == nil {
		problems = []app.InputProblem{} // [] вместо null
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(problems)
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
