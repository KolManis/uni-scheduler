package web

import (
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/app"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

type schedulesListData struct {
	Schedules []domain.ScheduleSummary
	Error     string
	Success   string
}

// assignmentView добавляет к назначению его индекс в исходном срезе Schedule.Assignments —
// он же используется как idx в PATCH /schedules/{id}/assignments/{idx}.
type assignmentView struct {
	Idx int
	domain.Assignment
}

type pairGroup struct {
	PairNum int
	Items   []assignmentView
}

type dayGroup struct {
	Day   domain.Day
	Pairs []pairGroup
}

type scheduleViewData struct {
	Schedule     *domain.Schedule
	DayGroups    []dayGroup
	Breakdown    domain.FitnessBreakdown
	Quality      domain.WeekQuality
	Teachers     []domain.Teacher
	Rooms        []domain.Room
	Buildings    []domain.Building
	Groups       []domain.Group
	SubjectPlans []domain.SubjectPlan
	Error        string
	Success      string
}

// groupCell — одна ячейка расписания группы (день×пара). IsGap = true, если ячейка пустая,
// но лежит МЕЖДУ первой и последней парой этой группы в этот день — то есть настоящее окно,
// а не «ещё не приехал»/«уже уехал».
type groupCell struct {
	Items []assignmentView
	IsGap bool
}

type groupRow struct {
	PairNum int
	Cells   []groupCell // по одной на каждый день из allDays, в этом же порядке
}

type groupBlock struct {
	GroupID   string
	GroupName string
	Rows      []groupRow
}

type scheduleGroupsViewData struct {
	Schedule     *domain.Schedule
	Blocks       []groupBlock
	Days         []domain.Day
	Teachers     []domain.Teacher
	Rooms        []domain.Room
	Buildings    []domain.Building
	SubjectPlans []domain.SubjectPlan
}

func buildGroupBlocks(sched *domain.Schedule, groups []domain.Group) []groupBlock {
	type key struct {
		day  domain.Day
		pair int
	}
	byGroup := make(map[string]map[key][]assignmentView)
	for idx, a := range sched.Assignments {
		for _, gid := range a.GroupIDs {
			if byGroup[gid] == nil {
				byGroup[gid] = make(map[key][]assignmentView)
			}
			k := key{a.TimeSlot.Day(), a.TimeSlot.PairNum()}
			byGroup[gid][k] = append(byGroup[gid][k], assignmentView{Idx: idx, Assignment: a})
		}
	}

	sortedGroups := make([]domain.Group, len(groups))
	copy(sortedGroups, groups)
	sort.Slice(sortedGroups, func(i, j int) bool { return sortedGroups[i].Name < sortedGroups[j].Name })

	blocks := make([]groupBlock, 0, len(sortedGroups))
	for _, g := range sortedGroups {
		cells := byGroup[g.ID]

		dayMin := map[domain.Day]int{}
		dayMax := map[domain.Day]int{}
		for k := range cells {
			if cur, ok := dayMin[k.day]; !ok || k.pair < cur {
				dayMin[k.day] = k.pair
			}
			if cur, ok := dayMax[k.day]; !ok || k.pair > cur {
				dayMax[k.day] = k.pair
			}
		}

		rows := make([]groupRow, 0, len(allPairs))
		for _, pair := range allPairs {
			row := groupRow{PairNum: pair}
			for _, day := range allDays {
				items := cells[key{day, pair}]
				isGap := false
				if len(items) == 0 {
					min, hasMin := dayMin[day]
					max, hasMax := dayMax[day]
					isGap = hasMin && hasMax && pair > min && pair < max
				}
				row.Cells = append(row.Cells, groupCell{Items: items, IsGap: isGap})
			}
			rows = append(rows, row)
		}
		blocks = append(blocks, groupBlock{GroupID: g.ID, GroupName: g.Name, Rows: rows})
	}
	return blocks
}

func (h *Handler) schedulesGroupsView(w http.ResponseWriter, r *http.Request) {
	id, err := int64FromPath(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sched, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, r, h.pages["schedules_groups_view.html"], scheduleGroupsViewData{
		Schedule: sched, Blocks: buildGroupBlocks(sched, data.Groups), Days: allDays,
		Teachers: data.Teachers, Rooms: data.Rooms, Buildings: data.Buildings, SubjectPlans: data.SubjectPlans,
	})
}

type assignmentFormData struct {
	ScheduleID   int64
	Idx          int
	Assignment   domain.Assignment
	Rooms        []domain.Room
	Teachers     []domain.Teacher
	Groups       []domain.Group
	SubjectPlans []domain.SubjectPlan
	Days         []domain.Day
	Pairs        []int
	Error        string
}

func buildDayGroups(sched *domain.Schedule) []dayGroup {
	byDay := map[domain.Day]map[int][]assignmentView{}
	for idx, a := range sched.Assignments {
		if byDay[a.TimeSlot.Day()] == nil {
			byDay[a.TimeSlot.Day()] = map[int][]assignmentView{}
		}
		byDay[a.TimeSlot.Day()][a.TimeSlot.PairNum()] = append(byDay[a.TimeSlot.Day()][a.TimeSlot.PairNum()], assignmentView{Idx: idx, Assignment: a})
	}
	var groups []dayGroup
	for _, day := range domain.AllDays {
		pairsMap := byDay[day]
		if len(pairsMap) == 0 {
			continue
		}
		var pairNums []int
		for p := range pairsMap {
			pairNums = append(pairNums, p)
		}
		sort.Ints(pairNums)
		var pairs []pairGroup
		for _, p := range pairNums {
			items := pairsMap[p]
			sort.Slice(items, func(i, j int) bool { return items[i].Idx < items[j].Idx })
			pairs = append(pairs, pairGroup{PairNum: p, Items: items})
		}
		groups = append(groups, dayGroup{Day: day, Pairs: pairs})
	}
	return groups
}

func (h *Handler) loadSchedulesList(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["schedules_list.html"], schedulesListData{Schedules: list, Error: errMsg})
}

func (h *Handler) schedulesList(w http.ResponseWriter, r *http.Request) {
	h.loadSchedulesList(w, r, http.StatusOK, "")
}

func (h *Handler) schedulesGenerate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	in := app.GenerateInput{
		Name:           r.FormValue("name"),
		SolverType:     r.FormValue("solver_type"),
		TimeoutSec:     atoi(r.FormValue("timeout_sec"), 30),
		SemesterHalf:   domain.SemesterHalf(r.FormValue("semester_half")),
		ImproveAlgo:    r.FormValue("improve_algo"),
		ParallelStarts: atoi(r.FormValue("parallel_starts"), 1),
		Preferences: domain.SolverPreferences{
			LectureBeforePractice:  r.FormValue("lecture_before_practice") != "",
			LecturePracticeSameDay: r.FormValue("lecture_practice_same_day") != "",
			SameSubjectSameDay:     r.FormValue("same_subject_same_day") != "",
		},
	}

	// Специальное значение "all" — запускаем ВСЕ методы параллельно, сохраняем каждый
	// как отдельное расписание. Пользователь потом сравнивает их в списке.
	if in.ImproveAlgo == "all" {
		saved, err := h.svc.GenerateAllMethods(r.Context(), in)
		if err != nil {
			h.loadSchedulesList(w, r, http.StatusUnprocessableEntity, "Не удалось сгенерировать все методы: "+err.Error())
			return
		}
		list, err := h.svc.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var bestScore int
		for i, s := range saved {
			if i == 0 || s.Score < bestScore {
				bestScore = s.Score
			}
		}
		successMsg := fmt.Sprintf("Сгенерировано %d расписаний (по методам). Лучший score: %d — сравните и выберите",
			len(saved), bestScore)
		renderStatus(w, r, http.StatusOK, h.pages["schedules_list.html"], schedulesListData{
			Schedules: list,
			Success:   successMsg,
		})
		return
	}

	sched, err := h.svc.Generate(r.Context(), in)
	if err != nil {
		h.loadSchedulesList(w, r, http.StatusUnprocessableEntity, "Не удалось сгенерировать расписание: "+err.Error())
		return
	}
	list, err := h.svc.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	successMsg := fmt.Sprintf("Расписание «%s» сгенерировано: %d занятий, score=%d", sched.Name, len(sched.Assignments), sched.Score)
	if n := len(sched.Unplaced); n > 0 {
		successMsg += fmt.Sprintf(". Не удалось разместить: %d (см. детали в расписании)", n)
	}
	renderStatus(w, r, http.StatusOK, h.pages["schedules_list.html"], schedulesListData{
		Schedules: list,
		Success:   successMsg,
	})
}

func (h *Handler) loadScheduleView(w http.ResponseWriter, r *http.Request, id int64, status int, errMsg, successMsg string) {
	sched, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["schedules_view.html"], scheduleViewData{
		Schedule: sched, DayGroups: buildDayGroups(sched),
		Breakdown: h.svc.Breakdown(sched, *data),
		Quality:   h.svc.Quality(sched),
		Teachers:  data.Teachers, Rooms: data.Rooms, Buildings: data.Buildings, Groups: data.Groups, SubjectPlans: data.SubjectPlans,
		Error: errMsg, Success: successMsg,
	})
}

func (h *Handler) schedulesView(w http.ResponseWriter, r *http.Request) {
	id, err := int64FromPath(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.loadScheduleView(w, r, id, http.StatusOK, "", "")
}

func (h *Handler) schedulesDelete(w http.ResponseWriter, r *http.Request) {
	id, err := int64FromPath(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.loadSchedulesList(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	h.loadSchedulesList(w, r, http.StatusOK, "")
}

func (h *Handler) schedulesAssignmentForm(w http.ResponseWriter, r *http.Request) {
	id, err := int64FromPath(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	idx, err := intFromPath(r, "idx")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sched, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if idx < 0 || idx >= len(sched.Assignments) {
		http.NotFound(w, r)
		return
	}
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, r, h.pages["schedules_assignment_form.html"], assignmentFormData{
		ScheduleID: id, Idx: idx, Assignment: sched.Assignments[idx],
		Rooms: data.Rooms, Teachers: data.Teachers, Groups: data.Groups, SubjectPlans: data.SubjectPlans,
		Days: allDays, Pairs: allPairs,
	})
}

func conflictMessage(c *app.ConflictError) string {
	if c.Type == app.ConflictTeacherUnavailable {
		return "Конфликт: преподаватель отметил это время как недоступное"
	}
	var what string
	switch c.Type {
	case "teacher_busy":
		what = "преподаватель уже занят в это время"
	case "group_busy":
		what = "группа уже занята в это время"
	case "room_busy":
		what = "аудитория уже занята в это время"
	default:
		what = c.Type
	}
	return fmt.Sprintf("Конфликт: %s (конфликтует с занятием №%d)", what, c.ConflictWith)
}

func (h *Handler) reAssignmentForm(w http.ResponseWriter, r *http.Request, id int64, idx int, req app.PatchRequest, status int, errMsg string) {
	sched, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := h.input.LoadInput(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a := sched.Assignments[idx]
	a.TimeSlot = req.TimeSlot
	if req.RoomID != "" {
		a.RoomID = req.RoomID
	}
	if req.Parity != "" {
		a.Parity = req.Parity
	}
	renderStatus(w, r, status, h.pages["schedules_assignment_form.html"], assignmentFormData{
		ScheduleID: id, Idx: idx, Assignment: a,
		Rooms: data.Rooms, Teachers: data.Teachers, Groups: data.Groups, SubjectPlans: data.SubjectPlans,
		Days: allDays, Pairs: allPairs, Error: errMsg,
	})
}

func (h *Handler) schedulesPatchAssignment(w http.ResponseWriter, r *http.Request) {
	id, err := int64FromPath(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	idx, err := intFromPath(r, "idx")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !parseForm(w, r) {
		return
	}
	ts, err := domain.NewTimeSlot(domain.Day(r.FormValue("day")), atoi(r.FormValue("pair_num"), 0))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req := app.PatchRequest{
		TimeSlot: ts,
		RoomID:   r.FormValue("room_id"),
		Parity:   domain.Parity(r.FormValue("parity")),
	}

	_, err = h.svc.PatchAssignment(r.Context(), id, idx, req)
	if err != nil {
		var conflict *app.ConflictError
		switch {
		case errors.As(err, &conflict):
			h.reAssignmentForm(w, r, id, idx, req, http.StatusConflict, conflictMessage(conflict))
		case errors.Is(err, domain.ErrNotFound):
			http.NotFound(w, r)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	h.loadScheduleView(w, r, id, http.StatusOK, "", "Занятие перенесено")
}
