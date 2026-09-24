package app

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Серьёзность проблемы в данных.
const (
	ProblemError   = "error"   // пару точно не поставить
	ProblemWarning = "warning" // поставить можно, но результат будет хуже или не таким, как ждут
)

// InputProblem — ошибка в справочниках или планах, найденная до генерации.
type InputProblem struct {
	Severity string `json:"severity"` // ProblemError | ProblemWarning
	Object   string `json:"object"`   // что не так: «План „Физика“», «Преподаватель Иванов»
	Message  string `json:"message"`
}

// maxPairsPerWeek — слотов в неделе: 6 дней по 6 пар.
var maxPairsPerWeek = len(domain.AllDays) * domain.LastPair

// CheckInput ищет в данных то, из-за чего пары не поставятся или встанут плохо, — чтобы
// узнать об этом до генерации, а не после ожидания.
func (s *Service) CheckInput(ctx context.Context) ([]InputProblem, error) {
	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}
	var problems []InputProblem
	problems = append(problems, checkPlans(*data)...)
	problems = append(problems, checkTeachers(*data)...)
	problems = append(problems, checkGroups(*data)...)
	return problems, nil
}

// checkPlans — у плана есть преподаватель, группы, часы и хотя бы одна подходящая аудитория.
func checkPlans(data domain.InputData) []InputProblem {
	var out []InputProblem
	for _, sp := range data.SubjectPlans {
		object := fmt.Sprintf("План «%s»", sp.Name)
		add := func(severity, msg string) {
			out = append(out, InputProblem{Severity: severity, Object: object, Message: msg})
		}

		if findTeacher(data.Teachers, sp.TeacherID) == nil {
			add(ProblemError, "преподаватель не найден в справочнике")
			continue
		}
		if len(sp.GroupIDs) == 0 {
			add(ProblemError, "не указаны группы")
			continue
		}
		if g := missingGroups(sp.GroupIDs, data.Groups); g != "" {
			add(ProblemError, "группа "+g+" не найдена в справочнике")
			continue
		}
		if sp.LectureHours+sp.PracticeHours+sp.LabHours == 0 {
			add(ProblemWarning, "нет часов — в расписание не попадёт")
			continue
		}
		for _, h := range []int{sp.LectureHours, sp.PracticeHours, sp.LabHours} {
			if h%2 == 1 {
				add(ProblemWarning, fmt.Sprintf("нечётное число часов (%d) — будет округлено вверх до целых пар", h))
				break
			}
		}
		pair := domain.Assignment{SubjectID: sp.ID, TeacherID: sp.TeacherID, GroupIDs: sp.GroupIDs}
		if len(solver.SuitableRooms(pair, data)) == 0 {
			add(ProblemError, fmt.Sprintf("нет аудитории типа «%s» на %d мест в допустимых корпусах",
				sp.RequiresRoomType, studentCount(sp.GroupIDs, data.Groups)))
		}
	}
	return out
}

// checkTeachers — преподавателю хватает свободного времени на все его пары.
func checkTeachers(data domain.InputData) []InputProblem {
	var out []InputProblem
	for _, t := range data.Teachers {
		need := teacherNeededPairs(t.ID, data.SubjectPlans)
		if need == 0 {
			continue
		}
		object := "Преподаватель " + t.Name
		if free := teacherFreeSlots(t); need > free {
			out = append(out, InputProblem{Severity: ProblemError, Object: object,
				Message: fmt.Sprintf("по планам нужно %d пар в неделю, а свободно только %d", need, free)})
		}
		if hours := teacherAverageHours(t.ID, data.SubjectPlans); t.MaxWeeklyHours > 0 && hours > float64(t.MaxWeeklyHours) {
			out = append(out, InputProblem{Severity: ProblemWarning, Object: object,
				Message: fmt.Sprintf("по планам в среднем %.0f ч в неделю, больше указанного максимума %d ч", hours, t.MaxWeeklyHours)})
		}
	}
	return out
}

// checkGroups — у группы в каждую неделю пар не больше, чем слотов в неделе.
func checkGroups(data domain.InputData) []InputProblem {
	var out []InputProblem
	for _, g := range data.Groups {
		even, odd := 0, 0
		for _, sp := range data.SubjectPlans {
			if !contains(sp.GroupIDs, g.ID) {
				continue
			}
			n := pairsOf(sp.LectureHours) + pairsOf(sp.PracticeHours) + pairsOf(sp.LabHours)
			switch planParity(sp) {
			case domain.Even:
				even += n
			case domain.Odd:
				odd += n
			default:
				even += n
				odd += n
			}
		}
		if busiest := max(even, odd); busiest > maxPairsPerWeek {
			out = append(out, InputProblem{Severity: ProblemError, Object: "Группа " + g.Name,
				Message: fmt.Sprintf("%d пар в неделю — больше, чем слотов в неделе (%d)", busiest, maxPairsPerWeek)})
		}
	}
	return out
}

func contains(list []string, x string) bool {
	for _, v := range list {
		if v == x {
			return true
		}
	}
	return false
}

// teacherAverageHours — часов в неделю в среднем по двум неделям: пара «через неделю»
// даёт половину своих часов.
func teacherAverageHours(teacherID string, plans []domain.SubjectPlan) float64 {
	total := 0.0
	for _, sp := range plans {
		if sp.TeacherID != teacherID {
			continue
		}
		hours := float64(sp.LectureHours + sp.PracticeHours + sp.LabHours)
		if planParity(sp) != domain.Always {
			hours /= 2
		}
		total += hours
	}
	return total
}
