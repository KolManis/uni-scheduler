package app

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Виды конфликтов — какое жёсткое ограничение нарушит пара.
const (
	ConflictTeacherBusy         = "teacher_busy"          // HC1: преподаватель занят другой парой
	ConflictGroupBusy           = "group_busy"            // HC2: группа занята другой парой
	ConflictRoomBusy            = "room_busy"             // HC3: аудитория занята другой парой
	ConflictTeacherUnavailable  = "teacher_unavailable"   // HC7: слот отмечен преподавателем как недоступный
	ConflictTeacherExternalPair = "teacher_external_pair" // HC7: у преподавателя пара на другом факультете
)

// ConflictError — пару нельзя поставить в это время: что именно мешает.
// ConflictWith — номер мешающей пары в расписании; −1, если мешает не пара
// (недоступность или пара на другом факультете).
type ConflictError struct {
	Type         string        `json:"type"`
	ResourceID   string        `json:"resource_id"`
	ConflictWith int           `json:"conflict_with"`
	Detail       string        `json:"detail,omitempty"` // пометка пары на другом факультете
	Parity       domain.Parity `json:"parity,omitempty"` // неделя пары на другом факультете
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict: %s resource=%s with_idx=%d", e.Type, e.ResourceID, e.ConflictWith)
}

// moveConflict — что помешает поставить пару m на место пары idx. nil — ничего.
//
// Порядок проверок: недоступность преподавателя, его пары на других факультетах, затем
// преподаватель и группы по всем парам расписания, аудитория — последней. Занятая
// аудитория поправима (можно взять другую) и не должна заслонять неустранимый конфликт.
// ignoreRoom — не проверять аудиторию вовсе (её подберут отдельно).
func moveConflict(assignments []domain.Assignment, idx int, m domain.Assignment, teachers []domain.Teacher, ignoreRoom bool) *ConflictError {
	if isTeacherUnavailable(teachers, m.TeacherID, m.TimeSlot) {
		return &ConflictError{Type: ConflictTeacherUnavailable, ResourceID: m.TeacherID, ConflictWith: -1}
	}
	if ep := externalPairAt(teachers, m.TeacherID, m.TimeSlot, m.Parity); ep != nil {
		return &ConflictError{Type: ConflictTeacherExternalPair, ResourceID: m.TeacherID,
			ConflictWith: -1, Detail: ep.Note, Parity: ep.Parity}
	}
	for j, a := range assignments {
		if j == idx || !sameWeekSlot(m, a) {
			continue
		}
		if a.TeacherID == m.TeacherID {
			return &ConflictError{Type: ConflictTeacherBusy, ResourceID: m.TeacherID, ConflictWith: j}
		}
		if g := sharedGroup(m.GroupIDs, a.GroupIDs); g != "" {
			return &ConflictError{Type: ConflictGroupBusy, ResourceID: g, ConflictWith: j}
		}
	}
	if ignoreRoom {
		return nil
	}
	return roomConflict(assignments, idx, m)
}

// roomConflict — аудитория пары m занята другой парой в тот же слот и неделю.
func roomConflict(assignments []domain.Assignment, idx int, m domain.Assignment) *ConflictError {
	for j, a := range assignments {
		if j != idx && a.RoomID == m.RoomID && sameWeekSlot(m, a) {
			return &ConflictError{Type: ConflictRoomBusy, ResourceID: m.RoomID, ConflictWith: j}
		}
	}
	return nil
}

// sameWeekSlot — пары стоят в одном слоте и идут хотя бы в одну общую неделю.
func sameWeekSlot(a, b domain.Assignment) bool {
	return a.TimeSlot == b.TimeSlot && weeksOverlap(a.Parity, b.Parity)
}

// weeksOverlap — пары с чётностями a и b идут хотя бы в одну общую неделю.
// Пустая чётность — «каждую неделю».
func weeksOverlap(a, b domain.Parity) bool {
	return a == "" || b == "" || a == domain.Always || b == domain.Always || a == b
}

// sharedGroup — первая группа, которая есть в обоих списках; "" — общих нет.
func sharedGroup(a, b []string) string {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return x
			}
		}
	}
	return ""
}

// isTeacherUnavailable — слот входит в недоступные слоты преподавателя.
func isTeacherUnavailable(teachers []domain.Teacher, teacherID string, slot domain.TimeSlot) bool {
	t := findTeacher(teachers, teacherID)
	if t == nil {
		return false
	}
	for _, s := range t.UnavailableSlots {
		if s == slot {
			return true
		}
	}
	return false
}

// externalPairAt — пара преподавателя на другом факультете в этом слоте, идущая хотя бы
// в одну неделю с парой чётности parity; nil — такой нет.
func externalPairAt(teachers []domain.Teacher, teacherID string, slot domain.TimeSlot, parity domain.Parity) *domain.ExternalPair {
	t := findTeacher(teachers, teacherID)
	if t == nil {
		return nil
	}
	for k, ep := range t.ExternalPairs {
		if ep.TimeSlot == slot && weeksOverlap(ep.Parity, parity) {
			return &t.ExternalPairs[k]
		}
	}
	return nil
}

func findTeacher(teachers []domain.Teacher, id string) *domain.Teacher {
	for k := range teachers {
		if teachers[k].ID == id {
			return &teachers[k]
		}
	}
	return nil
}
