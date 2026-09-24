package app

import (
	"context"
	"strings"
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestCheckInput(t *testing.T) {
	var everySlot []domain.TimeSlot
	for _, d := range domain.AllDays {
		for p := 1; p <= 6; p++ {
			everySlot = append(everySlot, domain.MustNewTimeSlot(d, p))
		}
	}
	input := &fakeInputRepo{data: domain.InputData{
		Groups: []domain.Group{{ID: "G1", Name: "ИВТ-1", StudentCount: 50}},
		Teachers: []domain.Teacher{
			{ID: "T1", Name: "Иванов"},
			{ID: "T2", Name: "Петров", UnavailableSlots: everySlot},
		},
		Rooms: []domain.Room{{ID: "R1", Capacity: 30, Type: "lecture"}},
		SubjectPlans: []domain.SubjectPlan{
			{ID: "P1", Name: "Нет аудитории", TeacherID: "T1", GroupIDs: []string{"G1"}, PracticeHours: 2, RequiresRoomType: "lecture"},
			{ID: "P2", Name: "Нет преподавателя", TeacherID: "T9", GroupIDs: []string{"G1"}, PracticeHours: 2},
			{ID: "P3", Name: "Занятой преподаватель", TeacherID: "T2", GroupIDs: []string{"G1"}, LabHours: 3, RequiresRoomType: "lecture"},
		},
	}}
	svc := NewService(input, &fakeOutputRepo{}, nil)

	problems, err := svc.CheckInput(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range problems {
		got = append(got, p.Severity+": "+p.Object+": "+p.Message)
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{
		"error: План «Нет аудитории»: нет аудитории типа «lecture» на 50 мест",
		"error: План «Нет преподавателя»: преподаватель не найден",
		"warning: План «Занятой преподаватель»: нечётное число часов (3)",
		"error: Преподаватель Петров: по планам нужно 2 пар в неделю, а свободно только 0",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("нет проблемы %q\nнайдено:\n%s", want, all)
		}
	}
}

func TestExplainUnplaced(t *testing.T) {
	data := domain.InputData{
		Groups:   []domain.Group{{ID: "G1", StudentCount: 20}},
		Teachers: []domain.Teacher{{ID: "T1"}},
		Rooms:    []domain.Room{{ID: "R1", Capacity: 10, Type: "lab"}},
		SubjectPlans: []domain.SubjectPlan{
			{ID: "P1", TeacherID: "T1", GroupIDs: []string{"G1"}, LabHours: 2, RequiresRoomType: "lab"},
		},
	}
	items := explainUnplaced([]domain.UnplacedItem{{SubjectID: "P1", Type: domain.Lab, MissingHours: 2}}, nil, data)
	if want := "нет аудитории типа «lab» на 20 мест в допустимых корпусах"; items[0].Reason != want {
		t.Errorf("причина: %q, ожидалось %q", items[0].Reason, want)
	}
}
