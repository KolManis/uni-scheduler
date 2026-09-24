package solver

import (
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// SuitableRooms — аудитории, в которые можно поставить пару a по HC4–HC6 (тип, вместимость,
// корпуса), лучшие по вместимости первыми — те же правила, что при построении.
func SuitableRooms(a domain.Assignment, input domain.InputData) []domain.Room {
	var plan domain.SubjectPlan
	for _, sp := range input.SubjectPlans {
		if sp.ID == a.SubjectID {
			plan = sp
			break
		}
	}
	var teacher domain.Teacher
	for _, t := range input.Teachers {
		if t.ID == a.TeacherID {
			teacher = t
			break
		}
	}
	groupMap := make(map[string]domain.Group, len(input.Groups))
	for _, g := range input.Groups {
		groupMap[g.ID] = g
	}
	total := 0
	for _, gid := range a.GroupIDs {
		total += groupMap[gid].StudentCount
	}

	var out []domain.Room
	for _, r := range input.Rooms {
		if !isRoomSuitable(r, plan.RequiresRoomType) {
			continue
		}
		if ok, _ := isRoomBigEnoughWithOverflow(r, a.GroupIDs, groupMap); !ok {
			continue
		}
		if !isRoomValidForSubject(r, plan, a.GroupIDs, groupMap, teacher) {
			continue
		}
		out = append(out, r)
	}
	excess := func(r domain.Room) int {
		d := r.Capacity - total
		if d < 0 {
			return -d * 3
		}
		return d
	}
	sort.SliceStable(out, func(i, j int) bool { return excess(out[i]) < excess(out[j]) })
	return out
}
