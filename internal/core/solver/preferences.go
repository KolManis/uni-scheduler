package solver

import (
	"regexp"
	"strings"
	"sync"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Веса необязательных правил. Меньше штрафа за день с одной парой (12000): правило
// не должно ради себя создавать форточки и окна.
const (
	practiceBeforeLecturePenalty = 2000
	lecturePracticeApartPenalty  = 2000
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

// followsLectureSameDay — в тот же день, что и слот практики, раньше него есть лекция.
func followsLectureSameDay(practice domain.TimeSlot, lectures []domain.TimeSlot) bool {
	for _, lec := range lectures {
		if lec.Day() == practice.Day() && lec.PairNum() < practice.PairNum() {
			return true
		}
	}
	return false
}

// prefPenalties — штрафы необязательных правил по отдельности, для разбивки score.
type prefPenalties struct {
	PracticeBeforeLecture int
	LecturePracticeApart  int
	SubjectSpread         int
}

// preferencePenalties считает штрафы необязательных правил для готового расписания.
func preferencePenalties(assignments []domain.Assignment, input domain.InputData) prefPenalties {
	var p prefPenalties
	prefs := input.Preferences

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
			p.SubjectSpread += (len(d) - 1) * subjectSpreadPenalty
		}
	}

	if !prefs.LectureBeforePractice && !prefs.LecturePracticeSameDay {
		return p
	}

	keyOf := make(map[string]string, len(input.SubjectPlans))
	for _, sp := range input.SubjectPlans {
		keyOf[sp.ID] = subjectKey(sp.Name)
	}
	type subjGroup struct{ subject, group string }
	lectures := make(map[subjGroup][]domain.TimeSlot)
	for _, a := range assignments {
		if a.Type != domain.Lecture {
			continue
		}
		for _, gid := range a.GroupIDs {
			k := subjGroup{keyOf[a.SubjectID], gid}
			lectures[k] = append(lectures[k], a.TimeSlot)
		}
	}

	for _, a := range assignments {
		if a.Type == domain.Lecture {
			continue
		}
		pos := weekPosition(a.TimeSlot)
		for _, gid := range a.GroupIDs {
			lecs, ok := lectures[subjGroup{keyOf[a.SubjectID], gid}]
			if !ok {
				continue
			}
			if prefs.LectureBeforePractice && pos < earliest(lecs) {
				p.PracticeBeforeLecture += practiceBeforeLecturePenalty
			}
			if prefs.LecturePracticeSameDay && !followsLectureSameDay(a.TimeSlot, lecs) {
				p.LecturePracticeApart += lecturePracticeApartPenalty
			}
		}
	}
	return p
}

func earliest(slots []domain.TimeSlot) int {
	first := weekPosition(slots[0])
	for _, s := range slots[1:] {
		if pos := weekPosition(s); pos < first {
			first = pos
		}
	}
	return first
}

// preferenceSlotPenalty — те же правила при построении: насколько слот нежелателен
// для очередной пары с учётом уже поставленных.
func preferenceSlotPenalty(draft *scheduleDraft, subject domain.SubjectPlan, classType domain.ClassType,
	slot domain.TimeSlot, groupIDs []string) int {

	prefs := draft.input.Preferences
	penalty := 0

	if prefs.SameSubjectSameDay && classType != domain.Lecture {
		sameDay, otherDay := false, false
		for _, a := range draft.assignments {
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

	if !prefs.LectureBeforePractice && !prefs.LecturePracticeSameDay {
		return penalty
	}

	// Уже поставленные занятия того же предмета у тех же групп.
	key := draft.planKeys[subject.ID]
	var lectures, practices []domain.TimeSlot
	for _, a := range draft.assignments {
		if draft.planKeys[a.SubjectID] != key || !sharesGroup(a.GroupIDs, groupIDs) {
			continue
		}
		if a.Type == domain.Lecture {
			lectures = append(lectures, a.TimeSlot)
		} else {
			practices = append(practices, a.TimeSlot)
		}
	}

	pos := weekPosition(slot)
	if classType == domain.Lecture {
		for _, pr := range practices {
			if prefs.LectureBeforePractice && weekPosition(pr) < pos {
				penalty += practiceBeforeLecturePenalty
			}
			if prefs.LecturePracticeSameDay && !followsLectureSameDay(pr, []domain.TimeSlot{slot}) {
				penalty += lecturePracticeApartPenalty
			}
		}
		return penalty
	}

	if len(lectures) > 0 {
		if prefs.LectureBeforePractice && pos < earliest(lectures) {
			penalty += practiceBeforeLecturePenalty
		}
		if prefs.LecturePracticeSameDay && !followsLectureSameDay(slot, lectures) {
			penalty += lecturePracticeApartPenalty
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
