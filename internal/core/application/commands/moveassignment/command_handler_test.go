package moveassignment

import (
	"context"
	"errors"
	"strings"
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

// Лекция потока из трёх групп по 100 человек: аудитория при переносе проверяется по
// типу, вместимости и корпусу (HC4–HC6), как при генерации.
func TestHandle_RoomSuitability(t *testing.T) {
	input := &testfakes.InputRepo{Data: domain.InputData{
		Teachers: []domain.Teacher{
			{ID: "T1"},
		},
		Groups: []domain.Group{
			{ID: "G1", StudentCount: 100, BuildingIDs: []string{"B1"}},
			{ID: "G2", StudentCount: 100, BuildingIDs: []string{"B1"}},
			{ID: "G3", StudentCount: 100, BuildingIDs: []string{"B1"}},
		},
		SubjectPlans: []domain.SubjectPlan{
			{ID: "P1", Name: "Философия (лекция)", RequiresRoomType: "lecture"},
		},
		Rooms: []domain.Room{
			{ID: "hall", Number: "Актовый", BuildingID: "B1", Capacity: 300, Type: "lecture"},
			{ID: "hall2", Number: "Большая", BuildingID: "B1", Capacity: 320, Type: "lecture"},
			{ID: "small", Number: "101", BuildingID: "B1", Capacity: 30, Type: "lecture"},
			{ID: "lab", Number: "Л-1", BuildingID: "B1", Capacity: 300, Type: "lab"},
			{ID: "far", Number: "Б-1", BuildingID: "B2", Capacity: 300, Type: "lecture"},
		},
	}}

	tests := []struct {
		name         string
		currentRoom  string
		newRoom      string // пусто — аудитория не меняется
		wantConflict string
		wantDetail   string // часть текста причины
		wantUpdated  bool
	}{
		{
			name:         "лекция на 300 человек в аудиторию на 30 мест — нельзя",
			currentRoom:  "hall",
			newRoom:      "small",
			wantConflict: rules.ConflictRoomUnsuitable,
			wantDetail:   "мест 30, а студентов в группах 300",
		},
		{
			name:         "лекция в лабораторию — нельзя",
			currentRoom:  "hall",
			newRoom:      "lab",
			wantConflict: rules.ConflictRoomUnsuitable,
			wantDetail:   "нужна аудитория типа lecture",
		},
		{
			name:         "аудитория в корпусе, где группы не учатся, — нельзя",
			currentRoom:  "hall",
			newRoom:      "far",
			wantConflict: rules.ConflictRoomUnsuitable,
			wantDetail:   "корпус",
		},
		{
			name:         "аудитория, которой нет в справочнике, — нельзя",
			currentRoom:  "hall",
			newRoom:      "unknown",
			wantConflict: rules.ConflictRoomUnsuitable,
			wantDetail:   "нет в справочнике",
		},
		{
			name:        "подходящая аудитория — сохраняется",
			currentRoom: "hall",
			newRoom:     "hall2",
			wantUpdated: true,
		},
		{
			name:        "перенос только времени, аудитория старая и неподходящая, — сохраняется",
			currentRoom: "small",
			newRoom:     "",
			wantUpdated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &testfakes.OutputRepo{Schedule: domain.Schedule{
				ID: 1,
				Assignments: []domain.Assignment{
					{
						GroupIDs:  []string{"G1", "G2", "G3"},
						TeacherID: "T1",
						RoomID:    tt.currentRoom,
						SubjectID: "P1",
						Type:      domain.Lecture,
						TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 1),
						Parity:    domain.Always,
					},
				},
			}}
			handler := NewHandler(input, output)

			_, err := handler.Handle(context.Background(), Command{
				ScheduleID: 1,
				Index:      0,
				TimeSlot:   domain.MustNewTimeSlot(domain.Tuesday, 2),
				RoomID:     tt.newRoom,
			})

			var conflict *rules.ConflictError
			gotConflict, gotDetail := "", ""
			if errors.As(err, &conflict) {
				gotConflict, gotDetail = conflict.Type, conflict.Detail
			} else if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if gotConflict != tt.wantConflict {
				t.Errorf("конфликт: получено %q, ожидалось %q", gotConflict, tt.wantConflict)
			}
			if !strings.Contains(gotDetail, tt.wantDetail) {
				t.Errorf("причина %q не содержит %q", gotDetail, tt.wantDetail)
			}
			if output.Updated != tt.wantUpdated {
				t.Errorf("расписание сохранено: %v, ожидалось %v", output.Updated, tt.wantUpdated)
			}
		})
	}
}
