package solver

import (
	"regexp"
	"strings"
	"sync"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Веса необязательных правил. Меньше штрафа за день с одной парой (4000): правило
// не должно ради себя создавать форточки и окна.
const (
	practiceBeforeLecturePenalty = 2000
	subjectSpreadPenalty         = 3000
)

// Пометка вида занятия в конце названия плана: «Высшая математика (лекция)».
var classTypeSuffix = regexp.MustCompile(`(?i)\s*\((лекция|лекции|практика|практики|лаб[^)]*|семинар[^)]*)\)\s*$`)

// subjectKeyCache — результат subjectKey по названию. Оценка вызывается десятки тысяч раз
// за прогон, регулярное выражение на каждый вызов было бы слишком дорогим.
var subjectKeyCache sync.Map

// subjectKey — название предмета без пометки вида занятия. Лекция и практика по одному
// предмету в данных — разные учебные планы, и связать их можно только по названию.
func subjectKey(planName string) string {
	if k, ok := subjectKeyCache.Load(planName); ok {
		return k.(string)
	}
	k := strings.ToLower(strings.TrimSpace(classTypeSuffix.ReplaceAllString(planName, "")))
	subjectKeyCache.Store(planName, k)
	return k
}

// weekPosition — место слота в неделе: сравнимо между днями (пн-1 < пн-2 < вт-1).
func weekPosition(slot domain.TimeSlot) int {
	for i, d := range domain.AllDays {
		if d == slot.Day() {
			return i*10 + slot.PairNum()
		}
	}
	return 0
}

// preferencePenalties считает штрафы необязательных правил для готового расписания.
func preferencePenalties(assignments []domain.Assignment, input domain.InputData) (beforeLecture, spread int) {
	prefs := input.Preferences
	if !prefs.LectureBeforePractice && !prefs.SameSubjectSameDay {
		return 0, 0
	}

	if prefs.SameSubjectSameDay {
		type planGroup struct{ plan, group string }
		days := make(map[planGroup]map[domain.Day]bool)
		for _, a := range assignments {
			if a.Type == domain.Lecture {
				continue
			}
			for _, gid := range a.GroupIDs {
				k := planGroup{a.SubjectID, gid}
				if days[k] == nil {
					days[k] = make(map[domain.Day]bool)
				}
				days[k][a.TimeSlot.Day()] = true
			}
		}
		for _, d := range days {
			spread += (len(d) - 1) * subjectSpreadPenalty
		}
	}

	if prefs.LectureBeforePractice {
		keyOf := make(map[string]string, len(input.SubjectPlans))
		for _, sp := range input.SubjectPlans {
			keyOf[sp.ID] = subjectKey(sp.Name)
		}
		type subjGroup struct{ subject, group string }
		firstLecture := make(map[subjGroup]int)
		for _, a := range assignments {
			if a.Type != domain.Lecture {
				continue
			}
			pos := weekPosition(a.TimeSlot)
			for _, gid := range a.GroupIDs {
				k := subjGroup{keyOf[a.SubjectID], gid}
				if cur, ok := firstLecture[k]; !ok || pos < cur {
					firstLecture[k] = pos
				}
			}
		}
		for _, a := range assignments {
			if a.Type == domain.Lecture {
				continue
			}
			pos := weekPosition(a.TimeSlot)
			for _, gid := range a.GroupIDs {
				if lec, ok := firstLecture[subjGroup{keyOf[a.SubjectID], gid}]; ok && pos < lec {
					beforeLecture += practiceBeforeLecturePenalty
				}
			}
		}
	}
	return beforeLecture, spread
}

// preferenceSlotPenalty — те же правила при построении: насколько слот нежелателен
// для очередной пары с учётом уже поставленных.
func preferenceSlotPenalty(state *teacherState, subject domain.SubjectPlan, classType domain.ClassType,
	slot domain.TimeSlot, groupIDs []string) int {

	prefs := state.input.Preferences
	penalty := 0

	if prefs.SameSubjectSameDay && classType != domain.Lecture {
		sameDay, otherDay := false, false
		for _, a := range state.assignments {
			if a.SubjectID != subject.ID || a.Type != classType {
				continue
			}
			if a.TimeSlot.Day() == slot.Day() {
				sameDay = true
			} else {
				otherDay = true
			}
		}
		if otherDay && !sameDay {
			penalty += subjectSpreadPenalty
		}
	}

	if prefs.LectureBeforePractice {
		key := state.planKeys[subject.ID]
		pos := weekPosition(slot)
		for _, a := range state.assignments {
			if state.planKeys[a.SubjectID] != key || !sharesGroup(a.GroupIDs, groupIDs) {
				continue
			}
			other := weekPosition(a.TimeSlot)
			isLecture := classType == domain.Lecture
			otherIsLecture := a.Type == domain.Lecture
			if isLecture && !otherIsLecture && pos > other {
				penalty += practiceBeforeLecturePenalty
			}
			if !isLecture && otherIsLecture && pos < other {
				penalty += practiceBeforeLecturePenalty
			}
		}
	}
	return penalty
}

func sharesGroup(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
