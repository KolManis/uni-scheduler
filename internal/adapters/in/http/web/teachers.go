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
	Teacher      domain.Teacher
	ExternalRows []externalRow
	Departments  []domain.Department
	Buildings    []domain.Building
	Days         []domain.Day
	Pairs        []int
	IsEdit       bool
	Error        string
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
	fd.ExternalRows = externalRows(fd.Teacher.ExternalPairs)
	render(w, r, h.pages["teachers_form.html"], fd)
}

// externalBlankRows — сколько пустых строк для новых пар на других факультетах.
const externalBlankRows = 4

// externalRow — строка таблицы «Пары на других факультетах» в форме преподавателя.
type externalRow struct {
	Set    bool
	Day    domain.Day
	Pair   int
	Parity domain.Parity
	Note   string
}

func externalRows(pairs []domain.ExternalPair) []externalRow {
	rows := make([]externalRow, 0, len(pairs)+externalBlankRows)
	for _, p := range pairs {
		rows = append(rows, externalRow{Set: true, Day: p.TimeSlot.Day(), Pair: p.TimeSlot.PairNum(), Parity: p.Parity, Note: p.Note})
	}
	for k := 0; k < externalBlankRows; k++ {
		rows = append(rows, externalRow{Parity: domain.Always})
	}
	return rows
}

// parseExternalPairs собирает строки таблицы «Пары на других факультетах». Строка без
// дня пропускается; поля идут параллельными списками в порядке строк формы.
func parseExternalPairs(r *http.Request) []domain.ExternalPair {
	days, pairs := r.Form["ext_day"], r.Form["ext_pair"]
	parities, notes := r.Form["ext_parity"], r.Form["ext_note"]
	var out []domain.ExternalPair
	for k, d := range days {
		if d == "" || k >= len(pairs) {
			continue
		}
		n, err := strconv.Atoi(pairs[k])
		if err != nil {
			continue
		}
		slot, err := domain.NewTimeSlot(domain.Day(d), n)
		if err != nil {
			continue
		}
		p := domain.Always
		if k < len(parities) && domain.Parity(parities[k]).IsValid() {
			p = domain.Parity(parities[k])
		}
		note := ""
		if k < len(notes) {
			note = strings.TrimSpace(notes[k])
		}
		out = append(out, domain.ExternalPair{TimeSlot: slot, Parity: p, Note: note})
	}
	return out
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
		UndesiredSlots:     parseUnavailable(formStrings(r, "undesired")),
		PreferredBuildings: formStrings(r, "preferred_buildings"),
		ExternalPairs:      parseExternalPairs(r),
	}
}

func (h *Handler) reTeachersForm(w http.ResponseWriter, r *http.Request, status int, t domain.Teacher, isEdit bool, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["teachers_form.html"], teacherFormData{
		Teacher: t, ExternalRows: externalRows(t.ExternalPairs), Departments: data.Departments, Buildings: data.Buildings,
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
