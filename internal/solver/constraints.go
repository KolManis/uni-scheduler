package solver

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func isSlotFree(
	slot schedule.TimeSlot,
	resourceID string,
	occupied map[schedule.TimeSlot]map[string]bool,
) bool {
	if occupied[slot] == nil {
		return true
	}
	return !occupied[slot][resourceID]
}

// isSlotFreeForAllGroups — проверяет, что ВСЕ группы потока свободны в этом слоте
func isSlotFreeForAllGroups(
	slot schedule.TimeSlot,
	groupIDs []string,
	occupied map[schedule.TimeSlot]map[string]bool,
) bool {
	for _, gid := range groupIDs {
		if !isSlotFree(slot, gid, occupied) {
			return false
		}
	}
	return true
}

func isTeacherAvailable(slot schedule.TimeSlot, teacher schedule.Teacher) bool {
	for _, unavailable := range teacher.UnavailableSlots {
		if unavailable.Day == slot.Day && unavailable.PairNum == slot.PairNum {
			return false
		}
	}
	return true
}

func isRoomSuitable(room schedule.Room, requiredType string) bool {
	return room.Type == requiredType
}

// func withinWeeklyLoad(teacherID string, currentLoad map[string]int, teacher schedule.Teacher) bool {
// 	return currentLoad[teacherID]+2 <= teacher.MaxWeeklyHours
// }

func withinSubjectLimit(
	subjectID string,
	classType schedule.ClassType,
	currentCount map[string]map[schedule.ClassType]int,
	plan schedule.SubjectPlan,
) bool {
	current := 0
	if currentCount[subjectID] != nil {
		current = currentCount[subjectID][classType]
	}

	var maxHours int
	switch classType {
	case schedule.Lecture:
		maxHours = plan.LectureHours
	case schedule.Practice:
		maxHours = plan.PracticeHours
	case schedule.Lab:
		maxHours = plan.LabHours
	}

	return (current + 2) <= maxHours
}

func isBuildingAllowedForGroup(buildingID string, group schedule.Group) bool {
	for _, bid := range group.BuildingIDs {
		if bid == buildingID {
			return true
		}
	}
	return false
}

func isBuildingAllowedForTeacher(buildingID string, teacher schedule.Teacher) bool {
	if len(teacher.PreferredBuildings) == 0 {
		return true
	}
	for _, bid := range teacher.PreferredBuildings {
		if bid == buildingID {
			return true
		}
	}
	return false
}

// isBuildingAllowedForAllGroups — корпус должен подходить ВСЕМ группам потока
func isBuildingAllowedForAllGroups(
	buildingID string,
	groupIDs []string,
	groupMap map[string]schedule.Group,
) bool {
	for _, gid := range groupIDs {
		if g, ok := groupMap[gid]; ok {
			if !isBuildingAllowedForGroup(buildingID, g) {
				return false
			}
		}
	}
	return true
}

// Проверка вместимости аудитории
// Для потоковых лекций (>2 групп) разрешено превышение до 50%
// Возвращает: подходит ли, и коэффициент переполнения (0.0 - 0.5)
func isRoomBigEnoughWithOverflow(
	room schedule.Room,
	groupIDs []string,
	groupMap map[string]schedule.Group,
) (bool, float64) {
	total := 0
	for _, gid := range groupIDs {
		if g, ok := groupMap[gid]; ok {
			total += g.StudentCount
		}
	}

	// Для большого потока — детальный лог
	if len(groupIDs) >= 8 {
		fmt.Printf("ROOM %s: cap=%d type=%s need=%d (1.5x=%d)\n",
			room.ID, room.Capacity, room.Type, total, int(float64(room.Capacity)*1.5))
	}

	if total <= room.Capacity {
		return true, 0.0
	}

	if len(groupIDs) >= 3 {
		maxAllowed := int(float64(room.Capacity) * 1.5)
		if total <= maxAllowed {
			overflow := float64(total-room.Capacity) / float64(room.Capacity)
			return true, overflow
		}
	}

	return false, 0.0
}
