package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type refWriter interface {
	InsertBuilding(ctx context.Context, b schedule.Building) error
	UpdateBuilding(ctx context.Context, b schedule.Building) (bool, error)
	DeleteBuilding(ctx context.Context, id string) (bool, error)

	InsertDepartment(ctx context.Context, d schedule.Department) error
	UpdateDepartment(ctx context.Context, d schedule.Department) (bool, error)
	DeleteDepartment(ctx context.Context, id string) (bool, error)

	InsertRoom(ctx context.Context, rm schedule.Room) error
	UpdateRoom(ctx context.Context, rm schedule.Room) (bool, error)
	DeleteRoom(ctx context.Context, id string) (bool, error)

	InsertGroup(ctx context.Context, g schedule.Group) error
	UpdateGroup(ctx context.Context, g schedule.Group) (bool, error)
	DeleteGroup(ctx context.Context, id string) (bool, error)

	InsertTeacher(ctx context.Context, t schedule.Teacher) error
	UpdateTeacher(ctx context.Context, t schedule.Teacher) (bool, error)
	DeleteTeacher(ctx context.Context, id string) (bool, error)

	InsertSubjectPlan(ctx context.Context, sp schedule.SubjectPlan) error
	UpdateSubjectPlan(ctx context.Context, sp schedule.SubjectPlan) (bool, error)
	DeleteSubjectPlan(ctx context.Context, id string) (bool, error)
}

type RefWriteHandler struct {
	repo refWriter
}

func NewRefWriteHandler(repo refWriter) *RefWriteHandler {
	return &RefWriteHandler{repo: repo}
}

func newID() string { return uuid.NewString() }

func decodeBody[T any](r *http.Request) (T, error) {
	var v T
	err := json.NewDecoder(r.Body).Decode(&v)
	return v, err
}

func writeCreated(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(v)
}

func writeNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "not found")
}

func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// ── Buildings ────────────────────────────────────────────────────────────────

func (h *RefWriteHandler) CreateBuilding(w http.ResponseWriter, r *http.Request) {
	b, err := decodeBody[schedule.Building](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b.ID = newID()
	if err := h.repo.InsertBuilding(r.Context(), b); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCreated(w, b)
}

func (h *RefWriteHandler) UpdateBuilding(w http.ResponseWriter, r *http.Request) {
	b, err := decodeBody[schedule.Building](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b.ID = mux.Vars(r)["id"]
	ok, err := h.repo.UpdateBuilding(r.Context(), b)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, b)
}

func (h *RefWriteHandler) DeleteBuilding(w http.ResponseWriter, r *http.Request) {
	ok, err := h.repo.DeleteBuilding(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeNoContent(w)
}

// ── Departments ───────────────────────────────────────────────────────────────

func (h *RefWriteHandler) CreateDepartment(w http.ResponseWriter, r *http.Request) {
	d, err := decodeBody[schedule.Department](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	d.ID = newID()
	if err := h.repo.InsertDepartment(r.Context(), d); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCreated(w, d)
}

func (h *RefWriteHandler) UpdateDepartment(w http.ResponseWriter, r *http.Request) {
	d, err := decodeBody[schedule.Department](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	d.ID = mux.Vars(r)["id"]
	ok, err := h.repo.UpdateDepartment(r.Context(), d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, d)
}

func (h *RefWriteHandler) DeleteDepartment(w http.ResponseWriter, r *http.Request) {
	ok, err := h.repo.DeleteDepartment(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeNoContent(w)
}

// ── Rooms ─────────────────────────────────────────────────────────────────────

func (h *RefWriteHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	rm, err := decodeBody[schedule.Room](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rm.ID = newID()
	if err := h.repo.InsertRoom(r.Context(), rm); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCreated(w, rm)
}

func (h *RefWriteHandler) UpdateRoom(w http.ResponseWriter, r *http.Request) {
	rm, err := decodeBody[schedule.Room](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rm.ID = mux.Vars(r)["id"]
	ok, err := h.repo.UpdateRoom(r.Context(), rm)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, rm)
}

func (h *RefWriteHandler) DeleteRoom(w http.ResponseWriter, r *http.Request) {
	ok, err := h.repo.DeleteRoom(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeNoContent(w)
}

// ── Groups ────────────────────────────────────────────────────────────────────

func (h *RefWriteHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	g, err := decodeBody[schedule.Group](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	g.ID = newID()
	if err := h.repo.InsertGroup(r.Context(), g); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCreated(w, g)
}

func (h *RefWriteHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	g, err := decodeBody[schedule.Group](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	g.ID = mux.Vars(r)["id"]
	ok, err := h.repo.UpdateGroup(r.Context(), g)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, g)
}

func (h *RefWriteHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	ok, err := h.repo.DeleteGroup(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeNoContent(w)
}

// ── Teachers ──────────────────────────────────────────────────────────────────

func (h *RefWriteHandler) CreateTeacher(w http.ResponseWriter, r *http.Request) {
	t, err := decodeBody[schedule.Teacher](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	t.ID = newID()
	if err := h.repo.InsertTeacher(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCreated(w, t)
}

func (h *RefWriteHandler) UpdateTeacher(w http.ResponseWriter, r *http.Request) {
	t, err := decodeBody[schedule.Teacher](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	t.ID = mux.Vars(r)["id"]
	ok, err := h.repo.UpdateTeacher(r.Context(), t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, t)
}

func (h *RefWriteHandler) DeleteTeacher(w http.ResponseWriter, r *http.Request) {
	ok, err := h.repo.DeleteTeacher(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeNoContent(w)
}

// ── SubjectPlans ──────────────────────────────────────────────────────────────

func (h *RefWriteHandler) CreateSubjectPlan(w http.ResponseWriter, r *http.Request) {
	sp, err := decodeBody[schedule.SubjectPlan](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sp.ID = newID()
	if err := h.repo.InsertSubjectPlan(r.Context(), sp); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCreated(w, sp)
}

func (h *RefWriteHandler) UpdateSubjectPlan(w http.ResponseWriter, r *http.Request) {
	sp, err := decodeBody[schedule.SubjectPlan](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sp.ID = mux.Vars(r)["id"]
	ok, err := h.repo.UpdateSubjectPlan(r.Context(), sp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, sp)
}

func (h *RefWriteHandler) DeleteSubjectPlan(w http.ResponseWriter, r *http.Request) {
	ok, err := h.repo.DeleteSubjectPlan(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeNotFound(w)
		return
	}
	writeNoContent(w)
}
