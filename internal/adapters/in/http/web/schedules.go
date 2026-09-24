package web

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/KolManis/uni-scheduler/internal/core/application/commands/deleteschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/generateallmethods"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/generateschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/moveassignment"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/pinassignment"
	"github.com/KolManis/uni-scheduler/internal/core/application/generation"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/checkinput"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/evaluateschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/moveoptions"
	"github.com/KolManis/uni-scheduler/internal/core/application/rules"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

type schedulesListData struct {
	Schedules       []domain.ScheduleSummary
	Problems        []checkinput.Problem // проверка данных до генерации
	ProblemErrors   int
	ProblemWarnings int
	Error           string
	Success         string
}

// withProblems дописывает к странице списка результат проверки данных. Если проверка
// не удалась, страница показывается без неё: генерации это не мешает.
func (h *Handler) withProblems(r *http.Request, d schedulesListData) schedulesListData {
	problems, err := h.uc.CheckInput.Handle(r.Context())
	if err != nil {
		return d
	}
	d.Problems = problems
	for _, p := range problems {
		if p.Severity == checkinput.ProblemError {
			d.ProblemErrors++
		} else {
			d.ProblemWarnings++
		}
	}
	return d
}

// assignmentView добавляет к назначению его индекс в исходном срезе Schedule.Assignments —
// он же используется как idx в PATCH /schedules/{id}/assignments/{idx}.
type assignmentView struct {
	Idx int
	domain.Assignment
}

type pairGroup struct {
	PairNum  int
	Items    []assignmentView
	External []externalView // пары преподавателей на других факультетах в этом слоте
}

// externalView — пара преподавателя на другом факультете: только для сведения, не двигается.
type externalView struct {
	TeacherID string
	domain.ExternalPair
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
	PinnedCount  int // закреплённых пар — есть ли что сохранить при перегенерации
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
	sched, err := h.schedule(r.Context(), id)
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
	Moves        []moveRow // куда можно перенести пару: строки — номера пар, колонки — дни
	Error        string
}

// moveCell — одна клетка сетки «куда перенести»: можно ли, в какую аудиторию и что изменится.
type moveCell struct {
	Day     domain.Day
	Pair    int
	Current bool
	OK      bool
	RoomID  string
	Reason  string // почему нельзя (кратко)
	Effects string // что изменится: окна, дни с одной парой, суббота
	Delta   int    // изменение score
	Warn    bool   // перенос откроет окно в 2+ пары: разрешён, но нарушает HC8
}

type moveRow struct {
	Pair  int
	Cells []moveCell
}

// buildMoveGrid раскладывает варианты переноса в сетку «пара × день».
func buildMoveGrid(opts []moveoptions.Option, currentRoom string) []moveRow {
	bySlot := make(map[domain.TimeSlot]moveoptions.Option, len(opts))
	for _, o := range opts {
		bySlot[o.Slot] = o
	}
	rows := make([]moveRow, 0, len(allPairs))
	for _, pair := range allPairs {
		row := moveRow{Pair: pair}
		for _, day := range allDays {
			o := bySlot[domain.MustNewTimeSlot(day, pair)]
			c := moveCell{Day: day, Pair: pair, Current: o.Current, RoomID: o.RoomID}
			switch {
			case o.Current:
			case o.Conflict != nil:
				c.Reason = shortConflict(o.Conflict)
			default:
				c.OK = true
				c.Delta = o.ScoreDelta
				c.Effects = moveEffects(o, currentRoom)
				c.Warn = o.LongGapsDelta > 0
			}
			row.Cells = append(row.Cells, c)
		}
		rows = append(rows, row)
	}
	return rows
}

// shortConflict — причина, по которой в слот нельзя, в два-три слова для клетки сетки.
func shortConflict(c *rules.ConflictError) string {
	switch c.Type {
	case rules.ConflictTeacherUnavailable:
		return "преподаватель недоступен"
	case rules.ConflictTeacherExternalPair:
		return "другой факультет"
	case "teacher_busy":
		return "преподаватель занят"
	case "group_busy":
		return "группа занята"
	case "room_busy":
		return "нет свободной аудитории"
	}
	return c.Type
}

// moveEffects — что изменит перенос, человеческими словами: «−1 окно, +1 день с одной парой».
func moveEffects(o moveoptions.Option, currentRoom string) string {
	var parts []string
	add := func(n int, what string) {
		if n != 0 {
			parts = append(parts, fmt.Sprintf("%+d %s", n, what))
		}
	}
	add(o.LongGapsDelta, "окно 2+")
	add(o.GapsDelta, "окна")
	add(o.SingleDaysDelta, "дн. с 1 парой")
	add(o.SaturdayDelta, "суббота")
	if o.RoomID != currentRoom {
		parts = append(parts, "другая ауд.")
	}
	return strings.Join(parts, ", ")
}

// buildDayGroups — пары расписания по дням и номерам пар. Пары на других факультетах
// показываются у преподавателей, которые ведут занятия в этом расписании.
func buildDayGroups(sched *domain.Schedule, teachers []domain.Teacher) []dayGroup {
	type cell struct {
		items    []assignmentView
		external []externalView
	}
	byDay := map[domain.Day]map[int]*cell{}
	at := func(slot domain.TimeSlot) *cell {
		if byDay[slot.Day()] == nil {
			byDay[slot.Day()] = map[int]*cell{}
		}
		c := byDay[slot.Day()][slot.PairNum()]
		if c == nil {
			c = &cell{}
			byDay[slot.Day()][slot.PairNum()] = c
		}
		return c
	}
	inSchedule := map[string]bool{}
	for idx, a := range sched.Assignments {
		c := at(a.TimeSlot)
		c.items = append(c.items, assignmentView{Idx: idx, Assignment: a})
		inSchedule[a.TeacherID] = true
	}
	for _, t := range teachers {
		if !inSchedule[t.ID] {
			continue
		}
		for _, ep := range t.ExternalPairs {
			c := at(ep.TimeSlot)
			c.external = append(c.external, externalView{TeacherID: t.ID, ExternalPair: ep})
		}
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
			c := pairsMap[p]
			sort.Slice(c.items, func(i, j int) bool { return c.items[i].Idx < c.items[j].Idx })
			pairs = append(pairs, pairGroup{PairNum: p, Items: c.items, External: c.external})
		}
		groups = append(groups, dayGroup{Day: day, Pairs: pairs})
	}
	return groups
}

func (h *Handler) loadSchedulesList(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	list, err := h.uc.ListSchedules.Handle(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderStatus(w, r, status, h.pages["schedules_list.html"], h.withProblems(r, schedulesListData{Schedules: list, Error: errMsg}))
}

func (h *Handler) schedulesList(w http.ResponseWriter, r *http.Request) {
	h.loadSchedulesList(w, r, http.StatusOK, "")
}

func (h *Handler) schedulesGenerate(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	req := generation.Request{
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
	if req.ImproveAlgo == "all" {
		cmd, err := generateallmethods.NewCommand(req)
		if err != nil {
			h.loadSchedulesList(w, r, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := h.uc.GenerateAllMethods.Handle(r.Context(), cmd)
		if err != nil {
			h.loadSchedulesList(w, r, http.StatusUnprocessableEntity, "Не удалось сгенерировать все методы: "+err.Error())
			return
		}
		list, err := h.uc.ListSchedules.Handle(r.Context())
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
		renderStatus(w, r, http.StatusOK, h.pages["schedules_list.html"], h.withProblems(r, schedulesListData{
			Schedules: list,
			Success:   successMsg,
		}))
		return
	}

	cmd, err := generateschedule.NewCommand(req)
	if err != nil {
		h.loadSchedulesList(w, r, http.StatusBadRequest, err.Error())
		return
	}
	sched, err := h.uc.GenerateSchedule.Handle(r.Context(), cmd)
	if err != nil {
		h.loadSchedulesList(w, r, http.StatusUnprocessableEntity, "Не удалось сгенерировать расписание: "+err.Error())
		return
	}
	list, err := h.uc.ListSchedules.Handle(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	successMsg := fmt.Sprintf("Расписание «%s» сгенерировано: %d занятий, score=%d", sched.Name, len(sched.Assignments), sched.Score)
	if n := len(sched.Unplaced); n > 0 {
		successMsg += fmt.Sprintf(". Не удалось разместить: %d (см. детали в расписании)", n)
	}
	renderStatus(w, r, http.StatusOK, h.pages["schedules_list.html"], h.withProblems(r, schedulesListData{
		Schedules: list,
		Success:   successMsg,
	}))
}

func (h *Handler) loadScheduleView(w http.ResponseWriter, r *http.Request, id int64, status int, errMsg, successMsg string) {
	sched, err := h.schedule(r.Context(), id)
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
	eval := h.uc.EvaluateSchedule.Handle(evaluateschedule.Query{Schedule: sched, Input: *data})
	renderStatus(w, r, status, h.pages["schedules_view.html"], scheduleViewData{
		Schedule: sched, DayGroups: buildDayGroups(sched, data.Teachers), PinnedCount: pinnedCount(sched),
		Breakdown: eval.Breakdown,
		Quality:   eval.Quality,
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
	cmd, err := deleteschedule.NewCommand(id)
	if err == nil {
		err = h.uc.DeleteSchedule.Handle(r.Context(), cmd)
	}
	if err != nil {
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
	sched, err := h.schedule(r.Context(), id)
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
		Days: allDays, Pairs: allPairs, Moves: h.moveGrid(r, id, idx, sched.Assignments[idx].RoomID),
	})
}

func conflictMessage(c *rules.ConflictError) string {
	if c.Type == rules.ConflictTeacherUnavailable {
		return "Конфликт: преподаватель отметил это время как недоступное"
	}
	if c.Type == rules.ConflictTeacherExternalPair {
		msg := "Конфликт: у преподавателя в это время пара на другом факультете"
		if c.Detail != "" {
			msg += ": " + c.Detail
		}
		return msg + " (" + parityLabel(c.Parity) + ")"
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

func (h *Handler) reAssignmentForm(w http.ResponseWriter, r *http.Request, id int64, idx int, cmd moveassignment.Command, status int, errMsg string) {
	sched, err := h.schedule(r.Context(), id)
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
	a.TimeSlot = cmd.TimeSlot
	if cmd.RoomID != "" {
		a.RoomID = cmd.RoomID
	}
	if cmd.Parity != "" {
		a.Parity = cmd.Parity
	}
	renderStatus(w, r, status, h.pages["schedules_assignment_form.html"], assignmentFormData{
		ScheduleID: id, Idx: idx, Assignment: a,
		Rooms: data.Rooms, Teachers: data.Teachers, Groups: data.Groups, SubjectPlans: data.SubjectPlans,
		Days: allDays, Pairs: allPairs, Moves: h.moveGrid(r, id, idx, sched.Assignments[idx].RoomID), Error: errMsg,
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
	cmd, err := moveassignment.NewCommand(id, idx, ts, r.FormValue("room_id"), domain.Parity(r.FormValue("parity")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err = h.uc.MoveAssignment.Handle(r.Context(), cmd)
	if err != nil {
		var conflict *rules.ConflictError
		switch {
		case errors.As(err, &conflict):
			h.reAssignmentForm(w, r, id, idx, cmd, http.StatusConflict, conflictMessage(conflict))
		case errors.Is(err, domain.ErrNotFound):
			http.NotFound(w, r)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	h.loadScheduleView(w, r, id, http.StatusOK, "", "Занятие перенесено")
}

// parityLabel — неделя по-русски: «каждая неделя», «чётная неделя», «нечётная неделя».
func parityLabel(p domain.Parity) string {
	switch p {
	case domain.Even:
		return "чётная неделя"
	case domain.Odd:
		return "нечётная неделя"
	}
	return "каждая неделя"
}

// moveGrid — сетка вариантов переноса; при ошибке пустая (форма работает и без неё).
func (h *Handler) moveGrid(r *http.Request, id int64, idx int, room string) []moveRow {
	q, err := moveoptions.NewQuery(id, idx)
	if err != nil {
		return nil
	}
	opts, err := h.uc.MoveOptions.Handle(r.Context(), q)
	if err != nil {
		return nil
	}
	return buildMoveGrid(opts, room)
}

// schedulesPin закрепляет пару или снимает закрепление (pinned=true|false).
func (h *Handler) schedulesPin(w http.ResponseWriter, r *http.Request) {
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
	pinned := r.FormValue("pinned") == "true"
	cmd, err := pinassignment.NewCommand(id, idx, pinned)
	if err == nil {
		_, err = h.uc.PinAssignment.Handle(r.Context(), cmd)
	}
	if err != nil {
		h.loadScheduleView(w, r, id, http.StatusInternalServerError, err.Error(), "")
		return
	}
	msg := "Закрепление снято"
	if pinned {
		msg = "Пара закреплена: при перегенерации она останется на месте"
	}
	h.loadScheduleView(w, r, id, http.StatusOK, "", msg)
}

// schedulesRegenerate строит новое расписание вокруг закреплённых пар этого.
// Построение и правила берутся из исходного расписания.
func (h *Handler) schedulesRegenerate(w http.ResponseWriter, r *http.Request) {
	id, err := int64FromPath(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !parseForm(w, r) {
		return
	}
	base, err := h.schedule(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	prefs := base.Options
	construction := prefs.Construction
	prefs.Construction = ""
	cmd, err := generateschedule.NewCommand(generation.Request{
		Name:           base.Name + " — перегенерация",
		SolverType:     construction,
		TimeoutSec:     atoi(r.FormValue("timeout_sec"), 120),
		ImproveAlgo:    r.FormValue("improve_algo"),
		Preferences:    prefs,
		BaseScheduleID: id,
	})
	if err != nil {
		h.loadScheduleView(w, r, id, http.StatusBadRequest, err.Error(), "")
		return
	}
	sched, err := h.uc.GenerateSchedule.Handle(r.Context(), cmd)
	if err != nil {
		h.loadScheduleView(w, r, id, http.StatusUnprocessableEntity, "Не удалось перегенерировать: "+err.Error(), "")
		return
	}
	h.loadScheduleView(w, r, sched.ID, http.StatusOK, "",
		fmt.Sprintf("Новое расписание построено вокруг закреплённых пар (score %d). Исходное сохранено в списке.", sched.Score))
}

func pinnedCount(sched *domain.Schedule) int {
	n := 0
	for _, a := range sched.Assignments {
		if a.Pinned {
			n++
		}
	}
	return n
}
