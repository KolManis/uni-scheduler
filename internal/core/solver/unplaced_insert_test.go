package solver

import (
	"testing"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// ejectionInput: у T1 свободен только пн-1, а там уже стоит пара T2 у той же группы.
// Без вытеснения пару T1 поставить некуда.
func ejectionInput() (domain.InputData, []domain.Assignment) {
	only := domain.MustNewTimeSlot(domain.Monday, 1)
	var blocked []domain.TimeSlot
	for _, d := range domain.AllDays {
		for p := 1; p <= 6; p++ {
			if s := domain.MustNewTimeSlot(d, p); s != only {
				blocked = append(blocked, s)
			}
		}
	}
	input := domain.InputData{
		Groups: []domain.Group{{ID: "G1", StudentCount: 20}},
		Teachers: []domain.Teacher{
			{ID: "T1", UnavailableSlots: blocked},
			{ID: "T2"},
		},
		Rooms: []domain.Room{{ID: "R1", BuildingID: "A", Capacity: 30, Type: "lecture"}},
		SubjectPlans: []domain.SubjectPlan{
			{ID: "P1", Name: "Алгебра", TeacherID: "T1", GroupIDs: []string{"G1"}, PracticeHours: 2, RequiresRoomType: "lecture", Parity: domain.Always},
			{ID: "P2", Name: "Физика", TeacherID: "T2", GroupIDs: []string{"G1"}, PracticeHours: 2, RequiresRoomType: "lecture", Parity: domain.Always},
		},
	}
	placed := []domain.Assignment{{
		GroupIDs: []string{"G1"}, TeacherID: "T2", RoomID: "R1", SubjectID: "P2",
		Type: domain.Practice, TimeSlot: only, Parity: domain.Always, BuildingID: "A",
	}}
	return input, placed
}

func TestInsertUnplaced_EjectsBlocker(t *testing.T) {
	input, placed := ejectionInput()
	unavail := buildTeacherUnavailable(input)

	got := insertUnplaced(placed, input, unavail)

	if n := len(ComputeUnplaced(got, input)); n != 0 {
		t.Fatalf("осталось непоставленных: %d", n)
	}
	if !checkHardConstraints(got, unavail) {
		t.Fatal("нарушены жёсткие ограничения")
	}
	for _, a := range got {
		if a.TeacherID == "T1" && a.TimeSlot != domain.MustNewTimeSlot(domain.Monday, 1) {
			t.Errorf("пара T1 не в единственном доступном слоте: %v", a.TimeSlot)
		}
	}
}

func TestInsertUnplaced_NothingToDo(t *testing.T) {
	input, placed := ejectionInput()
	input.SubjectPlans = input.SubjectPlans[1:] // только P2, она уже стоит
	got := insertUnplaced(placed, input, buildTeacherUnavailable(input))
	if len(got) != 1 || got[0].TimeSlot != placed[0].TimeSlot {
		t.Fatalf("расписание не должно меняться: %+v", got)
	}
}

// Какой бы порядок ни выбрало построение, после вставки обе пары должны стоять.
func TestSolve_PlacesAllAfterInsertion(t *testing.T) {
	input, _ := ejectionInput()
	for _, c := range []Construction{ConstructTeacher, ConstructDSatur} {
		sched, err := SolveWithBudget(input, c, 1000, ImproveHillClimb, 0, 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(ComputeUnplaced(sched.Assignments, input)); n != 0 {
			t.Errorf("%s: осталось непоставленных: %d", c, n)
		}
	}
}

func TestDSatur_RealDataValid(t *testing.T) {
	input := loadSnapshotInput(t)
	a, err := SolveWithBudget(input, ConstructDSatur, 1000, ImproveHillClimb, 0, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := SolveWithBudget(input, ConstructDSatur, 1000, ImproveHillClimb, 0, time.Nanosecond)
	if a.Score != b.Score {
		t.Errorf("построение без зерна должно быть детерминированным: %d и %d", a.Score, b.Score)
	}
	if v := checkHardConstraintsAll(a.Assignments, normalizeInput(input)); len(v) > 0 {
		t.Fatalf("нарушения: %v", v[:min(len(v), 5)])
	}
	if n := len(ComputeUnplaced(a.Assignments, input)); n != 0 {
		t.Errorf("не поставлено: %d", n)
	}
}

func TestParseConstruction(t *testing.T) {
	for in, want := range map[string]Construction{"": ConstructDSatur, "teacher": ConstructTeacher, "dsatur": ConstructDSatur} {
		if got, ok := ParseConstruction(in); !ok || got != want {
			t.Errorf("%q: получено %q, %v", in, got, ok)
		}
	}
	if _, ok := ParseConstruction("subject"); ok {
		t.Error("subject не должен приниматься")
	}
}
