package moveoptions

import (
	"context"
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/application/testfakes"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestHandle(t *testing.T) {
	mon := func(p int) domain.TimeSlot { return domain.MustNewTimeSlot(domain.Monday, p) }
	input := &testfakes.InputRepo{Data: domain.InputData{
		Groups:   []domain.Group{{ID: "G1", StudentCount: 20}, {ID: "G2", StudentCount: 20}},
		Teachers: []domain.Teacher{{ID: "T1"}, {ID: "T2"}},
		Rooms: []domain.Room{
			{ID: "R1", BuildingID: "A", Capacity: 30, Type: "lecture"},
			{ID: "R2", BuildingID: "A", Capacity: 30, Type: "lecture"},
		},
		SubjectPlans: []domain.SubjectPlan{
			{ID: "P1", TeacherID: "T1", GroupIDs: []string{"G1"}, PracticeHours: 2, RequiresRoomType: "lecture"},
			{ID: "P2", TeacherID: "T2", GroupIDs: []string{"G2"}, PracticeHours: 2, RequiresRoomType: "lecture"},
		},
	}}
	output := &testfakes.OutputRepo{Schedule: domain.Schedule{ID: 1, Assignments: []domain.Assignment{
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", SubjectID: "P1", Type: domain.Practice, TimeSlot: mon(1), Parity: domain.Always},
		// В пн-2 занята аудитория R1 (другой группой) — переносить можно, но в R2.
		{GroupIDs: []string{"G2"}, TeacherID: "T2", RoomID: "R1", SubjectID: "P2", Type: domain.Practice, TimeSlot: mon(2), Parity: domain.Always},
	}}}
	handler := NewHandler(input, output)

	opts, err := handler.Handle(context.Background(), Query{ScheduleID: 1, Index: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 36 {
		t.Fatalf("вариантов %d, ожидалось 36", len(opts))
	}
	by := map[domain.TimeSlot]Option{}
	for _, o := range opts {
		by[o.Slot] = o
	}
	if !by[mon(1)].Current {
		t.Error("пн-1 — текущее место")
	}
	if o := by[mon(2)]; o.Conflict != nil || o.RoomID != "R2" {
		t.Errorf("пн-2: ожидался перенос в R2, получено %+v", o)
	}
	if o := by[mon(3)]; o.Conflict != nil || o.RoomID != "R1" || o.LongGapsDelta != 0 {
		t.Errorf("пн-3: ожидался перенос в свою аудиторию, получено %+v", o)
	}
}
