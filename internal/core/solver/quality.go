package solver

import (
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// CalculateQuality — показатели в штуках, отдельно для чётной и нечётной недели:
// у каждой недели своё расписание (пары «всегда» плюс пары своей чётности).
func CalculateQuality(assignments []domain.Assignment) domain.WeekQuality {
	return domain.WeekQuality{
		Even: weekQuality(assignmentsInWeek(assignments, domain.Even)),
		Odd:  weekQuality(assignmentsInWeek(assignments, domain.Odd)),
	}
}

// weekQuality — показатели одной недели; на вход только её пары.
func weekQuality(assignments []domain.Assignment) domain.QualityStats {
	groupPairs := make(map[dayKey]map[int]bool)   // группа и день → номера пар
	teacherPairs := make(map[dayKey]map[int]bool) // преподаватель и день → номера пар

	var stats domain.QualityStats
	saturdaySlots := make(map[domain.TimeSlot]map[string]bool)
	for _, a := range assignments {
		day, pair := a.TimeSlot.Day(), a.TimeSlot.PairNum()
		for _, gid := range a.GroupIDs {
			addPair(groupPairs, dayKey{gid, day}, pair)
		}
		addPair(teacherPairs, dayKey{a.TeacherID, day}, pair)

		if day == domain.Saturday {
			if saturdaySlots[a.TimeSlot] == nil {
				saturdaySlots[a.TimeSlot] = make(map[string]bool)
			}
			if !saturdaySlots[a.TimeSlot][a.TeacherID] {
				saturdaySlots[a.TimeSlot][a.TeacherID] = true
				stats.SaturdayPairs++
			}
		}
	}

	for _, pairs := range groupPairs {
		n := len(pairs)
		if n == 1 {
			stats.SingleClassDays++
		}
		if n > stats.MaxGroupPairsPerDay {
			stats.MaxGroupPairsPerDay = n
		}
		sorted := sortedPairs(pairs)
		stats.GroupGaps += gapsIn(sorted)
		stats.GroupLongGaps += longGapsIn(sorted)
	}
	for _, pairs := range teacherPairs {
		if len(pairs) > stats.MaxTeacherPairsInDay {
			stats.MaxTeacherPairsInDay = len(pairs)
		}
	}
	return stats
}

// sortedPairs — номера пар из множества по возрастанию.
func sortedPairs(pairs map[int]bool) []int {
	nums := make([]int, 0, len(pairs))
	for p := range pairs {
		nums = append(nums, p)
	}
	sort.Ints(nums)
	return nums
}

// dayKey — день группы или преподавателя.
type dayKey struct {
	id  string
	day domain.Day
}

// addPair отмечает пару pair в дне key.
func addPair(days map[dayKey]map[int]bool, key dayKey, pair int) {
	if days[key] == nil {
		days[key] = make(map[int]bool)
	}
	days[key][pair] = true
}
