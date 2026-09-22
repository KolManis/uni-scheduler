package solver

import (
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func isSlotFree(
	slot domain.TimeSlot,
	resourceID string,
	parity domain.Parity,
	occupied map[domain.TimeSlot]map[string]domain.Parity,
) bool {
	if occupied[slot] == nil {
		return true
	}
	existing, exists := occupied[slot][resourceID]
	if !exists {
		return true
	}
	if existing == domain.Always || parity == domain.Always || existing == parity {
		return false
	}
	return true
}

func isSlotFreeForAllGroups(
	slot domain.TimeSlot,
	groupIDs []string,
	parity domain.Parity,
	occupied map[domain.TimeSlot]map[string]domain.Parity,
) bool {
	for _, gid := range groupIDs {
		if !isSlotFree(slot, gid, parity, occupied) {
			return false
		}
	}
	return true
}

func isTeacherAvailable(slot domain.TimeSlot, teacher domain.Teacher) bool {
	for _, unavailable := range teacher.UnavailableSlots {
		if unavailable.Day() == slot.Day() && unavailable.PairNum() == slot.PairNum() {
			return false
		}
	}
	return true
}

func isRoomSuitable(room domain.Room, requiredType string) bool {
	if requiredType == "" {
		return true
	}
	return room.Type == requiredType
}

func withinSubjectLimit(
	subjectID string,
	classType domain.ClassType,
	currentCount map[string]map[domain.ClassType]int,
	plan domain.SubjectPlan,
) bool {
	current := 0
	if currentCount[subjectID] != nil {
		current = currentCount[subjectID][classType]
	}

	var maxHours int
	switch classType {
	case domain.Lecture:
		maxHours = plan.LectureHours
	case domain.Practice:
		maxHours = plan.PracticeHours
	case domain.Lab:
		maxHours = plan.LabHours
	}

	return current < maxHours
}

func isBuildingAllowedForGroup(buildingID string, group domain.Group) bool {
	if len(group.BuildingIDs) == 0 {
		return true
	}
	for _, bid := range group.BuildingIDs {
		if bid == buildingID {
			return true
		}
	}
	return false
}

func isBuildingAllowedForTeacher(buildingID string, teacher domain.Teacher) bool {
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

func isBuildingAllowedForAllGroups(
	buildingID string,
	groupIDs []string,
	groupMap map[string]domain.Group,
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

// isRoomValidForSubject проверяет корпусные ограничения для конкретного занятия.
// Если у плана задан required_building_id — он имеет приоритет над building_ids групп:
// группы физически приходят в чужой корпус ради этого предмета.
// Если required_building_id не задан — применяется обычная проверка по группам.
func isRoomValidForSubject(
	room domain.Room,
	subject domain.SubjectPlan,
	groupIDs []string,
	groupMap map[string]domain.Group,
	teacher domain.Teacher,
) bool {
	if subject.RequiredBuildingID != "" {
		// Жёсткое требование корпуса от самого предмета — игнорируем building_ids групп
		if room.BuildingID != subject.RequiredBuildingID {
			return false
		}
	} else {
		// Стандартная проверка: корпус должен быть допустим для всех групп
		if !isBuildingAllowedForAllGroups(room.BuildingID, groupIDs, groupMap) {
			return false
		}
	}
	// Преподаватель тоже должен работать в этом корпусе
	if !isBuildingAllowedForTeacher(room.BuildingID, teacher) {
		return false
	}
	return true
}

func isRoomBigEnoughWithOverflow(
	room domain.Room,
	groupIDs []string,
	groupMap map[string]domain.Group,
) (bool, float64) {
	total := 0
	for _, gid := range groupIDs {
		if g, ok := groupMap[gid]; ok {
			total += g.StudentCount
		}
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
