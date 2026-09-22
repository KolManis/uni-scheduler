package web

import (
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/google/uuid"
)

type buildingsListData struct {
	Buildings []domain.Building
	Error     string
}

type buildingFormData struct {
	Building domain.Building
	IsEdit   bool
	Error    string
}

func (h *Handler) loadBuildingsList(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["buildings_list.html"], buildingsListData{
		Buildings: data.Buildings,
		Error:     errMsg,
	})
}

func (h *Handler) buildingsList(w http.ResponseWriter, r *http.Request) {
	h.loadBuildingsList(w, r, http.StatusOK, "")
}

func (h *Handler) buildingsForm(w http.ResponseWriter, r *http.Request) {
	fd := buildingFormData{}
	if id := idFromPath(r); id != "" {
		data, err := h.input.LoadInput(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		found := false
		for _, b := range data.Buildings {
			if b.ID == id {
				fd.Building = b
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
	render(w, r, h.pages["buildings_form.html"], fd)
}

func (h *Handler) buildingsCreate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	b := domain.Building{
		Name:    r.FormValue("name"),
		Address: r.FormValue("address"),
	}
	if b.Name == "" {
		renderStatus(w, r, http.StatusBadRequest, h.pages["buildings_form.html"], buildingFormData{Building: b, Error: "Название обязательно"})
		return
	}
	b.ID = uuid.NewString()
	if err := h.ref.InsertBuilding(r.Context(), b); err != nil {
		renderStatus(w, r, http.StatusInternalServerError, h.pages["buildings_form.html"], buildingFormData{Building: b, Error: err.Error()})
		return
	}
	h.loadBuildingsList(w, r, http.StatusOK, "")
}

func (h *Handler) buildingsUpdate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	b := domain.Building{
		ID:      idFromPath(r),
		Name:    r.FormValue("name"),
		Address: r.FormValue("address"),
	}
	if b.Name == "" {
		renderStatus(w, r, http.StatusBadRequest, h.pages["buildings_form.html"], buildingFormData{Building: b, IsEdit: true, Error: "Название обязательно"})
		return
	}
	ok, err := h.ref.UpdateBuilding(r.Context(), b)
	if err != nil {
		renderStatus(w, r, http.StatusInternalServerError, h.pages["buildings_form.html"], buildingFormData{Building: b, IsEdit: true, Error: err.Error()})
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.loadBuildingsList(w, r, http.StatusOK, "")
}

func (h *Handler) buildingsDelete(w http.ResponseWriter, r *http.Request) {
	ok, err := h.ref.DeleteBuilding(r.Context(), idFromPath(r))
	if err != nil {
		h.loadBuildingsList(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		h.loadBuildingsList(w, r, http.StatusNotFound, "Корпус не найден")
		return
	}
	h.loadBuildingsList(w, r, http.StatusOK, "")
}
