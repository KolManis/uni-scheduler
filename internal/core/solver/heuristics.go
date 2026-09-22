package solver

import (
	"math/rand"
	"sort"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type candidate struct {
	assignment domain.Assignment
	score      int
}

func generateCandidates(
	state *solverState,
	plan domain.SubjectPlan,
	classType domain.ClassType,
) []candidate {
	var candidates []candidate

	teacher := state.teacherMap[plan.TeacherID]
	groupIDs := plan.GroupIDs

	if classType != domain.Lecture && len(groupIDs) > 1 {
		return candidates
	}

	parity := domain.Always
	if plan.Parity != "" && plan.Parity != domain.Always {
		parity = plan.Parity
	}

	for _, day := range domain.AllDays {
		for pairNum := 1; pairNum <= 6; pairNum++ {
			slot := domain.MustNewTimeSlot(day, pairNum)

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

				if !isRoomValidForSubject(room, plan, groupIDs, state.groupMap, teacher) {
					continue
				}

				score := 0
				if day == domain.Saturday {
					score += 5000
				}
				if pairNum >= 6 {
					score += 1000
				}

				sameGroupSameDay := 0
				for _, a := range state.assignments {
					if a.TimeSlot.Day() == day {
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
					assignment: domain.Assignment{
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
	if classType == domain.Lecture && len(groupIDs) > 3 {
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

func selectMostConstrained(state *solverState) (domain.SubjectPlan, domain.ClassType) {
	type item struct {
		plan      domain.SubjectPlan
		classType domain.ClassType
		remaining int
	}

	var items []item

	for _, plan := range state.input.SubjectPlans {
		for _, ct := range []domain.ClassType{domain.Lecture, domain.Practice, domain.Lab} {
			if ct != domain.Lecture && len(plan.GroupIDs) > 1 {
				continue
			}

			current := 0
			if state.subjectCount[plan.ID] != nil {
				current = state.subjectCount[plan.ID][ct]
			}

			var total int
			switch ct {
			case domain.Lecture:
				total = plan.LectureHours
			case domain.Practice:
				total = plan.PracticeHours
			case domain.Lab:
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

	return domain.SubjectPlan{}, ""
}
