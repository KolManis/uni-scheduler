package rest

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/xuri/excelize/v2"

	"github.com/KolManis/uni-scheduler/internal/core/application/queries/getschedule"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
)

// ExcelHandler — выгрузка расписания в Excel: запрос getschedule плюс справочники для подписей.
type ExcelHandler struct {
	getSchedule *getschedule.Handler
	inputRepo   ports.InputRepository
}

func NewExcelHandler(getSchedule *getschedule.Handler, inputRepo ports.InputRepository) *ExcelHandler {
	return &ExcelHandler{getSchedule: getSchedule, inputRepo: inputRepo}
}

// Export — GET /api/v1/schedules/{id}/excel?type=…
//
//	type=all_teachers           один лист, колонка на каждого преподавателя (с парами на других факультетах)
//	type=all_groups             один лист, колонка на каждую группу
//	type=year&year=2024         лист на каждую группу этого года набора
//	type=teacher&id=… / group   один лист на одного преподавателя или группу (по умолчанию group)
//	week=even|odd               только чётная или нечётная неделя; по умолчанию обе
//
// Все виды рисует writeTimetable (excel_sheet.go): различается только набор колонок.
func (h *ExcelHandler) Export(w http.ResponseWriter, r *http.Request) {
	scheduleID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || scheduleID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid schedule id")
		return
	}
	params := r.URL.Query()
	viewType := params.Get("type")
	if viewType == "" {
		viewType = "group"
	}

	q, err := getschedule.NewQuery(scheduleID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	sched, err := h.getSchedule.Handle(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	input, err := h.inputRepo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot load input data")
		return
	}

	names := newExcelNames(*input)
	pairs := inWeek(sched.Assignments, params.Get("week"))
	pairsWithExternal := inWeek(withExternalPairs(sched.Assignments, input.Teachers), params.Get("week"))
	f := excelize.NewFile()
	defer f.Close()

	var filename string
	switch viewType {
	case "all_teachers":
		var cols []timetableColumn
		for _, t := range sortedTeachers(input.Teachers) {
			cols = append(cols, columnOf(t.Name, pairsWithExternal, byTeacher(t.ID)))
		}
		writeTimetable(f, "Все преподаватели", cols, wideSheet, names)
		filename = fmt.Sprintf("teachers_schedule_%d.xlsx", scheduleID)

	case "all_groups":
		var cols []timetableColumn
		for _, g := range sortedGroups(input.Groups) {
			cols = append(cols, columnOf(g.Name, pairs, byGroup(g.ID)))
		}
		writeTimetable(f, "Все группы", cols, wideSheet, names)
		filename = fmt.Sprintf("groups_schedule_%d.xlsx", scheduleID)

	case "year":
		year := params.Get("year")
		if year == "" {
			writeError(w, http.StatusBadRequest, "year param is required")
			return
		}
		groups := groupsOfYear(input.Groups, year)
		if len(groups) == 0 {
			writeError(w, http.StatusNotFound, "no groups found for year "+year)
			return
		}
		for _, g := range groups {
			writeTimetable(f, g.ID, []timetableColumn{columnOf("Предмет", pairs, byGroup(g.ID))}, singleSheet, names)
		}
		filename = fmt.Sprintf("schedule_%d_year%s.xlsx", scheduleID, year)

	default: // group или teacher
		id := params.Get("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "entity id is required for group/teacher view")
			return
		}
		sheet, col := names.groups[id], columnOf("Предмет", pairs, byGroup(id))
		if viewType != "group" {
			sheet, col = names.teachers[id], columnOf("Предмет", pairsWithExternal, byTeacher(id))
		}
		if sheet == "" {
			sheet = id
		}
		writeTimetable(f, sheet, []timetableColumn{col}, singleSheet, names)
		filename = fmt.Sprintf("%s_schedule_%d.xlsx", sheet, scheduleID)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	f.Write(w)
}

// inWeek — пары, которые идут в неделю week ("even" или "odd"); иначе — все.
func inWeek(assignments []domain.Assignment, week string) []domain.Assignment {
	if week != "even" && week != "odd" {
		return assignments
	}
	var out []domain.Assignment
	for _, a := range assignments {
		if a.Parity == domain.Always || a.Parity == domain.Parity(week) {
			out = append(out, a)
		}
	}
	return out
}

func byTeacher(id string) func(domain.Assignment) bool {
	return func(a domain.Assignment) bool { return a.TeacherID == id }
}

func byGroup(id string) func(domain.Assignment) bool {
	return func(a domain.Assignment) bool {
		for _, g := range a.GroupIDs {
			if g == id {
				return true
			}
		}
		return false
	}
}

func sortedTeachers(teachers []domain.Teacher) []domain.Teacher {
	out := append([]domain.Teacher(nil), teachers...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sortedGroups(groups []domain.Group) []domain.Group {
	out := append([]domain.Group(nil), groups...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// groupsOfYear — группы года набора year (по имени вида «ИВТ-24-1б» → 2024), по имени.
func groupsOfYear(groups []domain.Group, year string) []domain.Group {
	var out []domain.Group
	for _, g := range sortedGroups(groups) {
		if m := yearRe.FindStringSubmatch(g.Name); m != nil && "20"+m[1] == year {
			out = append(out, g)
		}
	}
	return out
}

// externalPairType — вид «занятия» для пары преподавателя на другом факультете в выгрузке;
// в SubjectID такой записи лежит пометка пары.
const externalPairType domain.ClassType = "external"

// withExternalPairs — пары расписания плюс пары преподавателей на других факультетах
// в виде записей externalPairType (пометка — в SubjectID).
func withExternalPairs(assignments []domain.Assignment, teachers []domain.Teacher) []domain.Assignment {
	cells := append([]domain.Assignment(nil), assignments...)
	for _, t := range teachers {
		for _, ep := range t.ExternalPairs {
			p := ep.Parity
			if p == "" {
				p = domain.Always
			}
			cells = append(cells, domain.Assignment{
				TeacherID: t.ID, SubjectID: ep.Note, Type: externalPairType, TimeSlot: ep.TimeSlot, Parity: p,
			})
		}
	}
	return cells
}
