package web

import (
	"net/http"
	"strconv"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/google/uuid"
)

type roomsListData struct {
	Rooms     []domain.Room
	Buildings []domain.Building
	Error     string
}

type roomFormData struct {
	Room      domain.Room
	Buildings []domain.Building
	RoomTypes []string
	IsEdit    bool
	Error     string
}

func distinctRoomTypes(rooms []domain.Room) []string {
	seen := map[string]bool{"lecture": true, "lab": true, "computer": true}
	types := []string{"lecture", "lab", "computer"}
	for _, r := range rooms {
		if r.Type != "" && !seen[r.Type] {
			seen[r.Type] = true
			types = append(types, r.Type)
		}
	}
	return types
}

func (h *Handler) loadRoomsList(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["rooms_list.html"], roomsListData{
		Rooms:     data.Rooms,
		Buildings: data.Buildings,
		Error:     errMsg,
	})
}

func (h *Handler) roomsList(w http.ResponseWriter, r *http.Request) {
	h.loadRoomsList(w, r, http.StatusOK, "")
}

func (h *Handler) roomsForm(w http.ResponseWriter, r *http.Request) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fd := roomFormData{Buildings: data.Buildings, RoomTypes: distinctRoomTypes(data.Rooms)}
	if id := idFromPath(r); id != "" {
		found := false
		for _, rm := range data.Rooms {
			if rm.ID == id {
				fd.Room = rm
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
	render(w, r, h.pages["rooms_form.html"], fd)
}

func roomFromForm(r *http.Request) domain.Room {
	capacity, _ := strconv.Atoi(r.FormValue("capacity"))
	return domain.Room{
		Number:     r.FormValue("number"),
		BuildingID: r.FormValue("building_id"),
		Capacity:   capacity,
		Type:       r.FormValue("type"),
	}
}

func (h *Handler) reRoomsForm(w http.ResponseWriter, r *http.Request, status int, rm domain.Room, isEdit bool, errMsg string) {
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["rooms_form.html"], roomFormData{
		Room: rm, Buildings: data.Buildings, RoomTypes: distinctRoomTypes(data.Rooms), IsEdit: isEdit, Error: errMsg,
	})
}

func (h *Handler) roomsCreate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	rm := roomFromForm(r)
	if rm.Number == "" || rm.BuildingID == "" || rm.Type == "" {
		h.reRoomsForm(w, r, http.StatusBadRequest, rm, false, "Номер, корпус и тип аудитории обязательны")
		return
	}
	rm.ID = uuid.NewString()
	if err := h.ref.InsertRoom(r.Context(), rm); err != nil {
		h.reRoomsForm(w, r, http.StatusInternalServerError, rm, false, err.Error())
		return
	}
	h.loadRoomsList(w, r, http.StatusOK, "")
}

func (h *Handler) roomsUpdate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	rm := roomFromForm(r)
	rm.ID = idFromPath(r)
	if rm.Number == "" || rm.BuildingID == "" || rm.Type == "" {
		h.reRoomsForm(w, r, http.StatusBadRequest, rm, true, "Номер, корпус и тип аудитории обязательны")
		return
	}
	ok, err := h.ref.UpdateRoom(r.Context(), rm)
	if err != nil {
		h.reRoomsForm(w, r, http.StatusInternalServerError, rm, true, err.Error())
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.loadRoomsList(w, r, http.StatusOK, "")
}

func (h *Handler) roomsDelete(w http.ResponseWriter, r *http.Request) {
	ok, err := h.ref.DeleteRoom(r.Context(), idFromPath(r))
	if err != nil {
		h.loadRoomsList(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		h.loadRoomsList(w, r, http.StatusNotFound, "Аудитория не найдена")
		return
	}
	h.loadRoomsList(w, r, http.StatusOK, "")
}
