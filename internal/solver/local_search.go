package solver

import (
	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

// LocalSearch улучшает расписание: сначала 2-opt swap, затем or-opt перемещение.
func LocalSearch(assignments []schedule.Assignment, input schedule.InputData) []schedule.Assignment {
	current := twoOpt(assignments, input)
	current = orOpt(current, input)
	return current
}

// twoOpt — попарный обмен слотами, устраняет окна.
func twoOpt(assignments []schedule.Assignment, input schedule.InputData) []schedule.Assignment {
	current := make([]schedule.Assignment, len(assignments))
	copy(current, assignments)
	currentScore := calculateFitness(current, input)

	improved := true
	for improved {
		improved = false
		for i := 0; i < len(current); i++ {
			for j := i + 1; j < len(current); j++ {
				if current[i].TimeSlot == current[j].TimeSlot {
					continue
				}
				swapped := swapSlots(current, i, j)
				if swapped == nil || !checkHardConstraints(swapped) {
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
func orOpt(assignments []schedule.Assignment, input schedule.InputData) []schedule.Assignment {
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
			origSlot := current[i].TimeSlot
			for _, day := range weekdays {
				if day == origSlot.Day {
					continue
				}
				for pairNum := 1; pairNum <= 6; pairNum++ {
					candidate := make([]schedule.Assignment, len(current))
					copy(candidate, current)
					candidate[i].TimeSlot = schedule.TimeSlot{Day: day, PairNum: pairNum}

					if !checkHardConstraints(candidate) {
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

// checkHardConstraints проверяет HC1–HC3 для набора назначений.
// HC4–HC9 уже были соблюдены при генерации и не нарушаются при swap слотов.
func checkHardConstraints(assignments []schedule.Assignment) bool {
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
