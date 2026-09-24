package rules

import (
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// FindPlan — учебный план по id.
func FindPlan(plans []domain.SubjectPlan, id string) (domain.SubjectPlan, bool) {
	for _, sp := range plans {
		if sp.ID == id {
			return sp, true
		}
	}
	return domain.SubjectPlan{}, false
}

// MissingGroup — первая группа плана, которой нет в справочнике; "" — все есть.
func MissingGroup(ids []string, groups []domain.Group) string {
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

// StudentCount — сколько студентов в группах ids.
func StudentCount(ids []string, groups []domain.Group) int {
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

// PlanParity — чётность плана; пустая — «каждую неделю».
func PlanParity(sp domain.SubjectPlan) domain.Parity {
	if sp.Parity == "" {
		return domain.Always
	}
	return sp.Parity
}

// TeacherFreeSlots — сколько пар в неделю преподаватель может вести у кафедры: 36 минус
// недоступные и пары на других факультетах «каждую неделю».
func TeacherFreeSlots(t domain.Teacher) int {
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

// TeacherNeededPairs — сколько слотов в неделю нужно преподавателю по всем его планам.
// Пары «через неделю» тоже занимают слот: в свою неделю он должен быть свободен.
func TeacherNeededPairs(teacherID string, plans []domain.SubjectPlan) int {
	need := 0
	for _, sp := range plans {
		if sp.TeacherID == teacherID {
			need += PairsOf(sp.LectureHours) + PairsOf(sp.PracticeHours) + PairsOf(sp.LabHours)
		}
	}
	return need
}

// PairsOf — сколько пар в неделю на hours часов (пара — 2 часа, нечётные округляются вверх).
func PairsOf(hours int) int {
	return (hours + 1) / 2
}
