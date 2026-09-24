package web

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/application/rules"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Пары на других факультетах попадают в свой слот и только у преподавателей этого расписания.
func TestBuildDayGroups_ExternalPairs(t *testing.T) {
	sched := &domain.Schedule{Assignments: []domain.Assignment{{
		TeacherID: "T1", TimeSlot: domain.MustNewTimeSlot(domain.Monday, 1), Parity: domain.Always,
	}}}
	ext := domain.ExternalPair{TimeSlot: domain.MustNewTimeSlot(domain.Tuesday, 3), Parity: domain.Odd, Note: "ФИТ"}
	teachers := []domain.Teacher{
		{ID: "T1", ExternalPairs: []domain.ExternalPair{ext}},
		{ID: "T2", ExternalPairs: []domain.ExternalPair{ext}}, // нет пар в расписании — не показывается
	}

	days := buildDayGroups(sched, teachers)

	if len(days) != 2 || days[1].Day != domain.Tuesday {
		t.Fatalf("ожидались понедельник и вторник, получено %+v", days)
	}
	got := days[1].Pairs[0]
	if got.PairNum != 3 || len(got.Items) != 0 || len(got.External) != 1 || got.External[0].TeacherID != "T1" {
		t.Errorf("вторник: %+v", got)
	}
}

func TestConflictMessage_ExternalPair(t *testing.T) {
	msg := conflictMessage(&rules.ConflictError{Type: rules.ConflictTeacherExternalPair, Detail: "ФИТ, 305", Parity: domain.Odd})
	want := "Конфликт: у преподавателя в это время пара на другом факультете: ФИТ, 305 (нечётная неделя)"
	if msg != want {
		t.Errorf("получено %q", msg)
	}
}
