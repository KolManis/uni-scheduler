package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func makeAssignment(groupID, teacherID string, day schedule.Day, pair int, parity schedule.Parity) schedule.Assignment {
	return schedule.Assignment{
		GroupIDs:   []string{groupID},
		TeacherID:  teacherID,
		RoomID:     "R1",
		SubjectID:  "S1",
		Type:       schedule.Lecture,
		TimeSlot:   schedule.TimeSlot{Day: day, PairNum: pair},
		Parity:     parity,
		BuildingID: "A",
	}
}

var emptyInput = schedule.InputData{}

func TestCalculateFitness_NoGaps_ZeroPenalty(t *testing.T) {
	// Пары подряд: 1,2,3 — нет окон, нет субботы
	assignments := []schedule.Assignment{
		makeAssignment("G1", "T1", schedule.Monday, 1, schedule.Always),
		makeAssignment("G1", "T1", schedule.Monday, 2, schedule.Always),
		makeAssignment("G1", "T1", schedule.Monday, 3, schedule.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	// SC7 форточка не применяется (>1 пары в день)
	// SC4 нет окон → 0
	if score != 0 {
		t.Fatalf("expected 0 penalty for compact schedule, got %d", score)
	}
}

func TestCalculateFitness_SaturdayPenalty(t *testing.T) {
	assignments := []schedule.Assignment{
		makeAssignment("G1", "T1", schedule.Saturday, 1, schedule.Always),
		makeAssignment("G1", "T1", schedule.Saturday, 2, schedule.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	// 2 пары × 200 = 400
	if score != 400 {
		t.Fatalf("expected saturday penalty 400, got %d", score)
	}
}

func TestCalculateFitness_GapPenalty(t *testing.T) {
	// Пары 1 и 3 у G1/T1 — окно SC4(10000 группа) + SC5(60 преподаватель)
	assignments := []schedule.Assignment{
		makeAssignment("G1", "T1", schedule.Monday, 1, schedule.Always),
		makeAssignment("G1", "T1", schedule.Monday, 3, schedule.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	// 1 окно группы × 10000 + 1 окно препода × 60 = 10060
	if score != 10060 {
		t.Fatalf("expected gap penalty 10060, got %d", score)
	}
}

func TestCalculateFitness_SinglePairWindow(t *testing.T) {
	// Одна пара в день — форточка +25
	assignments := []schedule.Assignment{
		makeAssignment("G1", "T1", schedule.Wednesday, 3, schedule.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	if score != 25 {
		t.Fatalf("expected single-pair penalty 25, got %d", score)
	}
}
