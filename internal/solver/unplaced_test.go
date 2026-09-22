package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func TestComputeUnplaced(t *testing.T) {
	input := schedule.InputData{
		SubjectPlans: []schedule.SubjectPlan{
			{ID: "S1", LectureHours: 2, PracticeHours: 4},
			{ID: "S2", LabHours: 2},
		},
	}
	assignments := []schedule.Assignment{
		{SubjectID: "S1", Type: schedule.Lecture, TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1}},
		{SubjectID: "S1", Type: schedule.Practice, TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 2}},
		// S1 practice: только 2 из 4 часов поставлено, S2 lab вообще не поставлен
	}

	got := ComputeUnplaced(assignments, input)

	if len(got) != 2 {
		t.Fatalf("expected 2 unplaced items, got %d: %+v", len(got), got)
	}

	byKey := map[string]schedule.UnplacedItem{}
	for _, u := range got {
		byKey[u.SubjectID+string(u.Type)] = u
	}

	if u, ok := byKey["S1"+string(schedule.Practice)]; !ok || u.MissingHours != 2 {
		t.Fatalf("expected S1 practice missing 2 hours, got %+v (ok=%v)", u, ok)
	}
	if u, ok := byKey["S2"+string(schedule.Lab)]; !ok || u.MissingHours != 2 {
		t.Fatalf("expected S2 lab missing 2 hours, got %+v (ok=%v)", u, ok)
	}
	if _, ok := byKey["S1"+string(schedule.Lecture)]; ok {
		t.Fatalf("S1 lecture was fully placed, should not appear in unplaced")
	}
}

func TestComputeUnplaced_AllPlaced(t *testing.T) {
	input := schedule.InputData{
		SubjectPlans: []schedule.SubjectPlan{{ID: "S1", LectureHours: 2}},
	}
	assignments := []schedule.Assignment{
		{SubjectID: "S1", Type: schedule.Lecture, TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1}},
	}
	got := ComputeUnplaced(assignments, input)
	if len(got) != 0 {
		t.Fatalf("expected no unplaced items, got %+v", got)
	}
}
