package moveassignment

import (
	"context"
	"errors"
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/application/rules"
	"github.com/KolManis/uni-scheduler/internal/core/application/testfakes"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestHandle_TeacherAvailability(t *testing.T) {
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
			wantConflict: rules.ConflictTeacherUnavailable,
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
			input := &testfakes.InputRepo{Data: domain.InputData{
				Teachers: []domain.Teacher{
					{
						ID:               "T1",
						UnavailableSlots: []domain.TimeSlot{blocked},
					},
				},
			}}
			output := &testfakes.OutputRepo{Schedule: domain.Schedule{
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
			handler := NewHandler(input, output)

			_, err := handler.Handle(context.Background(), Command{ScheduleID: 1, Index: 0, TimeSlot: tt.newSlot})

			var conflict *rules.ConflictError
			gotConflict := ""
			if errors.As(err, &conflict) {
				gotConflict = conflict.Type
			} else if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if gotConflict != tt.wantConflict {
				t.Errorf("конфликт: получено %q, ожидалось %q", gotConflict, tt.wantConflict)
			}
			if output.Updated != tt.wantUpdated {
				t.Errorf("расписание сохранено: %v, ожидалось %v", output.Updated, tt.wantUpdated)
			}
		})
	}
}

func TestHandle_ExternalPair(t *testing.T) {
	slot := domain.MustNewTimeSlot(domain.Wednesday, 2)
	tests := []struct {
		name         string
		parity       domain.Parity
		wantConflict string
	}{
		{"пара «всегда» на время внешней нечётной — конфликт", domain.Always, rules.ConflictTeacherExternalPair},
		{"нечётная на время внешней нечётной — конфликт", domain.Odd, rules.ConflictTeacherExternalPair},
		{"чётная на время внешней нечётной — можно", domain.Even, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &testfakes.InputRepo{Data: domain.InputData{Teachers: []domain.Teacher{{
				ID:            "T1",
				ExternalPairs: []domain.ExternalPair{{TimeSlot: slot, Parity: domain.Odd, Note: "ФИТ, 305"}},
			}}}}
			output := &testfakes.OutputRepo{Schedule: domain.Schedule{ID: 1, Assignments: []domain.Assignment{{
				GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
				TimeSlot: domain.MustNewTimeSlot(domain.Monday, 1), Parity: domain.Always,
			}}}}
			handler := NewHandler(input, output)

			_, err := handler.Handle(context.Background(), Command{ScheduleID: 1, Index: 0, TimeSlot: slot, Parity: tt.parity})

			var conflict *rules.ConflictError
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
