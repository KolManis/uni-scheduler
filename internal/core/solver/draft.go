package solver

// Черновик расписания (scheduleDraft) — общее состояние обоих алгоритмов построения:
// что уже поставлено, кто и где занят, сколько часов плана осталось.

import (
	"log/slog"
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// scheduleDraft — черновик расписания во время построения: поставленные пары и занятость
// преподавателей, групп и аудиторий по слотам. Меняет его только addAssignment (через
// placePair и placeFixed); остальные функции построения его только читают.
type scheduleDraft struct {
	input            domain.InputData
	groupMap         map[string]domain.Group
	roomMap          map[string]domain.Room
	assignments      []domain.Assignment
	subjectCount     map[string]map[domain.ClassType]int
	occupiedGroups   map[domain.TimeSlot]map[string]domain.Parity
	occupiedTeachers map[domain.TimeSlot]map[string]domain.Parity
	occupiedRooms    map[domain.TimeSlot]map[string]domain.Parity
	planKeys         map[string]string // id плана → subjectKey, считается один раз
	logger           *slog.Logger
}

func newDraft(input domain.InputData) *scheduleDraft {
	gm := make(map[string]domain.Group)
	for _, g := range input.Groups {
		gm[g.ID] = g
	}
	rm := make(map[string]domain.Room)
	for _, r := range input.Rooms {
		rm[r.ID] = r
	}
	pk := make(map[string]string, len(input.SubjectPlans))
	for _, sp := range input.SubjectPlans {
		pk[sp.ID] = subjectKey(sp.Name)
	}
	// Пары преподавателей на других факультетах занимают их заранее, в свою чётность:
	// построение обходит их так же, как уже поставленные пары (HC7).
	occupiedTeachers := make(map[domain.TimeSlot]map[string]domain.Parity)
	for _, t := range input.Teachers {
		for _, ep := range t.ExternalPairs {
			p := ep.Parity
			if p == "" {
				p = domain.Always
			}
			if occupiedTeachers[ep.TimeSlot] == nil {
				occupiedTeachers[ep.TimeSlot] = make(map[string]domain.Parity)
			}
			occupiedTeachers[ep.TimeSlot][t.ID] = mergeParity(occupiedTeachers[ep.TimeSlot][t.ID], p)
		}
	}
	return &scheduleDraft{
		input:            input,
		groupMap:         gm,
		roomMap:          rm,
		assignments:      []domain.Assignment{},
		subjectCount:     make(map[string]map[domain.ClassType]int),
		occupiedGroups:   make(map[domain.TimeSlot]map[string]domain.Parity),
		occupiedTeachers: occupiedTeachers,
		occupiedRooms:    make(map[domain.TimeSlot]map[string]domain.Parity),
		planKeys:         pk,
		logger:           slog.Default(),
	}
}

// placeFixed ставит закреплённую пару как есть.
func placeFixed(draft *scheduleDraft, a domain.Assignment) {
	a.Pinned = true
	if a.Parity == "" {
		a.Parity = domain.Always
	}
	addAssignment(draft, a)
}

// addAssignment добавляет пару в расписание: занимает преподавателя, группы и аудиторию
// в её слот и чётность и засчитывает её часы в план.
func addAssignment(draft *scheduleDraft, a domain.Assignment) {
	draft.assignments = append(draft.assignments, a)
	if draft.subjectCount[a.SubjectID] == nil {
		draft.subjectCount[a.SubjectID] = make(map[domain.ClassType]int)
	}
	draft.subjectCount[a.SubjectID][a.Type] += 2 // пара — 2 часа

	for _, gid := range a.GroupIDs {
		occupy(draft.occupiedGroups, a.TimeSlot, gid, a.Parity)
	}
	occupy(draft.occupiedTeachers, a.TimeSlot, a.TeacherID, a.Parity)
	occupy(draft.occupiedRooms, a.TimeSlot, a.RoomID, a.Parity)
}

// occupy отмечает, что ресурс id (группа, преподаватель или аудитория) занят в slot в
// недели parity; если он уже был занят в другую неделю — теперь занят в обе.
func occupy(occupied map[domain.TimeSlot]map[string]domain.Parity, slot domain.TimeSlot, id string, parity domain.Parity) {
	if occupied[slot] == nil {
		occupied[slot] = make(map[string]domain.Parity)
	}
	occupied[slot][id] = mergeParity(occupied[slot][id], parity)
}

// placementTask — одна пара, которую надо разместить: единица работы для фаз 1–3.
type placementTask struct {
	teacher   domain.Teacher
	subject   domain.SubjectPlan
	classType domain.ClassType
	parity    domain.Parity
}

// subjectsOfTeacher — учебные планы преподавателя в порядке входных данных.
func subjectsOfTeacher(input domain.InputData, teacherID string) []domain.SubjectPlan {
	var plans []domain.SubjectPlan
	for _, sp := range input.SubjectPlans {
		if sp.TeacherID == teacherID {
			plans = append(plans, sp)
		}
	}
	return plans
}

// collectRemaining — пары этого преподавателя, которые ещё не поставлены. Порядок:
// одиночные лекции → практики → лабы. Внутри типа — предметы с большим потоком идут раньше.
func collectRemaining(draft *scheduleDraft, input domain.InputData, teacher domain.Teacher) []placementTask {
	subjects := subjectsOfTeacher(input, teacher.ID)
	sortSubjectsByPriority(subjects)

	var tasks []placementTask
	for _, classType := range []domain.ClassType{domain.Lecture, domain.Practice, domain.Lab} {
		for _, sp := range subjects {
			remaining := remainingHours(draft, sp, classType)
			if remaining <= 0 {
				continue
			}
			parity := sp.Parity
			if parity == "" {
				parity = domain.Always
			}
			for h := 0; h < remaining; h += 2 {
				tasks = append(tasks, placementTask{teacher: teacher, subject: sp, classType: classType, parity: parity})
			}
		}
	}
	return tasks
}

// sortSubjectsByPriority — сначала предметы с большим потоком (больше групп), при равенстве — с большей нагрузкой.
func sortSubjectsByPriority(subjects []domain.SubjectPlan) {
	sort.Slice(subjects, func(i, j int) bool {
		lenI := len(subjects[i].GroupIDs)
		lenJ := len(subjects[j].GroupIDs)
		if lenI != lenJ {
			return lenI > lenJ
		}
		totalI := subjects[i].LectureHours + subjects[i].PracticeHours + subjects[i].LabHours
		totalJ := subjects[j].LectureHours + subjects[j].PracticeHours + subjects[j].LabHours
		return totalI > totalJ
	})
}

// mergeParity — чётность, в которую ресурс занят после ещё одной пары: чётная + нечётная = всегда.
func mergeParity(existing, new domain.Parity) domain.Parity {
	if existing == "" {
		return new
	}
	if existing == domain.Always || new == domain.Always {
		return domain.Always
	}
	if existing != new {
		return domain.Always
	}
	return existing
}

// remainingHours — сколько часов этого вида занятий по плану ещё не поставлено.
func remainingHours(draft *scheduleDraft, subject domain.SubjectPlan, classType domain.ClassType) int {
	current := 0
	if draft.subjectCount[subject.ID] != nil {
		current = draft.subjectCount[subject.ID][classType]
	}
	var total int
	switch classType {
	case domain.Lecture:
		total = subject.LectureHours
	case domain.Practice:
		total = subject.PracticeHours
	case domain.Lab:
		total = subject.LabHours
	}
	return total - current
}

// allSubjectsPlaced — поставлены ли все часы всех планов.
// Мультигрупповые практики/лабы не требуются (best-effort).
func allSubjectsPlaced(draft *scheduleDraft) bool {
	for _, plan := range draft.input.SubjectPlans {
		for _, ct := range []domain.ClassType{domain.Lecture, domain.Practice, domain.Lab} {
			if ct != domain.Lecture && len(plan.GroupIDs) > 1 {
				continue
			}
			if remainingHours(draft, plan, ct) > 0 {
				return false
			}
		}
	}
	return true
}
