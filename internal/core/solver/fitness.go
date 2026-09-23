package solver

import (
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// teacherMaxPairsPerDay — сколько пар в день у преподавателя считается нормой.
const teacherMaxPairsPerDay = 4

// CalculateFitness — публичная обёртка для использования из других пакетов.
func CalculateFitness(assignments []domain.Assignment, input domain.InputData) int {
	return calculateFitness(assignments, input)
}

func calculateFitness(assignments []domain.Assignment, input domain.InputData) int {
	return CalculateFitnessBreakdown(assignments, input).Total()
}

// CalculateFitnessBreakdown — та же логика, что calculateFitness, но с разбивкой по категориям.
func CalculateFitnessBreakdown(assignments []domain.Assignment, input domain.InputData) domain.FitnessBreakdown {
	var b domain.FitnessBreakdown

	groupSlots := make(map[string]map[domain.Day][]int)
	teacherSlots := make(map[string]map[domain.Day][]int)

	// Дедупликация по (group, day, pairNum): чётные и нечётные пары
	// в один и тот же слот не создают реального конфликта.
	type slotKey struct {
		id  string
		day domain.Day
		num int
	}
	seenGroup := make(map[slotKey]bool)
	seenTeacher := make(map[slotKey]bool)

	for _, a := range assignments {
		for _, gid := range a.GroupIDs {
			k := slotKey{gid, a.TimeSlot.Day(), a.TimeSlot.PairNum()}
			if !seenGroup[k] {
				seenGroup[k] = true
				if groupSlots[gid] == nil {
					groupSlots[gid] = make(map[domain.Day][]int)
				}
				groupSlots[gid][a.TimeSlot.Day()] = append(
					groupSlots[gid][a.TimeSlot.Day()],
					a.TimeSlot.PairNum(),
				)
			}
		}

		k := slotKey{a.TeacherID, a.TimeSlot.Day(), a.TimeSlot.PairNum()}
		if !seenTeacher[k] {
			seenTeacher[k] = true
			if teacherSlots[a.TeacherID] == nil {
				teacherSlots[a.TeacherID] = make(map[domain.Day][]int)
			}
			teacherSlots[a.TeacherID][a.TimeSlot.Day()] = append(
				teacherSlots[a.TeacherID][a.TimeSlot.Day()],
				a.TimeSlot.PairNum(),
			)
		}
	}

	// 1. Штраф за субботу + одиночная суббота у группы
	satGroupCount := make(map[string]int)
	for _, a := range assignments {
		if a.TimeSlot.Day() == domain.Saturday {
			b.Saturday += 200
			for _, gid := range a.GroupIDs {
				satGroupCount[gid]++
			}
		}
	}
	for _, cnt := range satGroupCount {
		if cnt == 1 {
			b.Saturday += 8000 // одна пара в субботу — нежелательно
		}
	}

	// 1b. Штраф за перегрузку дня у группы (видимый для or-opt)
	for _, daySlots := range groupSlots {
		for _, slots := range daySlots {
			n := len(slots)
			switch {
			case n >= 5:
				b.GroupDayOverload += (n-4)*4000 + 2000
			case n == 4:
				b.GroupDayOverload += 800
			}
		}
	}

	// 2. Штраф за длинный день (>4 пар)
	for _, daySlots := range groupSlots {
		for _, slots := range daySlots {
			if len(slots) > 4 {
				sorted := make([]int, len(slots))
				copy(sorted, slots)
				sort.Ints(sorted)

				consecutive := 0
				for i := 0; i < len(sorted)-1; i++ {
					if sorted[i+1]-sorted[i] == 1 {
						consecutive++
					}
				}
				if consecutive >= 4 {
					b.GroupLongDay += 500
				} else if len(slots) >= 5 {
					b.GroupLongDay += 300
				}
			}
		}
	}

	// 3. Штраф за слишком мало дней (группы)
	for _, daySlots := range groupSlots {
		totalPairs := 0
		for _, slots := range daySlots {
			totalPairs += len(slots)
		}
		if totalPairs >= 4 && len(daySlots) < 2 {
			b.GroupTooFewDays += 600
		} else if totalPairs >= 4 && len(daySlots) < 3 {
			b.GroupTooFewDays += 200
		}
	}

	// 3b. Нагрузка преподавателей. 3–4 пары в день — норма; каждая пара сверх
	// teacherMaxPairsPerDay — перегрузка. Раньше штраф начинался с 3-й пары и вместе
	// со штрафами групп растаскивал занятия по неделе, порождая дни с одной парой.
	for _, daySlots := range teacherSlots {
		for _, slots := range daySlots {
			if len(slots) > teacherMaxPairsPerDay {
				b.TeacherDayOverload += (len(slots) - teacherMaxPairsPerDay) * 3000
			}
		}
		// Штраф за концентрацию: если кол-во активных дней < 3 при >=4 парах в неделю
		totalPairs := 0
		for _, slots := range daySlots {
			totalPairs += len(slots)
		}
		if totalPairs >= 4 && len(daySlots) < 3 {
			b.TeacherConcentration += (3 - len(daySlots)) * 400
		}
	}

	// 4. Штраф за окна у групп
	for _, daySlots := range groupSlots {
		for _, slots := range daySlots {
			if len(slots) >= 2 {
				sorted := make([]int, len(slots))
				copy(sorted, slots)
				sort.Ints(sorted)
				gaps := (sorted[len(sorted)-1] - sorted[0] + 1) - len(slots)
				b.GroupGaps += gaps * 10000
			}
		}
	}

	// 5. Штраф за окна у преподавателей
	for _, daySlots := range teacherSlots {
		for _, slots := range daySlots {
			if len(slots) >= 2 {
				sorted := make([]int, len(slots))
				copy(sorted, slots)
				sort.Ints(sorted)
				gaps := (sorted[len(sorted)-1] - sorted[0] + 1) - len(slots)
				b.TeacherGaps += gaps * 60
			}
		}
	}

	// 6. Штраф за переходы между корпусами.
	// Строим map (gid, day, pairNum) → buildingID за один проход вместо O(n³).
	type gSlotKey struct {
		gid string
		day domain.Day
		num int
	}
	// Спортивные места (зал, стадион) в переходах не участвуют — см. isSportRoomType.
	sportRooms := make(map[string]bool)
	for _, r := range input.Rooms {
		if isSportRoomType(r.Type) {
			sportRooms[r.ID] = true
		}
	}
	groupBuilding := make(map[gSlotKey]string, len(assignments)*2)
	for _, a := range assignments {
		if sportRooms[a.RoomID] {
			continue
		}
		for _, gid := range a.GroupIDs {
			k := gSlotKey{gid, a.TimeSlot.Day(), a.TimeSlot.PairNum()}
			if groupBuilding[k] == "" {
				groupBuilding[k] = a.BuildingID
			}
		}
	}
	for gid, daySlots := range groupSlots {
		for day, slots := range daySlots {
			if len(slots) < 2 {
				continue
			}
			sorted := make([]int, len(slots))
			copy(sorted, slots)
			sort.Ints(sorted)

			for i := 0; i < len(sorted)-1; i++ {
				b1 := groupBuilding[gSlotKey{gid, day, sorted[i]}]
				b2 := groupBuilding[gSlotKey{gid, day, sorted[i+1]}]
				if b1 == "" || b2 == "" || b1 == b2 {
					continue
				}
				diff := sorted[i+1] - sorted[i]
				switch diff {
				case 1: // вплотную — критичный переход
					b.BuildingTransitions += 2000
				case 2: // через одно окно — есть время добраться
					b.BuildingTransitions += 700
				}
			}
		}
	}

	// 7. Штраф за одну пару в день у группы — ехать ради одной пары.
	// Должен быть меньше половины штрафа за окно (10000): иначе локальный поиск
	// выгодно склеивает два одиночных дня в один день с окном (−2×штраф +10000),
	// что для студентов не лучше. При 8000 так и происходило: окон стало в 10 раз
	// больше, одиночных дней почти не убавилось.
	for _, daySlots := range groupSlots {
		for _, slots := range daySlots {
			if len(slots) == 1 {
				b.SingleClassDay += 4000
			}
		}
	}

	b.PracticeBeforeLecture, b.SubjectSpread = preferencePenalties(assignments, input)

	return b
}
