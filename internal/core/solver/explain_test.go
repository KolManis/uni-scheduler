package solver

import (
	"testing"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// TestExplainScore_AddsUpToScore — сумма всех нарушений равна score, а суммы по
// категориям — разбивке CalculateFitnessBreakdown. Иначе подробный разбор врёт.
func TestExplainScore_AddsUpToScore(t *testing.T) {
	cases := map[string]func() ([]domain.Assignment, domain.InputData){
		"эталонный пример": func() ([]domain.Assignment, domain.InputData) {
			return goldenAssignments(), domain.InputData{}
		},
	}
	for name, prefs := range map[string]domain.SolverPreferences{
		"реальные данные без правил":         {},
		"реальные данные со всеми правилами": {LectureBeforePractice: true, LecturePracticeSameDay: true, SameSubjectSameDay: true},
	} {
		cases[name] = func() ([]domain.Assignment, domain.InputData) {
			input := loadSnapshotInput(t)
			input.Preferences = prefs
			sched, err := Solve(input, Options{Budget: time.Nanosecond})
			if err != nil {
				t.Fatal(err)
			}
			return sched.Assignments, normalizeInput(input)
		}
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			assignments, input := build()
			byCategory := map[string]int{}
			total := 0
			for _, v := range ExplainScore(assignments, input) {
				if v.Penalty <= 0 || v.Rule == "" || v.Category == "" {
					t.Fatalf("пустое нарушение: %+v", v)
				}
				byCategory[v.Category] += v.Penalty
				total += v.Penalty
			}
			if want := calculateFitness(assignments, input); total != want {
				t.Fatalf("сумма нарушений %d, score %d", total, want)
			}
			b := CalculateFitnessBreakdown(assignments, input)
			for category, want := range map[string]int{
				"Saturday": b.Saturday, "GroupDayOverload": b.GroupDayOverload, "GroupLongDay": b.GroupLongDay,
				"GroupTooFewDays": b.GroupTooFewDays, "TeacherDayOverload": b.TeacherDayOverload,
				"TeacherConcentration": b.TeacherConcentration, "GroupGaps": b.GroupGaps, "TeacherGaps": b.TeacherGaps,
				"BuildingTransitions": b.BuildingTransitions, "SingleClassDay": b.SingleClassDay,
				"GroupLongGaps": b.GroupLongGaps, "PracticeBeforeLecture": b.PracticeBeforeLecture,
				"LecturePracticeApart": b.LecturePracticeApart, "SubjectSpread": b.SubjectSpread,
			} {
				if byCategory[category] != want {
					t.Errorf("%s: по нарушениям %d, в разбивке %d", category, byCategory[category], want)
				}
			}
		})
	}
}

// TestExplainScore_GoldenDetails — нарушения эталонного примера названы понятно.
func TestExplainScore_GoldenDetails(t *testing.T) {
	var found []string
	for _, v := range ExplainScore(goldenAssignments(), domain.InputData{}) {
		if v.Category == "GroupLongGaps" || v.Category == "SingleClassDay" {
			found = append(found, v.Rule+" | "+string(v.Week)+" | "+string(v.Day)+" | "+v.Detail)
		}
	}
	want := map[string]bool{
		"Окно в 2+ пары подряд (HC8) | even | thursday | между 2-й и 5-й парой": false,
		"День с одной парой | odd | wednesday | только 4-я пара":                false,
	}
	for _, f := range found {
		if _, ok := want[f]; ok {
			want[f] = true
		}
	}
	for w, ok := range want {
		if !ok {
			t.Errorf("нет нарушения %q среди:\n%v", w, found)
		}
	}
}

// Одна пара за неделю — день с одной парой неизбежен; две пары в разные дни — нет.
func TestExplainScore_Unavoidable(t *testing.T) {
	pair := func(group string, day domain.Day, parity domain.Parity) domain.Assignment {
		return domain.Assignment{GroupIDs: []string{group}, TeacherID: "T-" + group + string(day),
			TimeSlot: domain.MustNewTimeSlot(day, 2), Parity: parity}
	}
	assignments := []domain.Assignment{
		pair("ONE", domain.Monday, domain.Even),
		pair("TWO", domain.Monday, domain.Always), pair("TWO", domain.Tuesday, domain.Always),
	}
	for _, v := range ExplainScore(assignments, domain.InputData{}) {
		if v.Category != "SingleClassDay" {
			continue
		}
		if want := v.GroupIDs[0] == "ONE"; v.Unavoidable != want {
			t.Errorf("%v %s %s: Unavoidable = %v, want %v", v.GroupIDs, v.Week, v.Day, v.Unavoidable, want)
		}
	}
}

// Пара в нежелательное время преподавателя штрафуется в каждой неделе, когда она идёт,
// и разбор совпадает с оценкой.
func TestUndesiredSlots(t *testing.T) {
	slot := domain.MustNewTimeSlot(domain.Monday, 1)
	input := domain.InputData{Teachers: []domain.Teacher{{ID: "T1", UndesiredSlots: []domain.TimeSlot{slot}}}}
	assignments := []domain.Assignment{
		{GroupIDs: []string{"G1"}, TeacherID: "T1", TimeSlot: slot, Parity: domain.Always},
		{GroupIDs: []string{"G1"}, TeacherID: "T1", TimeSlot: domain.MustNewTimeSlot(domain.Monday, 2), Parity: domain.Always},
	}
	b := CalculateFitnessBreakdown(assignments, input)
	if b.TeacherUndesired != 2*teacherUndesiredPenalty {
		t.Errorf("TeacherUndesired = %d, want %d", b.TeacherUndesired, 2*teacherUndesiredPenalty)
	}
	sum := 0
	for _, v := range ExplainScore(assignments, input) {
		sum += v.Penalty
	}
	if sum != b.Total() {
		t.Errorf("сумма нарушений %d, score %d", sum, b.Total())
	}
	e := newEvaluator(assignments, input, nil)
	if e.score() != b.Total() {
		t.Errorf("evaluator %d, fitness %d", e.score(), b.Total())
	}
}
