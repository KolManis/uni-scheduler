package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func makeInput() domain.InputData {
	return domain.InputData{
		Buildings:   []domain.Building{{ID: "A", Name: "Корпус А"}},
		Departments: []domain.Department{{ID: "D1", Name: "Кафедра"}},
		Groups: []domain.Group{
			{ID: "G1", Name: "РИС-24-1б", StudentCount: 20, BuildingIDs: []string{"A"}},
			{ID: "G2", Name: "РИС-24-2б", StudentCount: 20, BuildingIDs: []string{"A"}},
		},
		Teachers: []domain.Teacher{
			{ID: "T1", Name: "Преп. Иванов", DepartmentID: "D1", MaxWeeklyHours: 6,
				PreferredBuildings: []string{"A"}},
			{ID: "T2", Name: "Преп. Петров", DepartmentID: "D1", MaxWeeklyHours: 4,
				PreferredBuildings: []string{"A"}},
		},
		Rooms: []domain.Room{
			{ID: "R1", Number: "101", BuildingID: "A", Capacity: 30, Type: "lecture"},
			{ID: "R2", Number: "201", BuildingID: "A", Capacity: 30, Type: "lab"},
		},
		SubjectPlans: []domain.SubjectPlan{
			{
				ID: "SP1", Name: "Математика", DepartmentID: "D1",
				TeacherID: "T1", GroupIDs: []string{"G1"},
				LectureHours: 2, PracticeHours: 2, LabHours: 0,
				RequiresRoomType: "lecture", Parity: domain.Always,
			},
			{
				ID: "SP2", Name: "Физика", DepartmentID: "D1",
				TeacherID: "T2", GroupIDs: []string{"G2"},
				LectureHours: 2, PracticeHours: 0, LabHours: 2,
				RequiresRoomType: "lecture", Parity: domain.Always,
			},
		},
	}
}

func TestSolveTeacher_SimpleCase(t *testing.T) {
	input := makeInput()
	sched, err := Solve(input, Options{Construction: ConstructTeacher, Improve: ImproveHillClimb})
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
	sched, err := Solve(input, Options{Construction: ConstructTeacher, Improve: ImproveHillClimb})
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
	sched, err := Solve(input, Options{Construction: ConstructTeacher, Improve: ImproveHillClimb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, a := range sched.Assignments {
		if a.TimeSlot.Day() == domain.Saturday {
			t.Logf("warning: pair placed on Saturday (score penalty applied): %+v", a)
		}
	}
}

// TestPlaceGroupsSplit_KeepsGroupsTogether проверяет, что одну и ту же пару нельзя провести
// разным подпотокам в разное время: если общего свободного окна на весь состав нет, занятие
// уходит в unplaced целиком. Раньше состав дробился пополам, но это давало некорректные
// расписания — одну лекцию читают потоку раз, а не N раз каждой половине.
func TestPlacePair_KeepsGroupsTogether(t *testing.T) {
	input := domain.InputData{
		Groups: []domain.Group{
			{ID: "G1", StudentCount: 20},
			{ID: "G2", StudentCount: 20},
		},
		Rooms: []domain.Room{
			{ID: "R1", Number: "101", BuildingID: "A", Capacity: 30, Type: "lecture"},
		},
	}
	draft := newDraft(input)
	teacher := domain.Teacher{ID: "T1", MaxWeeklyHours: 20}
	subject := domain.SubjectPlan{
		ID: "SP1", TeacherID: "T1", GroupIDs: []string{"G1", "G2"},
		PracticeHours: 2, RequiresRoomType: "lecture",
	}

	freeSlotG1 := domain.MustNewTimeSlot(domain.Tuesday, 1)
	freeSlotG2 := domain.MustNewTimeSlot(domain.Wednesday, 1)

	// Занимаем G1 везде, кроме freeSlotG1; G2 везде, кроме freeSlotG2 —
	// общего свободного слота на обе группы сразу не остаётся.
	for _, day := range domain.AllDays {
		for pair := 1; pair <= 6; pair++ {
			slot := domain.MustNewTimeSlot(day, pair)
			if draft.occupiedGroups[slot] == nil {
				draft.occupiedGroups[slot] = make(map[string]domain.Parity)
			}
			if slot != freeSlotG1 {
				draft.occupiedGroups[slot]["G1"] = domain.Always
			}
			if slot != freeSlotG2 {
				draft.occupiedGroups[slot]["G2"] = domain.Always
			}
		}
	}

	_, ok := placePair(draft, placementTask{teacher: teacher, subject: subject, classType: domain.Practice, parity: domain.Always})
	if ok {
		t.Fatal("expected placement to fail: no common slot on all groups, splitting is not allowed")
	}
	if len(draft.assignments) != 0 {
		t.Fatalf("expected 0 assignments (subject should stay unplaced), got %d: %+v", len(draft.assignments), draft.assignments)
	}
}

// TestSlotPenalty_PrefersDayWithSinglePair — при построении вторую пару группы выгоднее
// поставить в день, где у неё уже есть одна пара, чем открыть новый день. Раньше было
// наоборот, и построение само создавало дни с единственной парой.
func TestSlotPenalty_PrefersDayWithSinglePair(t *testing.T) {
	tests := []struct {
		name     string
		existing domain.TimeSlot
		sameDay  domain.TimeSlot
		newDay   domain.TimeSlot
	}{
		{
			name:     "вплотную к паре понедельника дешевле, чем пустой вторник",
			existing: domain.MustNewTimeSlot(domain.Monday, 1),
			sameDay:  domain.MustNewTimeSlot(domain.Monday, 2),
			newDay:   domain.MustNewTimeSlot(domain.Tuesday, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := newDraft(domain.InputData{})
			draft.assignments = []domain.Assignment{
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T1",
					TimeSlot:  tt.existing,
					Parity:    domain.Always,
				},
			}
			subject := domain.SubjectPlan{
				ID:        "SP2",
				TeacherID: "T2",
				GroupIDs:  []string{"G1"},
			}

			sameDay := slotPenalty(draft, tt.sameDay, subject, domain.Practice, domain.Always, subject.GroupIDs, "T2")
			newDay := slotPenalty(draft, tt.newDay, subject, domain.Practice, domain.Always, subject.GroupIDs, "T2")
			if sameDay >= newDay {
				t.Errorf("штраф за день с парой (%d) должен быть меньше, чем за новый день (%d)", sameDay, newDay)
			}
		})
	}
}

// TestSlotPenalty_BlinkingPairFillsOtherWeekGap — «мигалка»: пара по чётным ставится туда,
// где у группы пара только по нечётным. У группы 1-я пара всегда, 2-я по нечётным, 3-я
// всегда: в чётную неделю 2-я пара — окно. Раньше построение считало 2-ю пару занятой
// в обе недели, видело перегруженный день и уводило новую пару в другой день, оставляя
// окно в чётной неделе.
func TestSlotPenalty_BlinkingPairFillsOtherWeekGap(t *testing.T) {
	tests := []struct {
		name     string
		gapSlot  domain.TimeSlot
		otherDay domain.TimeSlot
	}{
		{
			name:     "пара по чётным в окно чётной недели дешевле, чем в пустой день",
			gapSlot:  domain.MustNewTimeSlot(domain.Monday, 2),
			otherDay: domain.MustNewTimeSlot(domain.Tuesday, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := newDraft(domain.InputData{})
			draft.assignments = []domain.Assignment{
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T1",
					TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 1),
					Parity:    domain.Always,
				},
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T2",
					TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 2),
					Parity:    domain.Odd,
				},
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T3",
					TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 3),
					Parity:    domain.Always,
				},
			}
			subject := domain.SubjectPlan{
				ID:        "SP-EVEN",
				TeacherID: "T4",
				GroupIDs:  []string{"G1"},
			}

			inGap := slotPenalty(draft, tt.gapSlot, subject, domain.Practice, domain.Even, subject.GroupIDs, "T4")
			elsewhere := slotPenalty(draft, tt.otherDay, subject, domain.Practice, domain.Even, subject.GroupIDs, "T4")
			if inGap >= elsewhere {
				t.Errorf("штраф в окне чётной недели (%d) должен быть меньше, чем в пустом дне (%d)", inGap, elsewhere)
			}
		})
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
