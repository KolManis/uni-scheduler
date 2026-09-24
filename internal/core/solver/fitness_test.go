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

func TestCalculateFitness_BuildingTransitionSkipsSport(t *testing.T) {
	input := domain.InputData{
		Rooms: []domain.Room{
			{ID: "R-main", BuildingID: "MAIN", Type: "lecture"},
			{ID: "R-other", BuildingID: "OTHER", Type: "lecture"},
			{ID: "R-stadium", BuildingID: "STADIUM", Type: "outdoor"},
			{ID: "R-gym", BuildingID: "OTHER", Type: "gym"},
		},
	}
	tests := []struct {
		name       string
		secondRoom string
		secondBldg string
		want       int
	}{
		{
			name:       "лекция и следом пара в другом учебном корпусе — штраф за переход",
			secondRoom: "R-other",
			secondBldg: "OTHER",
			want:       4000, // 2000 в каждую из двух недель
		},
		{
			name:       "лекция и следом физкультура на стадионе — без штрафа",
			secondRoom: "R-stadium",
			secondBldg: "STADIUM",
			want:       0,
		},
		{
			name:       "лекция и следом физкультура в спортзале другого корпуса — без штрафа",
			secondRoom: "R-gym",
			secondBldg: "OTHER",
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignments := []domain.Assignment{
				{
					GroupIDs:   []string{"G1"},
					TeacherID:  "T1",
					RoomID:     "R-main",
					BuildingID: "MAIN",
					TimeSlot:   domain.MustNewTimeSlot(domain.Monday, 1),
					Parity:     domain.Always,
				},
				{
					GroupIDs:   []string{"G1"},
					TeacherID:  "T2",
					RoomID:     tt.secondRoom,
					BuildingID: tt.secondBldg,
					TimeSlot:   domain.MustNewTimeSlot(domain.Monday, 2),
					Parity:     domain.Always,
				},
			}
			got := CalculateFitnessBreakdown(assignments, input).BuildingTransitions
			if got != tt.want {
				t.Errorf("BuildingTransitions: получено %d, ожидалось %d", got, tt.want)
			}
		})
	}
}

func TestCalculateFitness_TeacherDayOverload(t *testing.T) {
	tests := []struct {
		name  string
		pairs int
		want  int
	}{
		{
			name:  "3 пары в день у преподавателя — норма, без штрафа",
			pairs: 3,
			want:  0,
		},
		{
			name:  "4 пары в день у преподавателя — норма, без штрафа",
			pairs: 4,
			want:  0,
		},
		{
			name:  "5 пар в день у преподавателя — перегрузка на одну пару",
			pairs: 5,
			want:  6000, // 3000 в каждую из двух недель
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var assignments []domain.Assignment
			for pair := 1; pair <= tt.pairs; pair++ {
				assignments = append(assignments, makeAssignment("G1", "T1", domain.Monday, pair, domain.Always))
			}
			got := CalculateFitnessBreakdown(assignments, emptyInput).TeacherDayOverload
			if got != tt.want {
				t.Errorf("TeacherDayOverload: получено %d, ожидалось %d", got, tt.want)
			}
		})
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
	// 2 пары × 200 × 2 недели = 800
	if score != 800 {
		t.Fatalf("expected saturday penalty 800, got %d", score)
	}
}

func TestCalculateFitness_GapPenalty(t *testing.T) {
	// Пары 1 и 3 у G1/T1 — окно SC4(10000 группа) + SC5(60 преподаватель)
	assignments := []domain.Assignment{
		makeAssignment("G1", "T1", domain.Monday, 1, domain.Always),
		makeAssignment("G1", "T1", domain.Monday, 3, domain.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	// (1 окно группы × 10000 + 1 окно препода × 60) × 2 недели = 20120
	if score != 20120 {
		t.Fatalf("expected gap penalty 20120, got %d", score)
	}
}

func TestCalculateFitness_SinglePairWindow(t *testing.T) {
	// Одна пара в день у группы — штраф 12000, дороже окна в одну пару (ADR-0016).
	assignments := []domain.Assignment{
		makeAssignment("G1", "T1", domain.Wednesday, 3, domain.Always),
	}
	score := calculateFitness(assignments, emptyInput)
	if score != 24000 { // 12000 в каждую из двух недель
		t.Fatalf("expected single-pair penalty 24000, got %d", score)
	}
}
