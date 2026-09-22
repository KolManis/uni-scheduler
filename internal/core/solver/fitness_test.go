package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func makeAssignment(groupID, teacherID string, day domain.Day, pair int, parity domain.Parity) domain.Assignment {
	return domain.Assignment{
		GroupIDs:   []string{groupID},
		TeacherID:  teacherID,
		RoomID:     "R1",
		SubjectID:  "S1",
		Type:       domain.Lecture,
		TimeSlot:   domain.MustNewTimeSlot(day, pair),
		Parity:     parity,
		BuildingID: "A",
	}
}

var emptyInput = domain.InputData{}

func TestCalculateFitness_NoGaps_OnlyTeacherOverload(t *testing.T) {
	// Пары подряд: 1,2,3 — нет окон, нет субботы, один преподаватель
	assignments := []domain.Assignment{
		makeAssignment("G1", "T1", domain.Monday, 1, domain.Always),
		makeAssignment("G1", "T1", domain.Monday, 2, domain.Always),
		makeAssignment("G1", "T1", domain.Monday, 3, domain.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	// Нет окон у группы → SC4=0
	// Нет окон у препода → SC5=0
	// Препод с 3 парами в день → SC3b: (3-2)*350 = 350
	// Итого: 350
	if score != 350 {
		t.Fatalf("expected 350 (teacher overload 3b), got %d", score)
	}
}

func TestCalculateFitness_NoGaps_TwoTeachers_Zero(t *testing.T) {
	// 2 пары подряд у одной группы, но разные преподаватели — нет перегрузки
	a1 := makeAssignment("G1", "T1", domain.Monday, 1, domain.Always)
	a2 := makeAssignment("G1", "T2", domain.Monday, 2, domain.Always)
	score := calculateFitness([]domain.Assignment{a1, a2}, emptyInput)
	// Нет окон, нет субботы, каждый препод по 1 паре → 0
	if score != 0 {
		t.Fatalf("expected 0 penalty, got %d", score)
	}
}

func TestCalculateFitness_SaturdayPenalty(t *testing.T) {
	assignments := []domain.Assignment{
		makeAssignment("G1", "T1", domain.Saturday, 1, domain.Always),
		makeAssignment("G1", "T1", domain.Saturday, 2, domain.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	// 2 пары × 200 = 400
	if score != 400 {
		t.Fatalf("expected saturday penalty 400, got %d", score)
	}
}

func TestCalculateFitness_GapPenalty(t *testing.T) {
	// Пары 1 и 3 у G1/T1 — окно SC4(10000 группа) + SC5(60 преподаватель)
	assignments := []domain.Assignment{
		makeAssignment("G1", "T1", domain.Monday, 1, domain.Always),
		makeAssignment("G1", "T1", domain.Monday, 3, domain.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	// 1 окно группы × 10000 + 1 окно препода × 60 = 10060
	if score != 10060 {
		t.Fatalf("expected gap penalty 10060, got %d", score)
	}
}

func TestCalculateFitness_SinglePairWindow(t *testing.T) {
	// Одна пара в день — форточка +25
	assignments := []domain.Assignment{
		makeAssignment("G1", "T1", domain.Wednesday, 3, domain.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	if score != 25 {
		t.Fatalf("expected single-pair penalty 25, got %d", score)
	}
}
