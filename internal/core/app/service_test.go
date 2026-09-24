package app

import (
	"context"
	"errors"
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

type fakeInputRepo struct {
	data domain.InputData
}

func (f *fakeInputRepo) LoadInput(context.Context) (*domain.InputData, error) {
	data := f.data
	return &data, nil
}

type fakeOutputRepo struct {
	sched   domain.Schedule
	updated bool
}

func (f *fakeOutputRepo) SaveSchedule(context.Context, *domain.Schedule) (*domain.Schedule, error) {
	return nil, errors.New("not used")
}

func (f *fakeOutputRepo) GetSchedule(context.Context, int64) (*domain.Schedule, error) {
	s := f.sched
	s.Assignments = append([]domain.Assignment(nil), f.sched.Assignments...)
	return &s, nil
}

func (f *fakeOutputRepo) ListSchedules(context.Context) ([]domain.ScheduleSummary, error) {
	return nil, nil
}

func (f *fakeOutputRepo) DeleteSchedule(context.Context, int64) error { return nil }

func (f *fakeOutputRepo) UpdateSchedule(context.Context, *domain.Schedule) error {
	f.updated = true
	return nil
}

func TestPatchAssignment_TeacherAvailability(t *testing.T) {
	blocked := domain.MustNewTimeSlot(domain.Tuesday, 3)

	tests := []struct {
		name         string
		newSlot      domain.TimeSlot
		wantConflict string
		wantUpdated  bool
	}{
		{
			name:         "перенос в недоступный слот преподавателя — конфликт, расписание не меняется",
			newSlot:      blocked,
			wantConflict: ConflictTeacherUnavailable,
			wantUpdated:  false,
		},
		{
			name:        "перенос в доступный свободный слот — сохраняется",
			newSlot:     domain.MustNewTimeSlot(domain.Tuesday, 4),
			wantUpdated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &fakeInputRepo{data: domain.InputData{
				Teachers: []domain.Teacher{
					{
						ID:               "T1",
						UnavailableSlots: []domain.TimeSlot{blocked},
					},
				},
			}}
			output := &fakeOutputRepo{sched: domain.Schedule{
				ID: 1,
				Assignments: []domain.Assignment{
					{
						GroupIDs:  []string{"G1"},
						TeacherID: "T1",
						RoomID:    "R1",
						TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 1),
						Parity:    domain.Always,
					},
				},
			}}
			svc := NewService(input, output, nil)

			_, err := svc.PatchAssignment(context.Background(), 1, 0, PatchRequest{TimeSlot: tt.newSlot})

			var conflict *ConflictError
			gotConflict := ""
			if errors.As(err, &conflict) {
				gotConflict = conflict.Type
			} else if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if gotConflict != tt.wantConflict {
				t.Errorf("конфликт: получено %q, ожидалось %q", gotConflict, tt.wantConflict)
			}
			if output.updated != tt.wantUpdated {
				t.Errorf("расписание сохранено: %v, ожидалось %v", output.updated, tt.wantUpdated)
			}
		})
	}
}

func TestPatchAssignment_ExternalPair(t *testing.T) {
	slot := domain.MustNewTimeSlot(domain.Wednesday, 2)
	tests := []struct {
		name         string
		parity       domain.Parity
		wantConflict string
	}{
		{"пара «всегда» на время внешней нечётной — конфликт", domain.Always, ConflictTeacherExternalPair},
		{"нечётная на время внешней нечётной — конфликт", domain.Odd, ConflictTeacherExternalPair},
		{"чётная на время внешней нечётной — можно", domain.Even, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &fakeInputRepo{data: domain.InputData{Teachers: []domain.Teacher{{
				ID:            "T1",
				ExternalPairs: []domain.ExternalPair{{TimeSlot: slot, Parity: domain.Odd, Note: "ФИТ, 305"}},
			}}}}
			output := &fakeOutputRepo{sched: domain.Schedule{ID: 1, Assignments: []domain.Assignment{{
				GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
				TimeSlot: domain.MustNewTimeSlot(domain.Monday, 1), Parity: domain.Always,
			}}}}
			svc := NewService(input, output, nil)

			_, err := svc.PatchAssignment(context.Background(), 1, 0, PatchRequest{TimeSlot: slot, Parity: tt.parity})

			var conflict *ConflictError
			got := ""
			if errors.As(err, &conflict) {
				got = conflict.Type
				if conflict.Detail != "ФИТ, 305" || conflict.Parity != domain.Odd {
					t.Errorf("в конфликте нет данных внешней пары: %+v", conflict)
				}
			} else if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if got != tt.wantConflict {
				t.Errorf("конфликт: получено %q, ожидалось %q", got, tt.wantConflict)
			}
		})
	}
}

func TestMoveOptions(t *testing.T) {
	mon := func(p int) domain.TimeSlot { return domain.MustNewTimeSlot(domain.Monday, p) }
	input := &fakeInputRepo{data: domain.InputData{
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
	output := &fakeOutputRepo{sched: domain.Schedule{ID: 1, Assignments: []domain.Assignment{
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", SubjectID: "P1", Type: domain.Practice, TimeSlot: mon(1), Parity: domain.Always},
		// В пн-2 занята аудитория R1 (другой группой) — переносить можно, но в R2.
		{GroupIDs: []string{"G2"}, TeacherID: "T2", RoomID: "R1", SubjectID: "P2", Type: domain.Practice, TimeSlot: mon(2), Parity: domain.Always},
	}}}
	svc := NewService(input, output, nil)

	opts, err := svc.MoveOptions(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 36 {
		t.Fatalf("вариантов %d, ожидалось 36", len(opts))
	}
	by := map[domain.TimeSlot]MoveOption{}
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
