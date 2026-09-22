package solver

import (
	"math/rand"
	"time"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

const (
	// localSearchTotalBudget — жёсткий потолок на ВСЮ работу LocalSearch (и обычную
	// сходимость converge, и iteratedLocalSearch поверх неё). На реальном датасете
	// (300+ занятий) один проход twoOpt/orOpt сам по себе не бесплатный — без общего
	// потолка converge в цикле "пока есть улучшение" мог разово растянуться на минуты.
	// Замерено: один проход twoOpt+orOpt на 362 занятиях сходится ~35 сек — бюджет должен
	// быть больше этого, иначе converge просто не успевает сравняться со старым качеством.
	localSearchTotalBudget = 60 * time.Second
	// localSearchMaxStagnation — если столько раундов iteratedLocalSearch подряд не дали
	// улучшения, прекращаем раньше срока (на маленьких расписаниях сходится почти мгновенно).
	localSearchMaxStagnation = 40
)

// teacherUnavailable — предвычисленные недоступные слоты преподавателей (HC7).
// Строится один раз на запуск LocalSearch: checkHardConstraints вызывается в горячем
// цикле O(n²) раз, и пересобирать карту на каждый вызов было бы непозволительно дорого.
// nil/пустая карта означает "ограничений нет" — проверка тогда ничего не стоит.
type teacherUnavailable map[string]map[schedule.TimeSlot]bool

func buildTeacherUnavailable(input schedule.InputData) teacherUnavailable {
	var m teacherUnavailable
	for _, t := range input.Teachers {
		if len(t.UnavailableSlots) == 0 {
			continue
		}
		if m == nil {
			m = make(teacherUnavailable)
		}
		slots := make(map[schedule.TimeSlot]bool, len(t.UnavailableSlots))
		for _, s := range t.UnavailableSlots {
			slots[s] = true
		}
		m[t.ID] = slots
	}
	return m
}

// LocalSearch улучшает расписание в два этапа, уложившись в localSearchTotalBudget суммарно:
//  1. converge — обычные 2-opt/or-opt по очереди до тех пор, пока хоть один из них
//     находит улучшение (одного прохода каждого не всегда достаточно: swap может
//     открыть возможность для move, и наоборот).
//  2. iteratedLocalSearch — поверх результата: случайно "встряхиваем" расписание
//     несколькими валидными обменами слотов (даже если это временно ухудшает score)
//     и снова прогоняем converge. Это даёт шанс выбраться из локального оптимума,
//     который чистый 2-opt/or-opt в принципе не видит (например когда убрать окно
//     можно, только двигая два занятия согласованно). Результат никогда не хуже
//     чистого 2-opt/or-opt — худшие попытки просто отбрасываются.
func LocalSearch(assignments []schedule.Assignment, input schedule.InputData) []schedule.Assignment {
	deadline := time.Now().Add(localSearchTotalBudget)
	unavail := buildTeacherUnavailable(input)
	current := converge(assignments, input, deadline, unavail)
	current = iteratedLocalSearch(current, input, deadline, unavail)
	return current
}

// converge крутит twoOpt и orOpt по очереди, пока очередной раунд обоих даёт
// строгое улучшение суммарного score, но не дольше deadline.
func converge(assignments []schedule.Assignment, input schedule.InputData, deadline time.Time,
	unavail teacherUnavailable) []schedule.Assignment {
	current := assignments
	currentScore := calculateFitness(current, input)
	for !time.Now().After(deadline) {
		next := twoOpt(current, input, deadline, unavail)
		next = orOpt(next, input, deadline, unavail)
		nextScore := calculateFitness(next, input)
		if nextScore >= currentScore {
			return current
		}
		current = next
		currentScore = nextScore
	}
	return current
}

// iteratedLocalSearch — классический iterated local search: возмущаем текущее лучшее
// решение случайными (но допустимыми по HC1-3) перестановками слотов, заново сходимся
// через converge, и оставляем результат, только если он строго лучше уже найденного.
func iteratedLocalSearch(assignments []schedule.Assignment, input schedule.InputData, deadline time.Time,
	unavail teacherUnavailable) []schedule.Assignment {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	best := assignments
	bestScore := calculateFitness(best, input)
	current := best

	stagnant := 0
	for stagnant < localSearchMaxStagnation && !time.Now().After(deadline) {
		perturbed := perturb(current, rng, unavail)
		reoptimized := converge(perturbed, input, deadline, unavail)
		score := calculateFitness(reoptimized, input)

		if score < bestScore {
			best = reoptimized
			bestScore = score
			current = reoptimized
			stagnant = 0
		} else {
			current = best
			stagnant++
		}
	}
	return best
}

// perturb делает несколько случайных (но проверенных на HC1-3) обменов слотами —
// "толчок", чтобы вывести поиск из локального оптимума. Может временно ухудшить score:
// это не проблема, дальше идёт полноценный converge, а итоговый результат отбирается
// в iteratedLocalSearch только если он лучше предыдущего лучшего.
func perturb(assignments []schedule.Assignment, rng *rand.Rand, unavail teacherUnavailable) []schedule.Assignment {
	current := make([]schedule.Assignment, len(assignments))
	copy(current, assignments)
	if len(current) < 2 {
		return current
	}

	kicks := 2 + rng.Intn(3) // 2-4 случайных обмена за один "толчок"
	for k := 0; k < kicks; k++ {
		for attempt := 0; attempt < 20; attempt++ {
			i := rng.Intn(len(current))
			j := rng.Intn(len(current))
			if i == j || current[i].TimeSlot == current[j].TimeSlot {
				continue
			}
			swapped := swapSlots(current, i, j)
			if swapped != nil && checkHardConstraints(swapped, unavail) {
				current = swapped
				break
			}
		}
	}
	return current
}

// twoOpt — попарный обмен слотами, устраняет окна.
func twoOpt(assignments []schedule.Assignment, input schedule.InputData, deadline time.Time,
	unavail teacherUnavailable) []schedule.Assignment {
	current := make([]schedule.Assignment, len(assignments))
	copy(current, assignments)
	currentScore := calculateFitness(current, input)

	improved := true
	for improved {
		improved = false
		for i := 0; i < len(current); i++ {
			if time.Now().After(deadline) {
				return current
			}
			for j := i + 1; j < len(current); j++ {
				if current[i].TimeSlot == current[j].TimeSlot {
					continue
				}
				swapped := swapSlots(current, i, j)
				if swapped == nil || !checkHardConstraints(swapped, unavail) {
					continue
				}
				newScore := calculateFitness(swapped, input)
				if newScore < currentScore {
					current = swapped
					currentScore = newScore
					improved = true
				}
			}
		}
	}
	return current
}

// orOpt — перемещает одно назначение в другой день/слот.
// Целенаправленно убирает перегрузку конкретных дней у групп.
func orOpt(assignments []schedule.Assignment, input schedule.InputData, deadline time.Time,
	unavail teacherUnavailable) []schedule.Assignment {
	current := make([]schedule.Assignment, len(assignments))
	copy(current, assignments)
	currentScore := calculateFitness(current, input)

	weekdays := []schedule.Day{
		schedule.Monday, schedule.Tuesday, schedule.Wednesday,
		schedule.Thursday, schedule.Friday,
	}

	improved := true
	for improved {
		improved = false
		for i := 0; i < len(current); i++ {
			if time.Now().After(deadline) {
				return current
			}
			origSlot := current[i].TimeSlot
			for _, day := range weekdays {
				if day == origSlot.Day {
					continue
				}
				for pairNum := 1; pairNum <= 6; pairNum++ {
					candidate := make([]schedule.Assignment, len(current))
					copy(candidate, current)
					candidate[i].TimeSlot = schedule.TimeSlot{Day: day, PairNum: pairNum}

					if !checkHardConstraints(candidate, unavail) {
						continue
					}
					newScore := calculateFitness(candidate, input)
					if newScore < currentScore {
						current = candidate
						currentScore = newScore
						improved = true
					}
				}
			}
		}
	}
	return current
}

// swapSlots возвращает копию assignments с переставленными TimeSlot для i и j.
// BuildingID не меняется: корпус определяется аудиторией, а не временным слотом.
func swapSlots(assignments []schedule.Assignment, i, j int) []schedule.Assignment {
	result := make([]schedule.Assignment, len(assignments))
	copy(result, assignments)
	result[i].TimeSlot = assignments[j].TimeSlot
	result[j].TimeSlot = assignments[i].TimeSlot
	return result
}

// checkHardConstraints проверяет HC1–HC3 (двойная занятость преподавателя, группы, аудитории)
// и HC7 (недоступные слоты преподавателя) для набора назначений.
//
// HC7 проверять обязательно: перестановка меняет именно временной слот, поэтому занятие
// может уехать в слот, который преподаватель отметил как недоступный. Остальные ограничения
// (тип и вместимость аудитории, корпуса) завязаны на аудиторию, а она при обмене слотами
// не меняется — они были соблюдены при генерации и остаются соблюдёнными.
//
// unavail может быть nil — тогда ограничений по доступности нет и проверка бесплатна.
func checkHardConstraints(assignments []schedule.Assignment, unavail teacherUnavailable) bool {
	type key struct {
		slot   schedule.TimeSlot
		id     string
		entity string // "teacher" | "group" | "room"
	}

	type paritySet struct {
		even, odd, always bool
	}

	occupied := make(map[key]*paritySet)

	conflicts := func(ps *paritySet, p schedule.Parity) bool {
		switch p {
		case schedule.Always:
			return ps.even || ps.odd || ps.always
		case schedule.Even:
			return ps.always || ps.even
		case schedule.Odd:
			return ps.always || ps.odd
		}
		return false
	}

	add := func(ps *paritySet, p schedule.Parity) {
		switch p {
		case schedule.Always:
			ps.always = true
		case schedule.Even:
			ps.even = true
		case schedule.Odd:
			ps.odd = true
		}
	}

	for _, a := range assignments {
		parity := a.Parity
		if parity == "" {
			parity = schedule.Always
		}

		// HC7: слот не должен быть отмечен преподавателем как недоступный
		if len(unavail) > 0 {
			if slots, ok := unavail[a.TeacherID]; ok && slots[a.TimeSlot] {
				return false
			}
		}

		// HC1: преподаватель
		tk := key{slot: a.TimeSlot, id: a.TeacherID, entity: "teacher"}
		if ps, ok := occupied[tk]; ok && conflicts(ps, parity) {
			return false
		}
		if occupied[tk] == nil {
			occupied[tk] = &paritySet{}
		}
		add(occupied[tk], parity)

		// HC2: группы
		for _, gid := range a.GroupIDs {
			gk := key{slot: a.TimeSlot, id: gid, entity: "group"}
			if ps, ok := occupied[gk]; ok && conflicts(ps, parity) {
				return false
			}
			if occupied[gk] == nil {
				occupied[gk] = &paritySet{}
			}
			add(occupied[gk], parity)
		}

		// HC3: аудитория
		rk := key{slot: a.TimeSlot, id: a.RoomID, entity: "room"}
		if ps, ok := occupied[rk]; ok && conflicts(ps, parity) {
			return false
		}
		if occupied[rk] == nil {
			occupied[rk] = &paritySet{}
		}
		add(occupied[rk], parity)
	}
	return true
}
