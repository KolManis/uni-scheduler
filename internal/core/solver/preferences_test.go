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
	at := func(subject string, typ domain.ClassType, day domain.Day, pair int) domain.Assignment {
		return domain.Assignment{
			GroupIDs:  []string{"G1"},
			SubjectID: subject,
			Type:      typ,
			TimeSlot:  domain.MustNewTimeSlot(day, pair),
		}
	}

	tests := []struct {
		name        string
		prefs       domain.SolverPreferences
		assignments []domain.Assignment
		want        prefPenalties
	}{
		{
			name:  "правила выключены — штрафов нет",
			prefs: domain.SolverPreferences{},
			assignments: []domain.Assignment{
				at("LEC", domain.Lecture, domain.Wednesday, 1),
				at("PR", domain.Practice, domain.Monday, 1),
				at("LAB", domain.Lab, domain.Tuesday, 1),
				at("LAB", domain.Lab, domain.Thursday, 1),
			},
			want: prefPenalties{},
		},
		{
			name:  "в один день: лекция 1-й парой, практика 2-й — без штрафа",
			prefs: domain.SolverPreferences{LecturePracticeSameDay: true},
			assignments: []domain.Assignment{
				at("LEC", domain.Lecture, domain.Wednesday, 1),
				at("PR", domain.Practice, domain.Wednesday, 2),
			},
			want: prefPenalties{},
		},
		{
			name:  "в один день: практика раньше лекции в тот же день — штраф",
			prefs: domain.SolverPreferences{LecturePracticeSameDay: true},
			assignments: []domain.Assignment{
				at("LEC", domain.Lecture, domain.Wednesday, 3),
				at("PR", domain.Practice, domain.Wednesday, 2),
			},
			want: prefPenalties{LecturePracticeApart: lecturePracticeApartPenalty},
		},
		{
			name:  "в один день: практика в другой день после лекции — штраф",
			prefs: domain.SolverPreferences{LecturePracticeSameDay: true},
			assignments: []domain.Assignment{
				at("LEC", domain.Lecture, domain.Wednesday, 1),
				at("PR", domain.Practice, domain.Friday, 1),
			},
			want: prefPenalties{LecturePracticeApart: lecturePracticeApartPenalty},
		},
		{
			name:  "в неделе: практика в понедельник раньше лекции в среду — штраф",
			prefs: domain.SolverPreferences{LectureBeforePractice: true},
			assignments: []domain.Assignment{
				at("LEC", domain.Lecture, domain.Wednesday, 1),
				at("PR", domain.Practice, domain.Monday, 1),
			},
			want: prefPenalties{PracticeBeforeLecture: practiceBeforeLecturePenalty},
		},
		{
			name:  "в неделе: практика в пятницу после лекции в среду — без штрафа",
			prefs: domain.SolverPreferences{LectureBeforePractice: true},
			assignments: []domain.Assignment{
				at("LEC", domain.Lecture, domain.Wednesday, 1),
				at("PR", domain.Practice, domain.Friday, 1),
			},
			want: prefPenalties{},
		},
		{
			name:  "две лабораторные в один день — без штрафа",
			prefs: domain.SolverPreferences{SameSubjectSameDay: true},
			assignments: []domain.Assignment{
				at("LAB", domain.Lab, domain.Tuesday, 1),
				at("LAB", domain.Lab, domain.Tuesday, 2),
			},
			want: prefPenalties{},
		},
		{
			name:  "лабораторные во вторник и четверг — штраф за второй день",
			prefs: domain.SolverPreferences{SameSubjectSameDay: true},
			assignments: []domain.Assignment{
				at("LAB", domain.Lab, domain.Tuesday, 1),
				at("LAB", domain.Lab, domain.Thursday, 1),
			},
			want: prefPenalties{SubjectSpread: subjectSpreadPenalty},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := domain.InputData{SubjectPlans: plans, Preferences: tt.prefs}
			if got := preferencePenalties(tt.assignments, input); got != tt.want {
				t.Errorf("получено %+v, ожидалось %+v", got, tt.want)
			}
		})
	}
}
