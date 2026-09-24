package solver

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Эталонные тесты фиксируют ТОЧНОЕ поведение детерминированных частей солвера — штрафной
// функции и сходимости операторов. Нужны как страховка при рефакторинге: инварианты
// (нет накладок, score не хуже) изменение алгоритма пропустят, а эти — нет.
//
// Если эталон разошёлся после намеренного изменения алгоритма — пересчитать ожидаемые
// значения и объяснить в коммите, почему поведение поменялось.

// goldenAssignments — вход, в котором есть всё, что штрафует функция качества: окна у групп
// и преподавателей, суббота, переход между корпусами вплотную, чётность, «форточка».
func goldenAssignments() []domain.Assignment {
	mk := func(group, teacher, room, building string, day domain.Day, pair int, p domain.Parity) domain.Assignment {
		return domain.Assignment{
			GroupIDs:   []string{group},
			TeacherID:  teacher,
			RoomID:     room,
			SubjectID:  "S-" + group + "-" + teacher,
			Type:       domain.Practice,
			TimeSlot:   domain.MustNewTimeSlot(day, pair),
			Parity:     p,
			BuildingID: building,
		}
	}
	return []domain.Assignment{
		// G1: окно на 2-й паре в понедельник, переход в другой корпус вплотную во вторник
		mk("G1", "T1", "R1", "A", domain.Monday, 1, domain.Always),
		mk("G1", "T2", "R2", "A", domain.Monday, 3, domain.Always),
		mk("G1", "T1", "R1", "A", domain.Tuesday, 1, domain.Always),
		mk("G1", "T3", "R9", "B", domain.Tuesday, 2, domain.Always),
		// G2: одна пара в субботу, «форточка» в среду
		mk("G2", "T2", "R2", "A", domain.Saturday, 1, domain.Always),
		mk("G2", "T3", "R3", "A", domain.Wednesday, 4, domain.Always),
		mk("G2", "T1", "R1", "A", domain.Thursday, 2, domain.Always),
		mk("G2", "T1", "R1", "A", domain.Thursday, 5, domain.Always),
		// G3: разделение по чётности в одном слоте — законно
		mk("G3", "T2", "R2", "A", domain.Friday, 2, domain.Odd),
		mk("G3", "T3", "R3", "A", domain.Friday, 2, domain.Even),
		mk("G3", "T3", "R3", "A", domain.Friday, 4, domain.Always),
	}
}

// renderSchedule — компактное каноническое представление: порядок не зависит от
// порядка в слайсе, поэтому сравнивается само расписание, а не его внутреннее устройство.
func renderSchedule(assignments []domain.Assignment) string {
	lines := make([]string, 0, len(assignments))
	for _, a := range assignments {
		lines = append(lines, fmt.Sprintf("%s|%s|%s|%d|%s",
			strings.Join(a.GroupIDs, ","), a.TeacherID, a.TimeSlot.Day(), a.TimeSlot.PairNum(), a.Parity))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestGolden_FitnessBreakdown(t *testing.T) {
	// 4 окна у групп: G1 пн 1–3, G2 чт 2–5 (два), G3 пт 2–4.
	// Суббота: 200 за сам факт + 8000 за единственную пару у G2 в этот день.
	// SingleClassDay: G2 в среду (1 пара) и G2 в субботу (1 пара) — 2 дня × 12000.
	// GroupLongGaps: у G2 в четверг пары 2 и 5 — окно в две пары подряд (HC8), 1 000 000.
	// Штрафы считаются по чётной и нечётной неделе и складываются (ADR-0020): всё, что
	// стоит «каждую неделю», учтено дважды. Окно у T3 в пятницу (2-я пара по чётным,
	// 4-я всегда) есть только в чётную неделю и учтено один раз.
	want := domain.FitnessBreakdown{
		Saturday:             16400,
		GroupDayOverload:     0,
		GroupLongDay:         0,
		GroupTooFewDays:      400,
		TeacherDayOverload:   0,
		TeacherConcentration: 0,
		GroupGaps:            80000,
		TeacherGaps:          300,
		BuildingTransitions:  4000,
		SingleClassDay:       48000,
		GroupLongGaps:        2000000,
	}

	got := CalculateFitnessBreakdown(goldenAssignments(), domain.InputData{})

	if got != want {
		t.Errorf("разбивка штрафа изменилась\nполучено: %+v\nожидалось: %+v", got, want)
	}
	if got.Total() != 2149100 {
		t.Errorf("итоговый штраф: получено %d, ожидалось 2149100", got.Total())
	}
}

func TestGolden_Converge(t *testing.T) {
	// Сходимость убирает окно в две пары у G2 (HC8), окна, субботу и дни с одной парой.
	// G1 собирается в один день из четырёх пар (800 за перегрузку дня + 600 за «мало дней» в каждую из двух недель):
	// с дорогим днём с одной парой (ADR-0016) до двух дней по две пары отсюда одиночными
	// ходами не дойти. У G3 пара по нечётным и пара по чётным стоят в одном слоте
	// («мигалка»), следом пара «всегда»: в обе недели у G3 две пары подряд.
	want := strings.Join([]string{
		"G1|T1|tuesday|3|always",
		"G1|T1|tuesday|4|always",
		"G1|T2|tuesday|2|always",
		"G1|T3|tuesday|1|always",
		"G2|T1|monday|3|always",
		"G2|T1|wednesday|4|always",
		"G2|T2|wednesday|3|always",
		"G2|T3|monday|2|always",
		"G3|T2|friday|3|odd",
		"G3|T3|friday|3|even",
		"G3|T3|friday|4|always",
	}, "\n")

	got := converge(goldenAssignments(), domain.InputData{}, time.Now().Add(10*time.Second), nil)

	if gotRender := renderSchedule(got); gotRender != want {
		t.Errorf("результат сходимости изменился\nполучено:\n%s\n\nожидалось:\n%s", gotRender, want)
	}
	if score := calculateFitness(got, domain.InputData{}); score != 7200 {
		t.Errorf("штраф после сходимости: получено %d, ожидалось 7200", score)
	}
	if !checkHardConstraints(got, nil) {
		t.Error("сходимость нарушила жёсткие ограничения")
	}
}
