package solver

import (
	"sort"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

// orderTeachersByDifficulty — сортировка преподавателей по сложности (используется в teacher_solver.go)
func orderTeachersByDifficulty(input schedule.InputData) []schedule.Teacher {
	teachers := make([]schedule.Teacher, len(input.Teachers))
	copy(teachers, input.Teachers)

	subjectCount := make(map[string]int)
	for _, sp := range input.SubjectPlans {
		subjectCount[sp.TeacherID]++
	}

	sort.Slice(teachers, func(i, j int) bool {
		ti, tj := teachers[i], teachers[j]
		if ti.MaxWeeklyHours != tj.MaxWeeklyHours {
			return ti.MaxWeeklyHours < tj.MaxWeeklyHours
		}
		return subjectCount[ti.ID] > subjectCount[tj.ID]
	})
	return teachers
}

// getSubjectsForTeacher — возвращает предметы преподавателя
// Эта функция дублируется в teacher_solver.go, но оставляем для обратной совместимости
func getSubjectsForTeacher(input schedule.InputData, teacherID string) []schedule.SubjectPlan {
	var plans []schedule.SubjectPlan
	for _, sp := range input.SubjectPlans {
		if sp.TeacherID == teacherID {
			plans = append(plans, sp)
		}
	}
	return plans
}

// generateCompactBlocks — генерирует компактные блоки для преподавателя
func generateCompactBlocks(state *teacherState, teacher schedule.Teacher, subjects []schedule.SubjectPlan) [][]schedule.TimeSlot {
	neededSlots := teacher.MaxWeeklyHours / 2
	existing := state.teacherSlots[teacher.ID]
	remaining := neededSlots - len(existing)
	if remaining <= 0 {
		return nil
	}

	var allSlots []schedule.TimeSlot
	for _, day := range schedule.AllDays {
		for pairNum := 1; pairNum <= 6; pairNum++ {
			slot := schedule.TimeSlot{Day: day, PairNum: pairNum}
			if !isTeacherAvailable(slot, teacher) {
				continue
			}
			// teacherState хранит Parity, проверяем просто наличие ключа (любая чётность — занят)
			if occupied, ok := state.occupiedTeachers[slot]; ok {
				if _, exists := occupied[teacher.ID]; exists {
					continue
				}
			}
			allSlots = append(allSlots, slot)
		}
	}

	sort.Slice(allSlots, func(i, j int) bool {
		if allSlots[i].Day == schedule.Saturday && allSlots[j].Day != schedule.Saturday {
			return false
		}
		if allSlots[i].Day != schedule.Saturday && allSlots[j].Day == schedule.Saturday {
			return true
		}
		return allSlots[i].PairNum < allSlots[j].PairNum
	})

	daySlots := make(map[schedule.Day][]schedule.TimeSlot)
	for _, slot := range allSlots {
		daySlots[slot.Day] = append(daySlots[slot.Day], slot)
	}

	var blocks [][]schedule.TimeSlot
	slotsLeft := remaining
	for _, day := range []schedule.Day{schedule.Monday, schedule.Tuesday, schedule.Wednesday, schedule.Thursday, schedule.Friday, schedule.Saturday} {
		if slotsLeft <= 0 {
			break
		}
		slots, ok := daySlots[day]
		if !ok || len(slots) == 0 {
			continue
		}
		if len(slots) > slotsLeft {
			slots = slots[:slotsLeft]
		}
		blocks = append(blocks, slots)
		slotsLeft -= len(slots)
	}

	return blocks
}

// findSlotInBlock — ищет слот в блоке для предмета
func findSlotInBlock(state *teacherState, teacher schedule.Teacher, plan schedule.SubjectPlan, block []schedule.TimeSlot) (*schedule.TimeSlot, *schedule.Room, float64) {
	groupIDs := plan.GroupIDs

	parity := schedule.Always
	if plan.Parity != "" && plan.Parity != schedule.Always {
		parity = plan.Parity
	}

	for _, slot := range block {
		if !isSlotFreeForAllGroups(slot, groupIDs, parity, state.occupiedGroups) {
			continue
		}
		if !isSlotFree(slot, teacher.ID, parity, state.occupiedTeachers) {
			continue
		}
		requiredType := plan.RequiresRoomType
		for _, room := range state.input.Rooms {
			if !isRoomSuitable(room, requiredType) {
				continue
			}
			if !isSlotFree(slot, room.ID, parity, state.occupiedRooms) {
				continue
			}
			ok, overflow := isRoomBigEnoughWithOverflow(room, groupIDs, state.groupMap)
			if !ok {
				continue
			}
			if !isBuildingAllowedForAllGroups(room.BuildingID, groupIDs, state.groupMap) {
				continue
			}
			if !isBuildingAllowedForTeacher(room.BuildingID, teacher) {
				continue
			}
			s := slot
			r := room
			return &s, &r, overflow
		}
	}
	return nil, nil, 0
}
