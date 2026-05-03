package solver

import (
	"sort"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func calculateFitness(assignments []schedule.Assignment, _ schedule.InputData) int {
	penalty := 0

	groupSlots := make(map[string]map[schedule.Day][]int)
	teacherSlots := make(map[string]map[schedule.Day][]int)

	for _, a := range assignments {
		for _, gid := range a.GroupIDs {
			if groupSlots[gid] == nil {
				groupSlots[gid] = make(map[schedule.Day][]int)
			}
			groupSlots[gid][a.TimeSlot.Day] = append(
				groupSlots[gid][a.TimeSlot.Day],
				a.TimeSlot.PairNum,
			)
		}

		if teacherSlots[a.TeacherID] == nil {
			teacherSlots[a.TeacherID] = make(map[schedule.Day][]int)
		}
		teacherSlots[a.TeacherID][a.TimeSlot.Day] = append(
			teacherSlots[a.TeacherID][a.TimeSlot.Day],
			a.TimeSlot.PairNum,
		)
	}

	// 1. Штраф за субботу
	for _, a := range assignments {
		if a.TimeSlot.Day == schedule.Saturday {
			penalty += 200
			break
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
					penalty += 500
				} else if len(slots) >= 5 {
					penalty += 300
				}
			}
		}
	}

	// 3. Штраф за слишком мало дней
	for _, daySlots := range groupSlots {
		totalPairs := 0
		for _, slots := range daySlots {
			totalPairs += len(slots)
		}

		if totalPairs >= 4 && len(daySlots) < 2 {
			penalty += 600
		} else if totalPairs >= 4 && len(daySlots) < 3 {
			penalty += 200
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
				penalty += gaps * 10000 // БЫЛО 100, СТАЛО 10000 — ОКНА НЕДОПУСТИМЫ
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
				penalty += gaps * 60
			}
		}
	}

	// 6. Штраф за переходы между корпусами
	for gid, daySlots := range groupSlots {
		for day, slots := range daySlots {
			if len(slots) >= 2 {
				sorted := make([]int, len(slots))
				copy(sorted, slots)
				sort.Ints(sorted)

				for i := 0; i < len(sorted)-1; i++ {
					if sorted[i+1]-sorted[i] == 1 {
						var b1, b2 string
						for _, a := range assignments {
							for _, agid := range a.GroupIDs {
								if agid == gid && a.TimeSlot.Day == day {
									if a.TimeSlot.PairNum == sorted[i] {
										b1 = a.BuildingID
									}
									if a.TimeSlot.PairNum == sorted[i+1] {
										b2 = a.BuildingID
									}
								}
							}
						}
						if b1 != "" && b2 != "" && b1 != b2 {
							penalty += 150
						}
					}
				}
			}
		}
	}

	// 7. Штраф за "форточку" (1 пара в день)
	for _, daySlots := range groupSlots {
		for _, slots := range daySlots {
			if len(slots) == 1 {
				penalty += 25
			}
		}
	}

	return penalty
}

func allSubjectsPlaced(state *solverState) bool {
	for _, plan := range state.input.SubjectPlans {
		for _, ct := range []schedule.ClassType{schedule.Lecture, schedule.Practice, schedule.Lab} {
			// ПРОПУСКАЕМ практики и лабы с несколькими группами
			if ct != schedule.Lecture && len(plan.GroupIDs) > 1 {
				continue
			}

			current := 0
			if state.subjectCount[plan.ID] != nil {
				current = state.subjectCount[plan.ID][ct]
			}

			var total int
			switch ct {
			case schedule.Lecture:
				total = plan.LectureHours
			case schedule.Practice:
				total = plan.PracticeHours
			case schedule.Lab:
				total = plan.LabHours
			}

			if current < total {
				return false
			}
		}
	}
	return true
}
