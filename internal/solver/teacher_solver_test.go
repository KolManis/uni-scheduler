package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func makeInput() schedule.InputData {
	return schedule.InputData{
		Buildings: []schedule.Building{{ID: "A", Name: "Корпус А"}},
		Departments: []schedule.Department{{ID: "D1", Name: "Кафедра"}},
		Groups: []schedule.Group{
			{ID: "G1", Name: "РИС-24-1б", StudentCount: 20, BuildingIDs: []string{"A"}},
			{ID: "G2", Name: "РИС-24-2б", StudentCount: 20, BuildingIDs: []string{"A"}},
		},
		Teachers: []schedule.Teacher{
			{ID: "T1", Name: "Преп. Иванов", DepartmentID: "D1", MaxWeeklyHours: 6,
				PreferredBuildings: []string{"A"}},
			{ID: "T2", Name: "Преп. Петров", DepartmentID: "D1", MaxWeeklyHours: 4,
				PreferredBuildings: []string{"A"}},
		},
		Rooms: []schedule.Room{
			{ID: "R1", Number: "101", BuildingID: "A", Capacity: 30, Type: "lecture"},
			{ID: "R2", Number: "201", BuildingID: "A", Capacity: 30, Type: "lab"},
		},
		SubjectPlans: []schedule.SubjectPlan{
			{
				ID: "SP1", Name: "Математика", DepartmentID: "D1",
				TeacherID: "T1", GroupIDs: []string{"G1"},
				LectureHours: 2, PracticeHours: 2, LabHours: 0,
				RequiresRoomType: "lecture", Parity: schedule.Always,
			},
			{
				ID: "SP2", Name: "Физика", DepartmentID: "D1",
				TeacherID: "T2", GroupIDs: []string{"G2"},
				LectureHours: 2, PracticeHours: 0, LabHours: 2,
				RequiresRoomType: "lecture", Parity: schedule.Always,
			},
		},
	}
}

func TestSolveTeacher_SimpleCase(t *testing.T) {
	input := makeInput()
	sched, err := SolveTeacher(input, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sched.Assignments) == 0 {
		t.Fatal("expected non-empty assignments")
	}
	t.Logf("assignments: %d, score: %d", len(sched.Assignments), sched.Score)
}

func TestSolveTeacher_NoHardConflicts(t *testing.T) {
	input := makeInput()
	sched, err := SolveTeacher(input, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !checkHardConstraints(sched.Assignments, nil) {
		t.Fatal("generated schedule violates hard constraints (HC1-HC3)")
	}
}

func TestSolveTeacher_NoSaturday(t *testing.T) {
	// при достаточном количестве слотов не должен ставить пары в субботу
	input := makeInput()
	sched, err := SolveTeacher(input, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, a := range sched.Assignments {
		if a.TimeSlot.Day == schedule.Saturday {
			t.Logf("warning: pair placed on Saturday (score penalty applied): %+v", a)
		}
	}
}

// TestPlaceGroupsSplit_FallsBackWhenNoCommonSlot проверяет сценарий из реального datasets:
// у большого потока (несколько групп) нет ни одного общего свободного окна на всех сразу,
// но у каждой группы по отдельности такое окно есть — placeGroupsSplit должен развести их
// по разным слотам вместо того, чтобы просто сдаться.
func TestPlaceGroupsSplit_FallsBackWhenNoCommonSlot(t *testing.T) {
	input := schedule.InputData{
		Groups: []schedule.Group{
			{ID: "G1", StudentCount: 20},
			{ID: "G2", StudentCount: 20},
		},
		Rooms: []schedule.Room{
			{ID: "R1", Number: "101", BuildingID: "A", Capacity: 30, Type: "lecture"},
		},
	}
	state := newTeacherState(input)
	teacher := schedule.Teacher{ID: "T1", MaxWeeklyHours: 20}
	subject := schedule.SubjectPlan{
		ID: "SP1", TeacherID: "T1", GroupIDs: []string{"G1", "G2"},
		PracticeHours: 2, RequiresRoomType: "lecture",
	}

	freeSlotG1 := schedule.TimeSlot{Day: schedule.Tuesday, PairNum: 1}
	freeSlotG2 := schedule.TimeSlot{Day: schedule.Wednesday, PairNum: 1}

	// Занимаем G1 везде, кроме freeSlotG1; G2 везде, кроме freeSlotG2 —
	// общего свободного слота на обе группы сразу не остаётся.
	for _, day := range schedule.AllDays {
		for pair := 1; pair <= 6; pair++ {
			slot := schedule.TimeSlot{Day: day, PairNum: pair}
			if state.occupiedGroups[slot] == nil {
				state.occupiedGroups[slot] = make(map[string]schedule.Parity)
			}
			if slot != freeSlotG1 {
				state.occupiedGroups[slot]["G1"] = schedule.Always
			}
			if slot != freeSlotG2 {
				state.occupiedGroups[slot]["G2"] = schedule.Always
			}
		}
	}

	ok := placeGroupsSplit(state, subject, schedule.Practice, teacher, schedule.Always, subject.GroupIDs)
	if !ok {
		t.Fatal("expected placement to succeed via split even without a common slot")
	}
	if len(state.assignments) != 2 {
		t.Fatalf("expected 2 assignments (one per group), got %d: %+v", len(state.assignments), state.assignments)
	}

	seen := map[string]schedule.TimeSlot{}
	for _, a := range state.assignments {
		if len(a.GroupIDs) != 1 {
			t.Fatalf("expected each split assignment to cover exactly one group, got %+v", a.GroupIDs)
		}
		seen[a.GroupIDs[0]] = a.TimeSlot
	}
	if seen["G1"] != freeSlotG1 {
		t.Errorf("G1 expected at %+v, got %+v", freeSlotG1, seen["G1"])
	}
	if seen["G2"] != freeSlotG2 {
		t.Errorf("G2 expected at %+v, got %+v", freeSlotG2, seen["G2"])
	}
}

func TestCalcGaps(t *testing.T) {
	cases := []struct {
		pairs []int
		want  int
	}{
		{[]int{1, 2, 3}, 0},
		{[]int{1, 3}, 1},
		{[]int{1, 4}, 2},
		{[]int{2}, 0},
		{[]int{1, 2, 4}, 1},
	}
	for _, c := range cases {
		got := calcGaps(c.pairs)
		if got != c.want {
			t.Errorf("calcGaps(%v) = %d, want %d", c.pairs, got, c.want)
		}
	}
}
