package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func TestLocalSearch_ImprovesGapSchedule(t *testing.T) {
	// G1: пары 1 и 3 в понедельник → окно +10000
	// после LocalSearch должен уменьшить или сохранить score
	assignments := []schedule.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S1", Type: schedule.Lecture,
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1},
			Parity:   schedule.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G1"}, TeacherID: "T2", RoomID: "R2",
			SubjectID: "S2", Type: schedule.Practice,
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 3},
			Parity:   schedule.Always, BuildingID: "A",
		},
	}

	before := calculateFitness(assignments, schedule.InputData{})
	result := LocalSearch(assignments, schedule.InputData{})
	after := calculateFitness(result, schedule.InputData{})

	if after > before {
		t.Fatalf("LocalSearch made schedule worse: before=%d after=%d", before, after)
	}
}

func TestCheckHardConstraints_NoConflict(t *testing.T) {
	assignments := []schedule.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1},
			Parity:   schedule.Always,
		},
		{
			GroupIDs: []string{"G2"}, TeacherID: "T2", RoomID: "R2",
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1},
			Parity:   schedule.Always,
		},
	}
	if !checkHardConstraints(assignments) {
		t.Fatal("different teacher+group+room at same slot should not conflict")
	}
}

func TestCheckHardConstraints_TeacherConflict(t *testing.T) {
	slot := schedule.TimeSlot{Day: schedule.Monday, PairNum: 2}
	assignments := []schedule.Assignment{
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: schedule.Always},
		{GroupIDs: []string{"G2"}, TeacherID: "T1", RoomID: "R2", TimeSlot: slot, Parity: schedule.Always},
	}
	if checkHardConstraints(assignments) {
		t.Fatal("same teacher at same slot should conflict (HC1)")
	}
}

func TestCheckHardConstraints_EvenOddNoConflict(t *testing.T) {
	slot := schedule.TimeSlot{Day: schedule.Tuesday, PairNum: 2}
	assignments := []schedule.Assignment{
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: schedule.Even},
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: schedule.Odd},
	}
	if !checkHardConstraints(assignments) {
		t.Fatal("even and odd at same slot should not conflict")
	}
}
