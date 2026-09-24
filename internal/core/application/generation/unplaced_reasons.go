package generation

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/application/rules"
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
	plan, ok := rules.FindPlan(data.SubjectPlans, item.SubjectID)
	if !ok {
		return "учебный план не найден"
	}
	teacher := rules.FindTeacher(data.Teachers, plan.TeacherID)
	if teacher == nil {
		return "преподаватель плана не найден в справочнике"
	}
	if missing := rules.MissingGroup(plan.GroupIDs, data.Groups); missing != "" {
		return "группа " + missing + " не найдена в справочнике"
	}

	pair := domain.Assignment{SubjectID: plan.ID, TeacherID: plan.TeacherID, GroupIDs: plan.GroupIDs,
		Type: item.Type, Parity: rules.PlanParity(plan)}
	rooms := solver.SuitableRooms(pair, data)
	if len(rooms) == 0 {
		return fmt.Sprintf("нет аудитории типа «%s» на %d мест в допустимых корпусах",
			plan.RequiresRoomType, rules.StudentCount(plan.GroupIDs, data.Groups))
	}

	if free, need := rules.TeacherFreeSlots(*teacher), rules.TeacherNeededPairs(teacher.ID, data.SubjectPlans); free < need {
		return fmt.Sprintf("у преподавателя свободно %d пар в неделю, а по его планам нужно %d", free, need)
	}

	for _, day := range domain.AllDays {
		for num := domain.FirstPair; num <= domain.LastPair; num++ {
			pair.TimeSlot = domain.MustNewTimeSlot(day, num)
			if rules.Check(assignments, -1, pair, data.Teachers, true) != nil {
				continue
			}
			if _, ok := rules.FreeRoom(assignments, -1, pair, rooms); ok {
				return "свободное время есть, но занять его не удалось без переноса других пар — попробуйте перегенерировать или поставить вручную"
			}
		}
	}
	return "нет времени, когда свободны преподаватель, все группы плана и подходящая аудитория"
}
