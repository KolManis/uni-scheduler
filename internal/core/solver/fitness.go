package solver

import (
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// CalculateFitness — публичная обёртка для использования из других пакетов.
func CalculateFitness(assignments []domain.Assignment, input domain.InputData) int {
	return calculateFitness(assignments, input)
}

func calculateFitness(assignments []domain.Assignment, input domain.InputData) int {
	return CalculateFitnessBreakdown(assignments, input).Total()
}

// CalculateFitnessBreakdown — score с разбивкой по категориям.
//
// Учебные недели бывают чётные и нечётные, и у каждой своё расписание: пары «всегда»
// плюс пары своей чётности. Окна, дни с одной парой, перегрузки считаются для каждой
// недели отдельно, итог — среднее по двум неделям. Раньше обе недели сливались в одну:
// пара «только по чётным» закрывала слот и в нечётную неделю, где он на самом деле пуст,
// и реальные окна и форточки были не видны (на реальных данных: оценка видела 2 окна
// и 0 дней с одной парой, в чётной неделе их было 13 и 40).
//
// Если все пары «всегда», обе недели одинаковы и результат совпадает с прежним.
func CalculateFitnessBreakdown(assignments []domain.Assignment, input domain.InputData) domain.FitnessBreakdown {
	even := weekBreakdown(assignmentsInWeek(assignments, domain.Even), input)
	odd := weekBreakdown(assignmentsInWeek(assignments, domain.Odd), input)
	b := averageBreakdown(even, odd)

	pref := preferencePenalties(assignments, input)
	b.PracticeBeforeLecture = pref.PracticeBeforeLecture
	b.LecturePracticeApart = pref.LecturePracticeApart
	b.SubjectSpread = pref.SubjectSpread
	return b
}

// inWeek — идёт ли пара с чётностью parity в неделю week (Even или Odd).
func inWeek(parity, week domain.Parity) bool {
	return parity == "" || parity == domain.Always || parity == week
}

// assignmentsInWeek — пары, которые идут в неделю week.
func assignmentsInWeek(assignments []domain.Assignment, week domain.Parity) []domain.Assignment {
	out := make([]domain.Assignment, 0, len(assignments))
	for _, a := range assignments {
		if inWeek(a.Parity, week) {
			out = append(out, a)
		}
	}
	return out
}

func averageBreakdown(even, odd domain.FitnessBreakdown) domain.FitnessBreakdown {
	avg := func(x, y int) int { return (x + y) / 2 }
	return domain.FitnessBreakdown{
		Saturday:             avg(even.Saturday, odd.Saturday),
		GroupDayOverload:     avg(even.GroupDayOverload, odd.GroupDayOverload),
		GroupLongDay:         avg(even.GroupLongDay, odd.GroupLongDay),
		GroupTooFewDays:      avg(even.GroupTooFewDays, odd.GroupTooFewDays),
		TeacherDayOverload:   avg(even.TeacherDayOverload, odd.TeacherDayOverload),
		TeacherConcentration: avg(even.TeacherConcentration, odd.TeacherConcentration),
		GroupGaps:            avg(even.GroupGaps, odd.GroupGaps),
		TeacherGaps:          avg(even.TeacherGaps, odd.TeacherGaps),
		BuildingTransitions:  avg(even.BuildingTransitions, odd.BuildingTransitions),
		SingleClassDay:       avg(even.SingleClassDay, odd.SingleClassDay),
		GroupLongGaps:        avg(even.GroupLongGaps, odd.GroupLongGaps),
	}
}

// weekBreakdown — штрафы одной учебной недели. На вход — только пары этой недели
// (assignmentsInWeek): неделя каждой группы и преподавателя собирается в week, а штрафы
// считают общие правила из penalties.go.
func weekBreakdown(assignments []domain.Assignment, input domain.InputData) domain.FitnessBreakdown {
	sportRooms := make(map[string]bool)
	for _, r := range input.Rooms {
		if isSportRoomType(r.Type) {
			sportRooms[r.ID] = true
		}
	}

	groups := map[string]*week{}
	teachers := map[string]*week{}

	var b domain.FitnessBreakdown
	for _, a := range assignments {
		slot := slotIndex(a.TimeSlot)
		building := a.BuildingID
		if sportRooms[a.RoomID] {
			building = "" // спортзал и стадион не участвуют в переходах между корпусами
		}
		for _, gid := range a.GroupIDs {
			weekFor(groups, gid).add(slot, building)
		}
		weekFor(teachers, a.TeacherID).add(slot, "")
		if a.TimeSlot.Day() == domain.Saturday {
			b.Saturday += saturdayPairPenalty
		}
	}

	for _, w := range groups {
		addBreakdown(&b, groupPenalties(w), 1)
	}
	for _, w := range teachers {
		addBreakdown(&b, teacherPenalties(w), 1)
	}
	return b
}

// weekFor — неделя группы или преподавателя id; создаётся пустой при первом обращении.
func weekFor(weeks map[string]*week, id string) *week {
	if weeks[id] == nil {
		weeks[id] = &week{}
	}
	return weeks[id]
}
