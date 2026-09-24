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
	type key struct {
		day  domain.Day
		pair int
	}
	byTeacher := map[string]map[key]*teacherCell{}
	count := map[string]int{}
	cell := func(tid string, k key) *teacherCell {
		if byTeacher[tid] == nil {
			byTeacher[tid] = map[key]*teacherCell{}
		}
		if byTeacher[tid][k] == nil {
			byTeacher[tid][k] = &teacherCell{}
		}
		return byTeacher[tid][k]
	}
	for idx, a := range sched.Assignments {
		c := cell(a.TeacherID, key{a.TimeSlot.Day(), a.TimeSlot.PairNum()})
		c.Items = append(c.Items, assignmentView{Idx: idx, Assignment: a})
		count[a.TeacherID]++
	}

	sorted := append([]domain.Teacher(nil), teachers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var blocks []teacherBlock
	for _, t := range sorted {
		if count[t.ID] == 0 {
			continue
		}
		for _, ep := range t.ExternalPairs {
			c := cell(t.ID, key{ep.TimeSlot.Day(), ep.TimeSlot.PairNum()})
			c.External = append(c.External, ep)
		}
		cells := byTeacher[t.ID]
		first, last := map[domain.Day]int{}, map[domain.Day]int{}
		for k := range cells {
			if v, ok := first[k.day]; !ok || k.pair < v {
				first[k.day] = k.pair
			}
			if v, ok := last[k.day]; !ok || k.pair > v {
				last[k.day] = k.pair
			}
		}
		b := teacherBlock{TeacherID: t.ID, Name: t.Name, Pairs: count[t.ID]}
		for _, pair := range allPairs {
			row := teacherRow{PairNum: pair}
			for _, day := range allDays {
				var tc teacherCell
				if c := cells[key{day, pair}]; c != nil {
					tc = *c
				} else if f, ok := first[day]; ok && pair > f && pair < last[day] {
					tc.IsGap = true
				}
				row.Cells = append(row.Cells, tc)
			}
			b.Rows = append(b.Rows, row)
		}
		blocks = append(blocks, b)
	}
	return blocks
}

func (h *Handler) schedulesTeachersView(w http.ResponseWriter, r *http.Request) {
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
	render(w, r, h.pages["schedules_teachers_view.html"], scheduleTeachersViewData{
		Schedule: sched, Blocks: buildTeacherBlocks(sched, data.Teachers), Days: allDays,
		Rooms: data.Rooms, Buildings: data.Buildings, Groups: data.Groups, SubjectPlans: data.SubjectPlans,
	})
}
