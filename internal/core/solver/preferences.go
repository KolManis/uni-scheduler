package solver

import (
	"fmt"
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

// preferencePenalties — штрафы необязательных правил, суммы по правилам.
func preferencePenalties(assignments []domain.Assignment, input domain.InputData) prefPenalties {
	var p prefPenalties
	for _, v := range preferenceViolations(assignments, input) {
		switch v.Category {
		case "SubjectSpread":
			p.SubjectSpread += v.Penalty
		case "PracticeBeforeLecture":
			p.PracticeBeforeLecture += v.Penalty
		case "LecturePracticeApart":
			p.LecturePracticeApart += v.Penalty
		}
	}
	return p
}

// preferenceViolations — нарушения необязательных правил (ADR-0006), по одному на пару и
// группу. Правила не делятся на недели: считаются по всем парам сразу, один раз.
func preferenceViolations(assignments []domain.Assignment, input domain.InputData) []domain.Violation {
	prefs := input.Preferences
	planName := make(map[string]string, len(input.SubjectPlans))
	for _, sp := range input.SubjectPlans {
		planName[sp.ID] = sp.Name
	}

	var out []domain.Violation
	if prefs.SameSubjectSameDay {
		out = append(out, subjectSpreadViolations(assignments, planName)...)
	}
	if prefs.LectureBeforePractice || prefs.LecturePracticeSameDay {
		out = append(out, lectureOrderViolations(assignments, input, planName)...)
	}
	return out
}

// subjectSpreadViolations — практики и лабораторные одного плана у группы стоят в разные
// дни: subjectSpreadPenalty за каждый день сверх первого.
func subjectSpreadViolations(assignments []domain.Assignment, planName map[string]string) []domain.Violation {
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
	var out []domain.Violation
	for k, d := range days {
		if len(d) > 1 {
			out = append(out, domain.Violation{Category: "SubjectSpread", Rule: "Предмет в разные дни",
				GroupIDs: []string{k.group}, Detail: fmt.Sprintf("«%s» — в %d разных днях", planName[k.plan], len(d)),
				Penalty: (len(d) - 1) * subjectSpreadPenalty})
		}
	}
	return out
}

// lectureOrderViolations — практики и лабораторные относительно лекций по тому же предмету
// у той же группы: раньше лекции в неделе и/или не в день лекции после неё.
func lectureOrderViolations(assignments []domain.Assignment, input domain.InputData, planName map[string]string) []domain.Violation {
	prefs := input.Preferences
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

	var out []domain.Violation
	for _, a := range assignments {
		if a.Type == domain.Lecture {
			continue
		}
		for _, gid := range a.GroupIDs {
			lecs, ok := lectures[subjGroup{keyOf[a.SubjectID], gid}]
			if !ok {
				continue
			}
			pair := fmt.Sprintf("«%s», %d-я пара", planName[a.SubjectID], a.TimeSlot.PairNum())
			if prefs.LectureBeforePractice && weekPosition(a.TimeSlot) < earliest(lecs) {
				out = append(out, domain.Violation{Category: "PracticeBeforeLecture", Rule: "Практика раньше лекции",
					GroupIDs: []string{gid}, Day: a.TimeSlot.Day(), Detail: pair, Penalty: practiceBeforeLecturePenalty})
			}
			if prefs.LecturePracticeSameDay && !followsLectureSameDay(a.TimeSlot, lecs) {
				out = append(out, domain.Violation{Category: "LecturePracticeApart", Rule: "Практика не в день лекции",
					GroupIDs: []string{gid}, Day: a.TimeSlot.Day(), Detail: pair, Penalty: lecturePracticeApartPenalty})
			}
		}
	}
	return out
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
