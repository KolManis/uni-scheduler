package rest

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/KolManis/uni-scheduler/internal/core/application"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/deleteschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/generateschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/moveassignment"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/pinassignment"
	"github.com/KolManis/uni-scheduler/internal/core/application/generation"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/checkinput"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/getschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/moveoptions"
	"github.com/KolManis/uni-scheduler/internal/core/application/rules"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/gorilla/mux"
)

// ScheduleHandler — переводчик HTTP ↔ сценарии работы с расписаниями. Каждый метод:
// разобрать запрос → собрать команду или запрос → вызвать обработчик → ответить.
type ScheduleHandler struct {
	uc application.UseCases
}

func NewScheduleHandler(uc application.UseCases) *ScheduleHandler {
	return &ScheduleHandler{uc: uc}
}

// POST /api/v1/schedules/generate — 201 и расписание целиком.
func (h *ScheduleHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	cmd, err := generateschedule.NewCommand(generation.Request{
		Name:           req.Name,
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
		writeServiceError(w, err)
		return
	}

	result, err := h.uc.GenerateSchedule.Handle(r.Context(), cmd)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, result)
}

// GET /api/v1/schedules — список без пар, свежие сверху.
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.uc.ListSchedules.Handle(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if list == nil {
		list = []domain.ScheduleSummary{} // [] вместо null
	}
	writeJSONStatus(w, http.StatusOK, list)
}

// GET /api/v1/schedules/{id}
func (h *ScheduleHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	q, err := getschedule.NewQuery(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result, err := h.uc.GetSchedule.Handle(r.Context(), q)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusOK, result)
}

// DELETE /api/v1/schedules/{id} — 204.
func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	cmd, err := deleteschedule.NewCommand(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if err := h.uc.DeleteSchedule.Handle(r.Context(), cmd); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PATCH /api/v1/schedules/{id}/assignments/{idx} — перенести пару; 409 — конфликт.
func (h *ScheduleHandler) PatchAssignment(w http.ResponseWriter, r *http.Request) {
	id, idx, ok := parseAssignment(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid schedule id or assignment index")
		return
	}
	var req PatchAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	cmd, err := moveassignment.NewCommand(id, idx, req.TimeSlot, req.RoomID, domain.Parity(req.Parity))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	updated, err := h.uc.MoveAssignment.Handle(r.Context(), cmd)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusOK, updated)
}

// GET /api/v1/schedules/{id}/assignments/{idx}/options — куда можно перенести пару.
func (h *ScheduleHandler) MoveOptions(w http.ResponseWriter, r *http.Request) {
	id, idx, ok := parseAssignment(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid schedule id or assignment index")
		return
	}
	q, err := moveoptions.NewQuery(id, idx)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	opts, err := h.uc.MoveOptions.Handle(r.Context(), q)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusOK, opts)
}

// PUT /api/v1/schedules/{id}/assignments/{idx}/pinned — {"pinned": true|false}.
func (h *ScheduleHandler) SetPinned(w http.ResponseWriter, r *http.Request) {
	id, idx, ok := parseAssignment(r)
	if !ok {
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
	cmd, err := pinassignment.NewCommand(id, idx, req.Pinned)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	updated, err := h.uc.PinAssignment.Handle(r.Context(), cmd)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusOK, updated)
}

// GET /api/v1/input/check — ошибки в справочниках и планах до генерации.
func (h *ScheduleHandler) CheckInput(w http.ResponseWriter, r *http.Request) {
	problems, err := h.uc.CheckInput.Handle(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if problems == nil {
		problems = []checkinput.Problem{} // [] вместо null
	}
	writeJSONStatus(w, http.StatusOK, problems)
}

// --- helpers ---

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
}

// parseAssignment — номер расписания и номер пары из пути.
func parseAssignment(r *http.Request) (int64, int, bool) {
	id, err := parseID(r)
	if err != nil {
		return 0, 0, false
	}
	idx, err := strconv.Atoi(mux.Vars(r)["idx"])
	if err != nil || idx < 0 {
		return 0, 0, false
	}
	return id, idx, true
}

// writeServiceError переводит ошибку сценария в HTTP-статус.
func writeServiceError(w http.ResponseWriter, err error) {
	var conflict *rules.ConflictError
	switch {
	case errors.As(err, &conflict):
		writeJSONStatus(w, http.StatusConflict, conflict)
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "schedule not found")
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrNoSolution):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSONStatus(w, status, map[string]string{"error": message})
}
