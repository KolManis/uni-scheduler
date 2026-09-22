package web

import (
	"net/http"
	"strconv"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/google/uuid"
)

type groupsListData struct {
	Groups    []domain.Group
	Buildings []domain.Building
	Error     string
}

type groupFormData struct {
	Group     domain.Group
	Buildings []domain.Building
	IsEdit    bool
	Error     string
}

func (h *Handler) loadGroupsList(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["groups_list.html"], groupsListData{
		Groups: data.Groups, Buildings: data.Buildings, Error: errMsg,
	})
}

func (h *Handler) groupsList(w http.ResponseWriter, r *http.Request) {
	h.loadGroupsList(w, r, http.StatusOK, "")
}

func (h *Handler) groupsForm(w http.ResponseWriter, r *http.Request) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fd := groupFormData{Buildings: data.Buildings}
	if id := idFromPath(r); id != "" {
		found := false
		for _, g := range data.Groups {
			if g.ID == id {
				fd.Group = g
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
	render(w, r, h.pages["groups_form.html"], fd)
}

func groupFromForm(r *http.Request) domain.Group {
	count, _ := strconv.Atoi(r.FormValue("student_count"))
	return domain.Group{
		Name:         r.FormValue("name"),
		StudentCount: count,
		BuildingIDs:  formStrings(r, "building_ids"),
	}
}

func (h *Handler) reGroupsForm(w http.ResponseWriter, r *http.Request, status int, g domain.Group, isEdit bool, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["groups_form.html"], groupFormData{
		Group: g, Buildings: data.Buildings, IsEdit: isEdit, Error: errMsg,
	})
}

func (h *Handler) groupsCreate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	g := groupFromForm(r)
	if g.Name == "" {
		h.reGroupsForm(w, r, http.StatusBadRequest, g, false, "Название группы обязательно")
		return
	}
	g.ID = uuid.NewString()
	if err := h.ref.InsertGroup(r.Context(), g); err != nil {
		h.reGroupsForm(w, r, http.StatusInternalServerError, g, false, err.Error())
		return
	}
	h.loadGroupsList(w, r, http.StatusOK, "")
}

func (h *Handler) groupsUpdate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	g := groupFromForm(r)
	g.ID = idFromPath(r)
	if g.Name == "" {
		h.reGroupsForm(w, r, http.StatusBadRequest, g, true, "Название группы обязательно")
		return
	}
	ok, err := h.ref.UpdateGroup(r.Context(), g)
	if err != nil {
		h.reGroupsForm(w, r, http.StatusInternalServerError, g, true, err.Error())
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.loadGroupsList(w, r, http.StatusOK, "")
}

func (h *Handler) groupsDelete(w http.ResponseWriter, r *http.Request) {
	ok, err := h.ref.DeleteGroup(r.Context(), idFromPath(r))
	if err != nil {
		h.loadGroupsList(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		h.loadGroupsList(w, r, http.StatusNotFound, "Группа не найдена")
		return
	}
	h.loadGroupsList(w, r, http.StatusOK, "")
}
