package web

import (
	"errors"
	"net/http"
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// teacherCell — ячейка сетки преподавателя: его пары, пары на других факультетах и признак окна.
type teacherCell struct {
	Items    []assignmentView
	External []domain.ExternalPair
	IsGap    bool
}

type teacherRow struct {
	PairNum int
	Cells   []teacherCell // по одной на каждый день из allDays
}

type teacherBlock struct {
	TeacherID string
	Name      string
	Pairs     int // пар кафедры в неделю (без учёта чётности)
	Rows      []teacherRow
}

type scheduleTeachersViewData struct {
	Schedule     *domain.Schedule
	Blocks       []teacherBlock
	Days         []domain.Day
	Rooms        []domain.Room
	Buildings    []domain.Building
	Groups       []domain.Group
	SubjectPlans []domain.SubjectPlan
}

// buildTeacherBlocks — сетка «пара × день» для каждого преподавателя, у которого есть пары
// в этом расписании. Окно — пустая клетка между занятиями в один день, пары на других
// факультетах тоже считаются занятиями.
func buildTeacherBlocks(sched *domain.Schedule, teachers []domain.Teacher) []teacherBlock {
	byTeacher := map[string][]assignmentView{}
	for idx, a := range sched.Assignments {
		byTeacher[a.TeacherID] = append(byTeacher[a.TeacherID], assignmentView{Idx: idx, Assignment: a})
	}

	sorted := append([]domain.Teacher(nil), teachers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var blocks []teacherBlock
	for _, t := range sorted {
		if len(byTeacher[t.ID]) > 0 {
			blocks = append(blocks, buildTeacherBlock(t, byTeacher[t.ID]))
		}
	}
	return blocks
}

// buildTeacherBlock — сетка одного преподавателя: его пары и пары на других факультетах
// по слотам, пустые клетки между занятиями дня отмечены как окна.
func buildTeacherBlock(t domain.Teacher, pairs []assignmentView) teacherBlock {
	cells := map[domain.TimeSlot]teacherCell{}
	for _, p := range pairs {
		c := cells[p.TimeSlot]
		c.Items = append(c.Items, p)
		cells[p.TimeSlot] = c
	}
	for _, ep := range t.ExternalPairs {
		c := cells[ep.TimeSlot]
		c.External = append(c.External, ep)
		cells[ep.TimeSlot] = c
	}

	block := teacherBlock{TeacherID: t.ID, Name: t.Name, Pairs: len(pairs)}
	for _, num := range allPairs {
		row := teacherRow{PairNum: num}
		for _, day := range allDays {
			slot := domain.MustNewTimeSlot(day, num)
			c, busy := cells[slot]
			if !busy {
				first, last := busyRange(cells, day)
				c.IsGap = first > 0 && num > first && num < last
			}
			row.Cells = append(row.Cells, c)
		}
		block.Rows = append(block.Rows, row)
	}
	return block
}

// busyRange — первая и последняя занятая пара дня; 0, 0 — день свободен.
func busyRange(cells map[domain.TimeSlot]teacherCell, day domain.Day) (first, last int) {
	for num := domain.FirstPair; num <= domain.LastPair; num++ {
		if _, busy := cells[domain.MustNewTimeSlot(day, num)]; busy {
			if first == 0 {
				first = num
			}
			last = num
		}
	}
	return first, last
}

func (h *Handler) schedulesTeachersView(w http.ResponseWriter, r *http.Request) {
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
	render(w, r, h.pages["schedules_teachers_view.html"], scheduleTeachersViewData{
		Schedule: sched, Blocks: buildTeacherBlocks(sched, data.Teachers), Days: allDays,
		Rooms: data.Rooms, Buildings: data.Buildings, Groups: data.Groups, SubjectPlans: data.SubjectPlans,
	})
}
