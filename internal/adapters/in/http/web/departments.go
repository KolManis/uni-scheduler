package web

import (
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/google/uuid"
)

type departmentsListData struct {
	Departments []domain.Department
	Error       string
}

type departmentFormData struct {
	Department domain.Department
	IsEdit     bool
	Error      string
}

func (h *Handler) loadDepartmentsList(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["departments_list.html"], departmentsListData{
		Departments: data.Departments,
		Error:       errMsg,
	})
}

func (h *Handler) departmentsList(w http.ResponseWriter, r *http.Request) {
	h.loadDepartmentsList(w, r, http.StatusOK, "")
}

func (h *Handler) departmentsForm(w http.ResponseWriter, r *http.Request) {
	fd := departmentFormData{}
	if id := idFromPath(r); id != "" {
		data, err := h.input.LoadInput(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		found := false
		for _, d := range data.Departments {
			if d.ID == id {
				fd.Department = d
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
	render(w, r, h.pages["departments_form.html"], fd)
}

func (h *Handler) departmentsCreate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	d := domain.Department{Name: r.FormValue("name")}
	if d.Name == "" {
		renderStatus(w, r, http.StatusBadRequest, h.pages["departments_form.html"], departmentFormData{Department: d, Error: "Название обязательно"})
		return
	}
	d.ID = uuid.NewString()
	if err := h.ref.InsertDepartment(r.Context(), d); err != nil {
		renderStatus(w, r, http.StatusInternalServerError, h.pages["departments_form.html"], departmentFormData{Department: d, Error: err.Error()})
		return
	}
	h.loadDepartmentsList(w, r, http.StatusOK, "")
}

func (h *Handler) departmentsUpdate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	d := domain.Department{ID: idFromPath(r), Name: r.FormValue("name")}
	if d.Name == "" {
		renderStatus(w, r, http.StatusBadRequest, h.pages["departments_form.html"], departmentFormData{Department: d, IsEdit: true, Error: "Название обязательно"})
		return
	}
	ok, err := h.ref.UpdateDepartment(r.Context(), d)
	if err != nil {
		renderStatus(w, r, http.StatusInternalServerError, h.pages["departments_form.html"], departmentFormData{Department: d, IsEdit: true, Error: err.Error()})
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.loadDepartmentsList(w, r, http.StatusOK, "")
}

func (h *Handler) departmentsDelete(w http.ResponseWriter, r *http.Request) {
	ok, err := h.ref.DeleteDepartment(r.Context(), idFromPath(r))
	if err != nil {
		h.loadDepartmentsList(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		h.loadDepartmentsList(w, r, http.StatusNotFound, "Кафедра не найдена")
		return
	}
	h.loadDepartmentsList(w, r, http.StatusOK, "")
}
