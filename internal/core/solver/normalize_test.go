package solver

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// TestSolveTeacher_InputOrderDoesNotMatter — одни и те же данные в разном порядке дают
// одно и то же построение. Бюджет локального поиска почти нулевой, чтобы сравнивалось
// только детерминированное построение, без случайного улучшения.
func TestSolveTeacher_InputOrderDoesNotMatter(t *testing.T) {
	// Три одинаковые аудитории и два одинаковых преподавателя: выбор между ними
	// решается только порядком — ровно тот случай, где порядок из БД влиял на результат.
	input := domain.InputData{
		Groups: []domain.Group{
			{ID: "G1", StudentCount: 20},
			{ID: "G2", StudentCount: 20},
		},
		Teachers: []domain.Teacher{
			{ID: "T1", MaxWeeklyHours: 10},
			{ID: "T2", MaxWeeklyHours: 10},
		},
		Rooms: []domain.Room{
			{ID: "R1", BuildingID: "A", Capacity: 30, Type: "lecture"},
			{ID: "R2", BuildingID: "A", Capacity: 30, Type: "lecture"},
			{ID: "R3", BuildingID: "A", Capacity: 30, Type: "lecture"},
		},
		SubjectPlans: []domain.SubjectPlan{
			{ID: "SP1", TeacherID: "T1", GroupIDs: []string{"G1"}, LectureHours: 2, RequiresRoomType: "lecture"},
			{ID: "SP2", TeacherID: "T2", GroupIDs: []string{"G2"}, LectureHours: 2, RequiresRoomType: "lecture"},
			{ID: "SP3", TeacherID: "T1", GroupIDs: []string{"G2"}, PracticeHours: 2, RequiresRoomType: "lecture"},
		},
	}

	reversed := input
	reversed.Groups = slices.Clone(input.Groups)
	reversed.Teachers = slices.Clone(input.Teachers)
	reversed.Rooms = slices.Clone(input.Rooms)
	reversed.SubjectPlans = slices.Clone(input.SubjectPlans)
	slices.Reverse(reversed.Groups)
	slices.Reverse(reversed.Teachers)
	slices.Reverse(reversed.Rooms)
	slices.Reverse(reversed.SubjectPlans)

	tests := []struct {
		name  string
		input domain.InputData
	}{
		{
			name:  "обратный порядок всех справочников даёт то же построение",
			input: reversed,
		},
	}

	render := func(assignments []domain.Assignment) string {
		lines := make([]string, 0, len(assignments))
		for _, a := range assignments {
			lines = append(lines, fmt.Sprintf("%s|%s|%s|%s", a.SubjectID, a.Type, a.RoomID, a.TimeSlot))
		}
		sort.Strings(lines)
		return strings.Join(lines, "\n")
	}

	base, err := SolveTeacherWithBudget(input, 1000, ImproveHillClimb, 0, time.Nanosecond)
	if err != nil {
		t.Fatalf("исходный порядок: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SolveTeacherWithBudget(tt.input, 1000, ImproveHillClimb, 0, time.Nanosecond)
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if render(got.Assignments) != render(base.Assignments) {
				t.Errorf("построение зависит от порядка входных данных\nисходный:\n%s\n\nобратный:\n%s",
					render(base.Assignments), render(got.Assignments))
			}
		})
	}
}
