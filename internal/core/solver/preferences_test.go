package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestSubjectKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "пометка лекции отбрасывается",
			in:   "Высшая математика (лекция)",
			want: "высшая математика",
		},
		{
			name: "пометка практики отбрасывается",
			in:   "Высшая математика (практика)",
			want: "высшая математика",
		},
		{
			name: "название без пометки не меняется",
			in:   "Базы данных",
			want: "базы данных",
		},
		{
			name: "скобки не про вид занятия остаются",
			in:   "Физическая культура и спорт (зал)",
			want: "физическая культура и спорт (зал)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subjectKey(tt.in); got != tt.want {
				t.Errorf("получено %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestPreferencePenalties(t *testing.T) {
	plans := []domain.SubjectPlan{
		{ID: "LEC", Name: "Высшая математика (лекция)"},
		{ID: "PR", Name: "Высшая математика (практика)"},
		{ID: "LAB", Name: "Базы данных"},
	}
	lectureWed := domain.Assignment{
		GroupIDs:  []string{"G1"},
		SubjectID: "LEC",
		Type:      domain.Lecture,
		TimeSlot:  domain.MustNewTimeSlot(domain.Wednesday, 1),
	}
	practiceMon := domain.Assignment{
		GroupIDs:  []string{"G1"},
		SubjectID: "PR",
		Type:      domain.Practice,
		TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 1),
	}
	practiceFri := domain.Assignment{
		GroupIDs:  []string{"G1"},
		SubjectID: "PR",
		Type:      domain.Practice,
		TimeSlot:  domain.MustNewTimeSlot(domain.Friday, 1),
	}
	labTue1 := domain.Assignment{
		GroupIDs:  []string{"G1"},
		SubjectID: "LAB",
		Type:      domain.Lab,
		TimeSlot:  domain.MustNewTimeSlot(domain.Tuesday, 1),
	}
	labTue2 := domain.Assignment{
		GroupIDs:  []string{"G1"},
		SubjectID: "LAB",
		Type:      domain.Lab,
		TimeSlot:  domain.MustNewTimeSlot(domain.Tuesday, 2),
	}
	labThu := domain.Assignment{
		GroupIDs:  []string{"G1"},
		SubjectID: "LAB",
		Type:      domain.Lab,
		TimeSlot:  domain.MustNewTimeSlot(domain.Thursday, 1),
	}

	tests := []struct {
		name          string
		prefs         domain.SolverPreferences
		assignments   []domain.Assignment
		wantBeforeLec int
		wantSpread    int
	}{
		{
			name:          "правила выключены — штрафов нет",
			prefs:         domain.SolverPreferences{},
			assignments:   []domain.Assignment{lectureWed, practiceMon, labTue1, labThu},
			wantBeforeLec: 0,
			wantSpread:    0,
		},
		{
			name:          "практика в понедельник раньше лекции в среду — штраф",
			prefs:         domain.SolverPreferences{LectureBeforePractice: true},
			assignments:   []domain.Assignment{lectureWed, practiceMon},
			wantBeforeLec: practiceBeforeLecturePenalty,
		},
		{
			name:          "практика в пятницу после лекции в среду — без штрафа",
			prefs:         domain.SolverPreferences{LectureBeforePractice: true},
			assignments:   []domain.Assignment{lectureWed, practiceFri},
			wantBeforeLec: 0,
		},
		{
			name:        "две лабораторные в один день — без штрафа",
			prefs:       domain.SolverPreferences{SameSubjectSameDay: true},
			assignments: []domain.Assignment{labTue1, labTue2},
			wantSpread:  0,
		},
		{
			name:        "лабораторные во вторник и четверг — штраф за второй день",
			prefs:       domain.SolverPreferences{SameSubjectSameDay: true},
			assignments: []domain.Assignment{labTue1, labThu},
			wantSpread:  subjectSpreadPenalty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := domain.InputData{SubjectPlans: plans, Preferences: tt.prefs}
			gotBefore, gotSpread := preferencePenalties(tt.assignments, input)
			if gotBefore != tt.wantBeforeLec || gotSpread != tt.wantSpread {
				t.Errorf("получено (лекция=%d, разнос=%d), ожидалось (%d, %d)",
					gotBefore, gotSpread, tt.wantBeforeLec, tt.wantSpread)
			}
		})
	}
}
