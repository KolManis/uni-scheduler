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
	type dayKey struct {
		id  string
		day domain.Day
	}
	groupPairs := make(map[dayKey]map[int]bool)
	teacherPairs := make(map[dayKey]map[int]bool)
	addPair := func(m map[dayKey]map[int]bool, k dayKey, pair int) {
		if m[k] == nil {
			m[k] = make(map[int]bool)
		}
		m[k][pair] = true
	}

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
		stats.GroupGaps += gapsIn(pairs)
		stats.GroupLongGaps += longGapsInSet(pairs)
	}
	for _, pairs := range teacherPairs {
		if len(pairs) > stats.MaxTeacherPairsInDay {
			stats.MaxTeacherPairsInDay = len(pairs)
		}
	}
	return stats
}

func gapsIn(pairs map[int]bool) int {
	if len(pairs) < 2 {
		return 0
	}
	nums := make([]int, 0, len(pairs))
	for p := range pairs {
		nums = append(nums, p)
	}
	sort.Ints(nums)
	return nums[len(nums)-1] - nums[0] + 1 - len(nums)
}

func longGapsInSet(pairs map[int]bool) int {
	nums := make([]int, 0, len(pairs))
	for p := range pairs {
		nums = append(nums, p)
	}
	sort.Ints(nums)
	return longGapsIn(nums)
}
