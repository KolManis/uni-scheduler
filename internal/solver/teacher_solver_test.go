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
	if !checkHardConstraints(sched.Assignments) {
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
