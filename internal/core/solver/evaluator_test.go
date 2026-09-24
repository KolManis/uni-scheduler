package solver

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// loadSnapshotInput — реальные справочники из data/snapshots. Тест пропускается, если их нет.
func loadSnapshotInput(t testing.TB) domain.InputData {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "data", "snapshots")
	var in domain.InputData
	files := map[string]any{
		"buildings.json":     &in.Buildings,
		"departments.json":   &in.Departments,
		"groups.json":        &in.Groups,
		"teachers.json":      &in.Teachers,
		"rooms.json":         &in.Rooms,
		"subject_plans.json": &in.SubjectPlans,
	}
	for name, dst := range files {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Skipf("нет снимка справочников: %v", err)
		}
		raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
		if err := json.Unmarshal(raw, dst); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	return in
}

// TestEvaluator_MatchesFullFitness — инкрементальная оценка обязана совпадать с полной
// после любой последовательности ходов, включая откаты, и не нарушать жёсткие ограничения.
func TestEvaluator_MatchesFullFitness(t *testing.T) {
	cases := map[string]domain.SolverPreferences{
		"без правил":  {},
		"все правила": {LectureBeforePractice: true, LecturePracticeSameDay: true, SameSubjectSameDay: true},
	}
	for name, prefs := range cases {
		t.Run(name, func(t *testing.T) {
			input := loadSnapshotInput(t)
			input.Preferences = prefs
			sched, err := SolveTeacherWithBudget(input, 1000, ImproveHillClimb, 0, time.Nanosecond)
			if err != nil {
				t.Fatal(err)
			}
			input = normalizeInput(input)
			unavail := buildTeacherUnavailable(input)
			e := newEvaluator(sched.Assignments, input, unavail)
			if got, want := e.score(), calculateFitness(sched.Assignments, input); got != want {
				t.Fatalf("начальная оценка: %d, полная %d", got, want)
			}

			rng := rand.New(rand.NewSource(1))
			for step := 0; step < 3000; step++ {
				undo, ok := randomMove(e, rng)
				if ok && rng.Intn(3) == 0 {
					e.apply(undo)
				}
				if step%50 != 0 {
					continue
				}
				cur := e.assignments()
				if got, want := e.score(), calculateFitness(cur, input); got != want {
					t.Fatalf("шаг %d: инкрементальная %d, полная %d", step, got, want)
				}
				if !checkHardConstraints(cur, unavail) {
					t.Fatalf("шаг %d: нарушены жёсткие ограничения", step)
				}
			}
		})
	}
}

// TestLNS_KeepsAllPairs — после разрушения и восстановления ни одна пара не теряется.
func TestLNS_KeepsAllPairs(t *testing.T) {
	input := loadSnapshotInput(t)
	sched, err := SolveTeacherWithBudget(input, 1000, ImproveHillClimb, 0, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	input = normalizeInput(input)
	unavail := buildTeacherUnavailable(input)
	got := largeNeighborhoodSearch(sched.Assignments, input, time.Now().Add(2*time.Second), unavail)
	if len(got) != len(sched.Assignments) {
		t.Fatalf("пар было %d, стало %d", len(sched.Assignments), len(got))
	}
	if !checkHardConstraints(got, unavail) {
		t.Fatal("нарушены жёсткие ограничения")
	}
	if calculateFitness(got, input) > sched.Score {
		t.Fatalf("LNS ухудшил расписание: %d > %d", calculateFitness(got, input), sched.Score)
	}
}

// TestEvaluator_NeverAddsLongGap — HC8: перенос, открывающий окно в 2+ пары, отвергается.
func TestEvaluator_NeverAddsLongGap(t *testing.T) {
	mk := func(teacher string, pair int) domain.Assignment {
		return domain.Assignment{GroupIDs: []string{"G1"}, TeacherID: teacher, RoomID: "R" + teacher,
			SubjectID: "S" + teacher, Type: domain.Practice, Parity: domain.Always,
			TimeSlot: domain.MustNewTimeSlot(domain.Monday, pair)}
	}
	e := newEvaluator([]domain.Assignment{mk("T1", 1), mk("T2", 2)}, domain.InputData{}, nil)
	if _, ok := e.relocate(1, slotIndex(domain.MustNewTimeSlot(domain.Monday, 4))); ok {
		t.Fatal("перенос на 4-ю пару открыл бы окно в две пары и должен быть отвергнут")
	}
	if _, ok := e.relocate(1, slotIndex(domain.MustNewTimeSlot(domain.Monday, 3))); !ok {
		t.Fatal("окно в одну пару допустимо")
	}
}
