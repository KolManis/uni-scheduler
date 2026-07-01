package solver

import (
	"fmt"
	"testing"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

// buildTestInput создаёт минимальный набор данных: 2 группы, 2 преподавателя, 4 предмета
func buildTestInput() schedule.InputData {
	return schedule.InputData{
		Buildings: []schedule.Building{
			{ID: "A", Name: "Корпус А"},
		},
		Departments: []schedule.Department{
			{ID: "d1", Name: "ИТАС"},
		},
		Groups: []schedule.Group{
			{ID: "g1", Name: "РИС-23-1б", StudentCount: 25, BuildingIDs: []string{"A"}},
			{ID: "g2", Name: "РИС-23-2б", StudentCount: 20, BuildingIDs: []string{"A"}},
		},
		Teachers: []schedule.Teacher{
			{ID: "t1", Name: "Иванов И.И.", DepartmentID: "d1", MaxWeeklyHours: 20},
			{ID: "t2", Name: "Петров П.П.", DepartmentID: "d1", MaxWeeklyHours: 20},
		},
		Rooms: []schedule.Room{
			{ID: "r1", Number: "101", BuildingID: "A", Capacity: 30, Type: "lecture"},
			{ID: "r2", Number: "201", BuildingID: "A", Capacity: 30, Type: "lecture"},
			{ID: "r3", Number: "301", BuildingID: "A", Capacity: 30, Type: "lab"},
		},
		SubjectPlans: []schedule.SubjectPlan{
			// t1 ведёт Математику у g1 (2 лекции + 1 практика)
			{ID: "sp1", Name: "Математика", DepartmentID: "d1",
				LectureHours: 2, PracticeHours: 1,
				TeacherID: "t1", GroupIDs: []string{"g1"}, Parity: schedule.Always},
			// t2 ведёт Физику у g1 (2 лекции)
			{ID: "sp2", Name: "Физика", DepartmentID: "d1",
				LectureHours: 2,
				TeacherID: "t2", GroupIDs: []string{"g1"}, Parity: schedule.Always},
			// t1 ведёт ООП у g2 (1 лекция — любая аудитория)
			{ID: "sp3", Name: "ООП лекция", DepartmentID: "d1",
				LectureHours: 1,
				TeacherID: "t1", GroupIDs: []string{"g2"}, Parity: schedule.Always},
			// t1 ведёт ООП лаб у g2 (1 лаб — только lab тип)
			{ID: "sp3b", Name: "ООП лаб", DepartmentID: "d1",
				LabHours: 1, RequiresRoomType: "lab",
				TeacherID: "t1", GroupIDs: []string{"g2"}, Parity: schedule.Always},
			// t2 ведёт Сети у g2 (2 лекции) — чётные недели
			{ID: "sp4", Name: "Сети", DepartmentID: "d1",
				LectureHours: 2,
				TeacherID: "t2", GroupIDs: []string{"g2"}, Parity: schedule.Even},
		},
	}
}

// checkHardConstraintsAll проверяет все HC для сгенерированного расписания.
// Возвращает список нарушений.
func checkHardConstraintsAll(assignments []schedule.Assignment, input schedule.InputData) []string {
	var violations []string

	type paritySet struct{ even, odd, always bool }
	slotGroup := make(map[schedule.TimeSlot]map[string]*paritySet)
	slotTeacher := make(map[schedule.TimeSlot]map[string]*paritySet)
	slotRoom := make(map[schedule.TimeSlot]map[string]*paritySet)

	addParity := func(m map[schedule.TimeSlot]map[string]*paritySet, slot schedule.TimeSlot, id string, p schedule.Parity) bool {
		if m[slot] == nil {
			m[slot] = make(map[string]*paritySet)
		}
		ps := m[slot][id]
		if ps == nil {
			ps = &paritySet{}
			m[slot][id] = ps
		}
		conflict := false
		switch p {
		case schedule.Always:
			if ps.always || ps.even || ps.odd {
				conflict = true
			}
			ps.always = true
		case schedule.Even:
			if ps.always || ps.even {
				conflict = true
			}
			ps.even = true
		case schedule.Odd:
			if ps.always || ps.odd {
				conflict = true
			}
			ps.odd = true
		}
		return conflict
	}

	// HC: группы, преподаватели, аудитории не пересекаются
	for i, a := range assignments {
		for _, gid := range a.GroupIDs {
			if addParity(slotGroup, a.TimeSlot, gid, a.Parity) {
				violations = append(violations, fmt.Sprintf("HC1 группа %s конфликт слот %v (assign #%d)", gid, a.TimeSlot, i))
			}
		}
		if addParity(slotTeacher, a.TimeSlot, a.TeacherID, a.Parity) {
			violations = append(violations, fmt.Sprintf("HC2 преп %s конфликт слот %v (assign #%d)", a.TeacherID, a.TimeSlot, i))
		}
		if addParity(slotRoom, a.TimeSlot, a.RoomID, a.Parity) {
			violations = append(violations, fmt.Sprintf("HC3 аудитория %s конфликт слот %v (assign #%d)", a.RoomID, a.TimeSlot, i))
		}
	}

	// HC: номер пары 1..6
	for i, a := range assignments {
		if a.TimeSlot.PairNum < 1 || a.TimeSlot.PairNum > 6 {
			violations = append(violations, fmt.Sprintf("HC9 недопустимая пара %d (assign #%d)", a.TimeSlot.PairNum, i))
		}
	}

	// HC: аудитория правильного типа для лабораторных
	roomMap := make(map[string]schedule.Room)
	for _, r := range input.Rooms {
		roomMap[r.ID] = r
	}
	for i, a := range assignments {
		sp := findSubjectPlan(input, a.SubjectID)
		if sp == nil {
			continue
		}
		r := roomMap[a.RoomID]
		if sp.RequiresRoomType != "" && r.Type != sp.RequiresRoomType {
			violations = append(violations, fmt.Sprintf("HC5 неправильный тип аудитории %s (нужен %s, есть %s) assign #%d",
				a.RoomID, sp.RequiresRoomType, r.Type, i))
		}
	}

	// HC: преподаватель ведёт только свои предметы
	for i, a := range assignments {
		sp := findSubjectPlan(input, a.SubjectID)
		if sp != nil && sp.TeacherID != a.TeacherID {
			violations = append(violations, fmt.Sprintf("HC8 преп %s не ведёт предмет %s (assign #%d)", a.TeacherID, a.SubjectID, i))
		}
	}

	return violations
}

func findSubjectPlan(input schedule.InputData, id string) *schedule.SubjectPlan {
	for i, sp := range input.SubjectPlans {
		if sp.ID == id {
			return &input.SubjectPlans[i]
		}
	}
	return nil
}

func TestSolveTeacher_HardConstraints(t *testing.T) {
	input := buildTestInput()
	sched, err := SolveTeacher(input, 100)
	if err != nil {
		t.Fatalf("SolveTeacher вернул ошибку: %v", err)
	}
	if sched == nil {
		t.Fatal("SolveTeacher вернул nil расписание")
	}

	t.Logf("Сгенерировано пар: %d, штраф: %d", len(sched.Assignments), sched.Score)

	violations := checkHardConstraintsAll(sched.Assignments, input)
	for _, v := range violations {
		t.Errorf("НАРУШЕНИЕ: %s", v)
	}
	if len(violations) == 0 {
		t.Logf("Все жёсткие ограничения соблюдены")
	}
}

func TestSolveTeacher_AllPairsPlaced(t *testing.T) {
	input := buildTestInput()
	sched, err := SolveTeacher(input, 100)
	if err != nil {
		t.Fatalf("SolveTeacher вернул ошибку: %v", err)
	}

	// 1 пара = 2 академических часа: assignments = ceil(hours/2)
	pairsFor := func(hours int) int { return (hours + 1) / 2 }
	expected := 0
	for _, sp := range input.SubjectPlans {
		expected += pairsFor(sp.LectureHours) + pairsFor(sp.PracticeHours) + pairsFor(sp.LabHours)
	}
	got := len(sched.Assignments)
	t.Logf("Ожидается пар: %d, размещено: %d", expected, got)
	if got != expected {
		t.Errorf("не все пары размещены: ожидалось %d, получили %d", expected, got)
	}
}

func TestSolveTeacher_SubjectTypeCorrect(t *testing.T) {
	input := buildTestInput()
	sched, err := SolveTeacher(input, 100)
	if err != nil {
		t.Fatalf("SolveTeacher вернул ошибку: %v", err)
	}

	// Считаем по типам для каждого SubjectPlan
	type key struct {
		subjectID string
		classType schedule.ClassType
	}
	counts := make(map[key]int)
	for _, a := range sched.Assignments {
		counts[key{a.SubjectID, a.Type}]++
	}

	pairsFor := func(hours int) int { return (hours + 1) / 2 }
	for _, sp := range input.SubjectPlans {
		if sp.LectureHours > 0 {
			got := counts[key{sp.ID, schedule.Lecture}]
			exp := pairsFor(sp.LectureHours)
			if got != exp {
				t.Errorf("предмет %s: лекций ожидалось %d, размещено %d", sp.Name, exp, got)
			}
		}
		if sp.PracticeHours > 0 {
			got := counts[key{sp.ID, schedule.Practice}]
			exp := pairsFor(sp.PracticeHours)
			if got != exp {
				t.Errorf("предмет %s: практик ожидалось %d, размещено %d", sp.Name, exp, got)
			}
		}
		if sp.LabHours > 0 {
			got := counts[key{sp.ID, schedule.Lab}]
			exp := pairsFor(sp.LabHours)
			if got != exp {
				t.Errorf("предмет %s: лабораторных ожидалось %d, размещено %d", sp.Name, exp, got)
			}
		}
	}
}

func TestSolveTeacher_PrintSchedule(t *testing.T) {
	input := buildTestInput()
	sched, err := SolveTeacher(input, 100)
	if err != nil {
		t.Fatalf("SolveTeacher вернул ошибку: %v", err)
	}

	dayOrder := map[schedule.Day]int{
		schedule.Monday: 1, schedule.Tuesday: 2, schedule.Wednesday: 3,
		schedule.Thursday: 4, schedule.Friday: 5, schedule.Saturday: 6,
	}

	t.Logf("=== Расписание (штраф=%d) ===", sched.Score)
	// Сортируем по дням и парам
	sorted := make([]schedule.Assignment, len(sched.Assignments))
	copy(sorted, sched.Assignments)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			di, dj := dayOrder[sorted[i].TimeSlot.Day], dayOrder[sorted[j].TimeSlot.Day]
			if di > dj || (di == dj && sorted[i].TimeSlot.PairNum > sorted[j].TimeSlot.PairNum) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	for _, a := range sorted {
		t.Logf("  %s пара%d [%s] %s (%s) гр:%v ауд:%s чётн:%s",
			a.TimeSlot.Day, a.TimeSlot.PairNum, a.TeacherID, a.SubjectID, a.Type,
			a.GroupIDs, a.RoomID, a.Parity)
	}
}
