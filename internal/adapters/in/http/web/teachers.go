package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/google/uuid"
)

type teachersListData struct {
	Teachers    []domain.Teacher
	Departments []domain.Department
	Buildings   []domain.Building
	Error       string
}

type teacherFormData struct {
	Teacher     domain.Teacher
	Departments []domain.Department
	Buildings   []domain.Building
	Days        []domain.Day
	Pairs       []int
	IsEdit      bool
	Error       string
}

func (h *Handler) loadTeachersList(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["teachers_list.html"], teachersListData{
		Teachers: data.Teachers, Departments: data.Departments, Buildings: data.Buildings, Error: errMsg,
	})
}

func (h *Handler) teachersList(w http.ResponseWriter, r *http.Request) {
	h.loadTeachersList(w, r, http.StatusOK, "")
}

func (h *Handler) teachersForm(w http.ResponseWriter, r *http.Request) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fd := teacherFormData{Departments: data.Departments, Buildings: data.Buildings, Days: allDays, Pairs: allPairs}
	fd.Teacher.MaxWeeklyHours = 20
	if id := idFromPath(r); id != "" {
		found := false
		for _, t := range data.Teachers {
			if t.ID == id {
				fd.Teacher = t
				found = true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		fd.IsEdit = true
	}
	render(w, r, h.pages["teachers_form.html"], fd)
}

// parseUnavailable разбирает значения чекбоксов вида "monday:3" в TimeSlot.
func parseUnavailable(values []string) []domain.TimeSlot {
	var slots []domain.TimeSlot
	for _, v := range values {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) != 2 {
			continue
		}
		pair, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		slot, err := domain.NewTimeSlot(domain.Day(parts[0]), pair)
		if err != nil {
			continue
		}
		slots = append(slots, slot)
	}
	return slots
}

func teacherFromForm(r *http.Request) domain.Teacher {
	hours, _ := strconv.Atoi(r.FormValue("max_weekly_hours"))
	return domain.Teacher{
		Name:               r.FormValue("name"),
		DepartmentID:       r.FormValue("department_id"),
		MaxWeeklyHours:     hours,
		UnavailableSlots:   parseUnavailable(formStrings(r, "unavailable")),
		PreferredBuildings: formStrings(r, "preferred_buildings"),
	}
}

func (h *Handler) reTeachersForm(w http.ResponseWriter, r *http.Request, status int, t domain.Teacher, isEdit bool, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["teachers_form.html"], teacherFormData{
		Teacher: t, Departments: data.Departments, Buildings: data.Buildings,
		Days: allDays, Pairs: allPairs, IsEdit: isEdit, Error: errMsg,
	})
}

func (h *Handler) teachersCreate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	t := teacherFromForm(r)
	if t.Name == "" || t.DepartmentID == "" {
		h.reTeachersForm(w, r, http.StatusBadRequest, t, false, "Имя и кафедра обязательны")
		return
	}
	t.ID = uuid.NewString()
	if err := h.ref.InsertTeacher(r.Context(), t); err != nil {
		h.reTeachersForm(w, r, http.StatusInternalServerError, t, false, err.Error())
		return
	}
	h.loadTeachersList(w, r, http.StatusOK, "")
}

func (h *Handler) teachersUpdate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	t := teacherFromForm(r)
	t.ID = idFromPath(r)
	if t.Name == "" || t.DepartmentID == "" {
		h.reTeachersForm(w, r, http.StatusBadRequest, t, true, "Имя и кафедра обязательны")
		return
	}
	ok, err := h.ref.UpdateTeacher(r.Context(), t)
	if err != nil {
		h.reTeachersForm(w, r, http.StatusInternalServerError, t, true, err.Error())
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.loadTeachersList(w, r, http.StatusOK, "")
}

func (h *Handler) teachersDelete(w http.ResponseWriter, r *http.Request) {
	ok, err := h.ref.DeleteTeacher(r.Context(), idFromPath(r))
	if err != nil {
		h.loadTeachersList(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		h.loadTeachersList(w, r, http.StatusNotFound, "Преподаватель не найден")
		return
	}
	h.loadTeachersList(w, r, http.StatusOK, "")
}
