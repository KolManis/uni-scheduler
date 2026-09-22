package web

import (
	"net/http"
	"strconv"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/google/uuid"
)

type subjectPlansListData struct {
	Plans       []domain.SubjectPlan
	Departments []domain.Department
	Teachers    []domain.Teacher
	Groups      []domain.Group
	Buildings   []domain.Building
	Error       string
}

type subjectPlanFormData struct {
	Plan        domain.SubjectPlan
	Departments []domain.Department
	Teachers    []domain.Teacher
	Groups      []domain.Group
	Buildings   []domain.Building
	RoomTypes   []string
	IsEdit      bool
	Error       string
}

func (h *Handler) loadSubjectPlansList(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["subject_plans_list.html"], subjectPlansListData{
		Plans: data.SubjectPlans, Departments: data.Departments, Teachers: data.Teachers,
		Groups: data.Groups, Buildings: data.Buildings, Error: errMsg,
	})
}

func (h *Handler) subjectPlansList(w http.ResponseWriter, r *http.Request) {
	h.loadSubjectPlansList(w, r, http.StatusOK, "")
}

func (h *Handler) subjectPlansForm(w http.ResponseWriter, r *http.Request) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fd := subjectPlanFormData{
		Departments: data.Departments, Teachers: data.Teachers, Groups: data.Groups,
		Buildings: data.Buildings, RoomTypes: distinctRoomTypes(data.Rooms),
	}
	fd.Plan.Parity = domain.Always
	fd.Plan.SemesterHalf = domain.HalfFull
	if id := idFromPath(r); id != "" {
		found := false
		for _, sp := range data.SubjectPlans {
			if sp.ID == id {
				fd.Plan = sp
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
	render(w, r, h.pages["subject_plans_form.html"], fd)
}

func subjectPlanFromForm(r *http.Request) domain.SubjectPlan {
	lecture, _ := strconv.Atoi(r.FormValue("lecture_hours"))
	practice, _ := strconv.Atoi(r.FormValue("practice_hours"))
	lab, _ := strconv.Atoi(r.FormValue("lab_hours"))
	return domain.SubjectPlan{
		Name:               r.FormValue("name"),
		DepartmentID:       r.FormValue("department_id"),
		LectureHours:       lecture,
		PracticeHours:      practice,
		LabHours:           lab,
		RequiresRoomType:   r.FormValue("requires_room_type"),
		RequiredBuildingID: r.FormValue("required_building_id"),
		TeacherID:          r.FormValue("teacher_id"),
		GroupIDs:           formStrings(r, "group_ids"),
		Parity:             domain.Parity(r.FormValue("parity")),
		SemesterHalf:       domain.SemesterHalf(r.FormValue("semester_half")),
	}
}

func (h *Handler) reSubjectPlansForm(w http.ResponseWriter, r *http.Request, status int, sp domain.SubjectPlan, isEdit bool, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["subject_plans_form.html"], subjectPlanFormData{
		Plan: sp, Departments: data.Departments, Teachers: data.Teachers, Groups: data.Groups,
		Buildings: data.Buildings, RoomTypes: distinctRoomTypes(data.Rooms), IsEdit: isEdit, Error: errMsg,
	})
}

func (h *Handler) subjectPlansCreate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	sp := subjectPlanFromForm(r)
	if sp.Name == "" || sp.TeacherID == "" || len(sp.GroupIDs) == 0 {
		h.reSubjectPlansForm(w, r, http.StatusBadRequest, sp, false, "Название, преподаватель и хотя бы одна группа обязательны")
		return
	}
	sp.ID = uuid.NewString()
	if err := h.ref.InsertSubjectPlan(r.Context(), sp); err != nil {
		h.reSubjectPlansForm(w, r, http.StatusInternalServerError, sp, false, err.Error())
		return
	}
	h.loadSubjectPlansList(w, r, http.StatusOK, "")
}

func (h *Handler) subjectPlansUpdate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	sp := subjectPlanFromForm(r)
	sp.ID = idFromPath(r)
	if sp.Name == "" || sp.TeacherID == "" || len(sp.GroupIDs) == 0 {
		h.reSubjectPlansForm(w, r, http.StatusBadRequest, sp, true, "Название, преподаватель и хотя бы одна группа обязательны")
		return
	}
	ok, err := h.ref.UpdateSubjectPlan(r.Context(), sp)
	if err != nil {
		h.reSubjectPlansForm(w, r, http.StatusInternalServerError, sp, true, err.Error())
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.loadSubjectPlansList(w, r, http.StatusOK, "")
}

func (h *Handler) subjectPlansDelete(w http.ResponseWriter, r *http.Request) {
	ok, err := h.ref.DeleteSubjectPlan(r.Context(), idFromPath(r))
	if err != nil {
		h.loadSubjectPlansList(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		h.loadSubjectPlansList(w, r, http.StatusNotFound, "Учебный план не найден")
		return
	}
	h.loadSubjectPlansList(w, r, http.StatusOK, "")
}
