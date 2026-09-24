package solver

import "github.com/KolManis/uni-scheduler/internal/core/domain"

// Правила оценки расписания — единственное место, где они записаны. Полная оценка
// (fitness.go) и быстрая оценка хода (evaluator.go) обе собирают неделю группы или
// преподавателя в week и вызывают groupPenalties и teacherPenalties.
//
// Веса штрафов (подробно — docs/spec/03-algorithm.md, раздел 4).
const (
	saturdayPairPenalty     = 200       // каждая пара в субботу
	loneSaturdayPenalty     = 8000      // у группы в субботу ровно одна пара
	groupGapPenalty         = 10000     // окно в одну пару у группы
	longGapPenalty          = 1_000_000 // окно в 2+ пары подряд (HC8): только если построению было некуда деться
	singleClassDayPenalty   = 12000     // день с одной парой — дороже окна (ADR-0016)
	transitionNextPenalty   = 2000      // переход в другой корпус на следующую пару
	transitionGapPenalty    = 700       // переход в другой корпус через одну пару
	teacherGapPenalty       = 60        // окно у преподавателя
	teacherOverloadPenalty  = 3000      // каждая пара преподавателя сверх нормы в день
	teacherUndesiredPenalty = 500       // пара в нежелательное для преподавателя время (ADR-0022)
	unevenWeekPenalty       = 300       // неравномерная неделя: за каждую пару разницы сверх 1 (ADR-0022)

	// teacherMaxPairsPerDay — сколько пар в день у преподавателя — норма (ADR-0004).
	teacherMaxPairsPerDay = 4
)

// week — занятость одной группы или одного преподавателя в одну учебную неделю.
type week struct {
	busy     [numSlots]bool
	building [numSlots]string // корпус пары группы; "" — спортзал, стадион или неизвестно
	saturday int              // сколько пар в субботу
	// undesired — нежелательные слоты преподавателя (у групп пусто).
	undesired [numSlots]bool
}

// add отмечает пару в слоте slot (0..35) в корпусе building ("" — не учитывать корпус).
func (w *week) add(slot int, building string) {
	w.busy[slot] = true
	if w.building[slot] == "" {
		w.building[slot] = building
	}
	if slot/6 == saturdayIdx {
		w.saturday++
	}
}

// dayPairs — номера занятых пар дня day (0 — понедельник), от первой к последней, с нуля.
// Возвращает массив и сколько в нём пар, а не срез: оценка вызывается миллионы раз, и
// срез в куче на каждый вызов вдвое замедлял поиск.
func (w *week) dayPairs(day int) (pairs [6]int, n int) {
	for p := 0; p < 6; p++ {
		if w.busy[day*6+p] {
			pairs[n] = p
			n++
		}
	}
	return pairs, n
}

// groupPenalties — штрафы группы за неделю.
func groupPenalties(w *week) domain.FitnessBreakdown {
	var b domain.FitnessBreakdown
	if w.saturday == 1 {
		b.Saturday += loneSaturdayPenalty
	}
	days, total := 0, 0
	fewest, most := 0, 0 // меньше и больше всего пар в учебный день
	for day := 0; day < 6; day++ {
		all, n := w.dayPairs(day)
		pairs := all[:n]
		if n == 0 {
			continue
		}
		if days == 0 || n < fewest {
			fewest = n
		}
		most = max(most, n)
		days++
		total += len(pairs)
		b.GroupDayOverload += dayOverloadPenalty(len(pairs))
		b.GroupLongDay += longDayPenalty(pairs)
		b.GroupGaps += gapsIn(pairs) * groupGapPenalty
		b.GroupLongGaps += longGapsIn(pairs) * longGapPenalty
		b.BuildingTransitions += transitionsPenalty(w, day, pairs)
		if len(pairs) == 1 {
			b.SingleClassDay += singleClassDayPenalty
		}
	}
	b.GroupTooFewDays += tooFewDaysPenalty(total, days)
	b.GroupUnevenWeek += unevenWeekPenaltyFor(fewest, most)
	return b
}

// unevenWeekPenaltyFor — учебные дни группы сильно различаются по числу пар: лучше 3 и 3,
// чем 5 и 1. Разница в одну пару — норма; каждая пара сверх неё — unevenWeekPenalty.
func unevenWeekPenaltyFor(fewest, most int) int {
	if extra := most - fewest - 1; extra > 0 {
		return extra * unevenWeekPenalty
	}
	return 0
}

// teacherPenalties — штрафы преподавателя за неделю.
func teacherPenalties(w *week) domain.FitnessBreakdown {
	var b domain.FitnessBreakdown
	days, total := 0, 0
	for day := 0; day < 6; day++ {
		all, n := w.dayPairs(day)
		pairs := all[:n]
		if n == 0 {
			continue
		}
		days++
		total += len(pairs)
		if extra := len(pairs) - teacherMaxPairsPerDay; extra > 0 {
			b.TeacherDayOverload += extra * teacherOverloadPenalty
		}
		b.TeacherGaps += gapsIn(pairs) * teacherGapPenalty
		for _, p := range pairs {
			if w.undesired[day*6+p] {
				b.TeacherUndesired += teacherUndesiredPenalty
			}
		}
	}
	// Концентрация: 4+ пары в неделю уложены меньше чем в 3 дня.
	if total >= 4 && days < 3 {
		b.TeacherConcentration += (3 - days) * 400
	}
	return b
}

// dayOverloadPenalty — 4 пары у группы в день — немного, 5 и больше — сильно.
func dayOverloadPenalty(n int) int {
	switch {
	case n >= 5:
		return (n-4)*4000 + 2000
	case n == 4:
		return 800
	}
	return 0
}

// longDayPenalty — больше 4 пар за день: 500, если они идут подряд, иначе 300.
func longDayPenalty(pairs []int) int {
	if len(pairs) <= 4 {
		return 0
	}
	consecutive := 0
	for i := 1; i < len(pairs); i++ {
		if pairs[i]-pairs[i-1] == 1 {
			consecutive++
		}
	}
	if consecutive >= 4 {
		return 500
	}
	return 300
}

// tooFewDaysPenalty — нагрузка втиснута в 1–2 дня: в среднем больше 3 пар на учебный день
// (один день — 600, два — 200). 4–6 пар в два дня — не нарушение: разнести их на три дня
// можно только ценой дня с одной парой, а он намного хуже (ADR-0022).
func tooFewDaysPenalty(total, days int) int {
	if days == 0 || days >= 3 || total <= 3*days {
		return 0
	}
	if days == 1 {
		return 600
	}
	return 200
}

// transitionsPenalty — переходы группы в другой корпус между соседними парами дня.
// Пары в спортзале и на стадионе (корпус "") не считаются (ADR-0005).
func transitionsPenalty(w *week, day int, pairs []int) int {
	penalty := 0
	for i := 1; i < len(pairs); i++ {
		penalty += transitionPenalty(w.building[day*6+pairs[i-1]], w.building[day*6+pairs[i]], pairs[i]-pairs[i-1])
	}
	return penalty
}

// transitionPenalty — переход между соседними парами дня из корпуса from в корпус to,
// distance — на сколько пар дальше следующая (1 — сразу, 2 — через одну).
func transitionPenalty(from, to string, distance int) int {
	if from == "" || to == "" || from == to {
		return 0
	}
	switch distance {
	case 1:
		return transitionNextPenalty
	case 2:
		return transitionGapPenalty
	}
	return 0
}

// gapsIn — сколько пустых пар между первой и последней парой дня; pairs по возрастанию.
func gapsIn(pairs []int) int {
	if len(pairs) < 2 {
		return 0
	}
	return pairs[len(pairs)-1] - pairs[0] + 1 - len(pairs)
}

// longGapsIn — сколько в дне окон длиной 2+ пары подряд; pairs по возрастанию.
func longGapsIn(pairs []int) int {
	n := 0
	for i := 1; i < len(pairs); i++ {
		if pairs[i]-pairs[i-1] > 2 {
			n++
		}
	}
	return n
}

// undesiredSlots — нежелательные слоты каждого преподавателя: id → слот 0..35.
func undesiredSlots(input domain.InputData) map[string][numSlots]bool {
	m := map[string][numSlots]bool{}
	for _, t := range input.Teachers {
		if len(t.UndesiredSlots) == 0 {
			continue
		}
		var mask [numSlots]bool
		for _, s := range t.UndesiredSlots {
			mask[slotIndex(s)] = true
		}
		m[t.ID] = mask
	}
	return m
}
