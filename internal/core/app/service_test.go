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
