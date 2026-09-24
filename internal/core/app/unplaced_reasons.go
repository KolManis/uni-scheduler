package app

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// explainUnplaced дописывает к каждой непоставленной паре причину. Проверки идут от
// самой грубой к самой тонкой, берётся первая сработавшая:
//  1. нет преподавателя или групп в справочниках;
//  2. нет ни одной подходящей аудитории (тип, вместимость, корпуса);
//  3. у преподавателя меньше свободных пар в неделю, чем нужно по его планам;
//  4. в готовом расписании нет слота, где свободны преподаватель, все группы и аудитория.
func explainUnplaced(items []domain.UnplacedItem, assignments []domain.Assignment, data domain.InputData) []domain.UnplacedItem {
	for k := range items {
		items[k].Reason = unplacedReason(items[k], assignments, data)
	}
	return items
}

func unplacedReason(item domain.UnplacedItem, assignments []domain.Assignment, data domain.InputData) string {
	plan, ok := findPlan(data.SubjectPlans, item.SubjectID)
	if !ok {
		return "учебный план не найден"
	}
	teacher := findTeacher(data.Teachers, plan.TeacherID)
	if teacher == nil {
		return "преподаватель плана не найден в справочнике"
	}
	if missing := missingGroups(plan.GroupIDs, data.Groups); missing != "" {
		return "группа " + missing + " не найдена в справочнике"
	}

	pair := domain.Assignment{SubjectID: plan.ID, TeacherID: plan.TeacherID, GroupIDs: plan.GroupIDs,
		Type: item.Type, Parity: planParity(plan)}
	rooms := solver.SuitableRooms(pair, data)
	if len(rooms) == 0 {
		return fmt.Sprintf("нет аудитории типа «%s» на %d мест в допустимых корпусах",
			plan.RequiresRoomType, studentCount(plan.GroupIDs, data.Groups))
	}

	if free, need := teacherFreeSlots(*teacher), teacherNeededPairs(teacher.ID, data.SubjectPlans); free < need {
		return fmt.Sprintf("у преподавателя свободно %d пар в неделю, а по его планам нужно %d", free, need)
	}

	for _, day := range domain.AllDays {
		for num := domain.FirstPair; num <= domain.LastPair; num++ {
			pair.TimeSlot = domain.MustNewTimeSlot(day, num)
			if moveConflict(assignments, -1, pair, data.Teachers, true) != nil {
				continue
			}
			if _, ok := freeRoom(assignments, -1, pair, rooms); ok {
				return "свободное время есть, но занять его не удалось без переноса других пар — попробуйте перегенерировать или поставить вручную"
			}
		}
	}
	return "нет времени, когда свободны преподаватель, все группы плана и подходящая аудитория"
}

func findPlan(plans []domain.SubjectPlan, id string) (domain.SubjectPlan, bool) {
	for _, sp := range plans {
		if sp.ID == id {
			return sp, true
		}
	}
	return domain.SubjectPlan{}, false
}

// missingGroups — первая группа плана, которой нет в справочнике; "" — все есть.
func missingGroups(ids []string, groups []domain.Group) string {
	for _, id := range ids {
		found := false
		for _, g := range groups {
			if g.ID == id {
				found = true
				break
			}
		}
		if !found {
			return id
		}
	}
	return ""
}

func studentCount(ids []string, groups []domain.Group) int {
	total := 0
	for _, g := range groups {
		for _, id := range ids {
			if g.ID == id {
				total += g.StudentCount
			}
		}
	}
	return total
}

func planParity(sp domain.SubjectPlan) domain.Parity {
	if sp.Parity == "" {
		return domain.Always
	}
	return sp.Parity
}

// teacherFreeSlots — сколько пар в неделю преподаватель может вести у кафедры: 36 минус
// недоступные и пары на других факультетах «каждую неделю».
func teacherFreeSlots(t domain.Teacher) int {
	busy := map[domain.TimeSlot]bool{}
	for _, s := range t.UnavailableSlots {
		busy[s] = true
	}
	for _, ep := range t.ExternalPairs {
		if ep.Parity == "" || ep.Parity == domain.Always {
			busy[ep.TimeSlot] = true
		}
	}
	return len(domain.AllDays)*domain.LastPair - len(busy)
}

// teacherNeededPairs — сколько слотов в неделю нужно преподавателю по всем его планам.
// Пары «через неделю» тоже занимают слот: в свою неделю он должен быть свободен.
func teacherNeededPairs(teacherID string, plans []domain.SubjectPlan) int {
	need := 0
	for _, sp := range plans {
		if sp.TeacherID == teacherID {
			need += pairsOf(sp.LectureHours) + pairsOf(sp.PracticeHours) + pairsOf(sp.LabHours)
		}
	}
	return need
}

// pairsOf — сколько пар в неделю на hours часов (пара — 2 часа, нечётные округляются вверх).
func pairsOf(hours int) int {
	return (hours + 1) / 2
}
