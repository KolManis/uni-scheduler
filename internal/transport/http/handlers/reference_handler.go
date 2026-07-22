package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

var yearRe = regexp.MustCompile(`^[^-]+-(\d{2})-`)

type referenceInput interface {
	LoadInput(ctx context.Context) (*schedule.InputData, error)
}

// ReferenceHandler возвращает данные справочников.
type ReferenceHandler struct {
	repo referenceInput
}

func NewReferenceHandler(repo referenceInput) *ReferenceHandler {
	return &ReferenceHandler{repo: repo}
}

func (h *ReferenceHandler) GetTeachers(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.Teachers)
}

func (h *ReferenceHandler) GetGroups(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.Groups)
}

func (h *ReferenceHandler) GetRooms(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.Rooms)
}

func (h *ReferenceHandler) GetSubjectPlans(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.SubjectPlans)
}

func (h *ReferenceHandler) GetGroupsByYear(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	byYear := map[string][]schedule.Group{}
	for _, g := range data.Groups {
		m := yearRe.FindStringSubmatch(g.Name)
		year := "unknown"
		if m != nil {
			year = "20" + m[1]
		}
		byYear[year] = append(byYear[year], g)
	}

	years := make([]string, 0, len(byYear))
	for y := range byYear {
		years = append(years, y)
	}
	sort.Strings(years)

	type yearEntry struct {
		Year   string           `json:"year"`
		Groups []schedule.Group `json:"groups"`
	}
	result := make([]yearEntry, 0, len(years))
	for _, y := range years {
		gs := byYear[y]
		sort.Slice(gs, func(i, j int) bool { return gs[i].Name < gs[j].Name })
		result = append(result, yearEntry{Year: y, Groups: gs})
	}

	writeJSON(w, result)
}

func (h *ReferenceHandler) GetBuildings(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.Buildings)
}

func (h *ReferenceHandler) GetDepartments(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, data.Departments)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
