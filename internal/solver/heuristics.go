package solver

import (
	"math/rand"
	"sort"
	"time"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type candidate struct {
	assignment schedule.Assignment
	score      int
}

func generateCandidates(
	state *solverState,
	plan schedule.SubjectPlan,
	classType schedule.ClassType,
) []candidate {
	var candidates []candidate

	teacher := state.teacherMap[plan.TeacherID]
	groupIDs := plan.GroupIDs

	if classType != schedule.Lecture && len(groupIDs) > 1 {
		return candidates
	}

	parity := schedule.Always
	if plan.Parity != "" && plan.Parity != schedule.Always {
		parity = plan.Parity
	}

	if plan.ID == "MATH-L2" && state.iterations <= 1 {
		totalStudents := 0
		for _, gid := range groupIDs {
			if g, ok := state.groupMap[gid]; ok {
				totalStudents += g.StudentCount
			}
		}
		state.logger.Info("DEBUG MATH-L2",
			"groups_count", len(groupIDs),
			"total_students", totalStudents,
			"teacher_load", state.teacherLoad[plan.TeacherID],
			"teacher_max", teacher.MaxWeeklyHours,
			"subject_count", func() int {
				if state.subjectCount[plan.ID] != nil {
					return state.subjectCount[plan.ID][classType]
				}
				return 0
			}(),
		)

		suitableRooms := 0
		for _, room := range state.input.Rooms {
			if room.Type == plan.RequiresRoomType {
				ok, _ := isRoomBigEnoughWithOverflow(room, groupIDs, state.groupMap)
				if ok {
					suitableRooms++
				}
			}
		}
		state.logger.Info("DEBUG MATH-L2 rooms",
			"total_rooms", len(state.input.Rooms),
			"suitable_rooms", suitableRooms,
			"required_type", plan.RequiresRoomType,
		)
	}

	for _, day := range schedule.AllDays {
		for pairNum := 1; pairNum <= 6; pairNum++ {
			slot := schedule.TimeSlot{Day: day, PairNum: pairNum}

			if !isSlotFreeForAllGroups(slot, groupIDs, parity, state.occupiedGroups) {
				continue
			}
			if !isTeacherAvailable(slot, teacher) {
				continue
			}
			if !isSlotFree(slot, plan.TeacherID, parity, state.occupiedTeachers) {
				continue
			}
			if !withinSubjectLimit(plan.ID, classType, state.subjectCount, plan) {
				continue
			}

			for _, room := range state.input.Rooms {
				if !isRoomSuitable(room, plan.RequiresRoomType) {
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

				score := 0
				if day == schedule.Saturday {
					score += 5000
				}
				if pairNum >= 6 {
					score += 1000
				}

				sameGroupSameDay := 0
				for _, a := range state.assignments {
					if a.TimeSlot.Day == day {
						for _, gid := range groupIDs {
							for _, agid := range a.GroupIDs {
								if gid == agid {
									sameGroupSameDay++
								}
							}
						}
					}
				}
				if sameGroupSameDay > 0 {
					score -= 30 * sameGroupSameDay
				}

				if state.dayLoad[day] > 0 {
					score -= 20
				}
				if len(groupIDs) > 1 {
					score -= 50
				}
				if overflow > 0 {
					score += int(overflow * 200)
				}

				candidates = append(candidates, candidate{
					assignment: schedule.Assignment{
						GroupIDs:   groupIDs,
						TeacherID:  plan.TeacherID,
						RoomID:     room.ID,
						SubjectID:  plan.ID,
						Type:       classType,
						TimeSlot:   slot,
						Parity:     parity,
						BuildingID: room.BuildingID,
					},
					score: score,
				})
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score < candidates[j].score
	})

	maxCandidates := 30
	if classType == schedule.Lecture && len(groupIDs) > 3 {
		maxCandidates = 100
	}
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}

	state.rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	return candidates
}

func selectMostConstrained(state *solverState) (schedule.SubjectPlan, schedule.ClassType) {
	type item struct {
		plan      schedule.SubjectPlan
		classType schedule.ClassType
		remaining int
	}

	var items []item

	for _, plan := range state.input.SubjectPlans {
		for _, ct := range []schedule.ClassType{schedule.Lecture, schedule.Practice, schedule.Lab} {
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

			remaining := total - current
			if remaining > 0 {
				items = append(items, item{plan: plan, classType: ct, remaining: remaining})
			}
		}
	}

	state.rng.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})

	sort.Slice(items, func(i, j int) bool {
		if len(items[i].plan.GroupIDs) != len(items[j].plan.GroupIDs) {
			return len(items[i].plan.GroupIDs) > len(items[j].plan.GroupIDs)
		}
		return items[i].remaining < items[j].remaining
	})

	if len(items) > 0 {
		return items[0].plan, items[0].classType
	}

	return schedule.SubjectPlan{}, ""
}
