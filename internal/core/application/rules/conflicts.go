// Package rules — правила расписания, общие для нескольких сценариев: жёсткие ограничения
// для ручного переноса и подсказок (conflicts.go), расчёт нагрузки (load.go).
package rules

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
	ConflictRoomUnsuitable      = "room_unsuitable"       // HC4–HC6: аудитория не подходит паре по типу, вместимости или корпусу
)

// ConflictError — пару нельзя поставить в это время: что именно мешает.
// ConflictWith — номер мешающей пары в расписании; −1, если мешает не пара
// (недоступность или пара на другом факультете).
type ConflictError struct {
	Type         string        `json:"type"`
	ResourceID   string        `json:"resource_id"`
	ConflictWith int           `json:"conflict_with"`
	Detail       string        `json:"detail,omitempty"` // пометка пары на другом факультете или чем не подходит аудитория
	Parity       domain.Parity `json:"parity,omitempty"` // неделя пары на другом факультете
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict: %s resource=%s with_idx=%d", e.Type, e.ResourceID, e.ConflictWith)
}

// Check — что помешает поставить пару m на место пары idx. nil — ничего.
//
// Порядок проверок: недоступность преподавателя, его пары на других факультетах, затем
// преподаватель и группы по всем парам расписания, аудитория — последней. Занятая
// аудитория поправима (можно взять другую) и не должна заслонять неустранимый конфликт.
// ignoreRoom — не проверять аудиторию вовсе (её подберут отдельно).
func Check(assignments []domain.Assignment, idx int, m domain.Assignment, teachers []domain.Teacher, ignoreRoom bool) *ConflictError {
	if IsTeacherUnavailable(teachers, m.TeacherID, m.TimeSlot) {
		return &ConflictError{Type: ConflictTeacherUnavailable, ResourceID: m.TeacherID, ConflictWith: -1}
	}
	if ep := ExternalPairAt(teachers, m.TeacherID, m.TimeSlot, m.Parity); ep != nil {
		return &ConflictError{Type: ConflictTeacherExternalPair, ResourceID: m.TeacherID,
			ConflictWith: -1, Detail: ep.Note, Parity: ep.Parity}
	}
	for j, a := range assignments {
		if j == idx || !SameWeekSlot(m, a) {
			continue
		}
		if a.TeacherID == m.TeacherID {
			return &ConflictError{Type: ConflictTeacherBusy, ResourceID: m.TeacherID, ConflictWith: j}
		}
		if g := SharedGroup(m.GroupIDs, a.GroupIDs); g != "" {
			return &ConflictError{Type: ConflictGroupBusy, ResourceID: g, ConflictWith: j}
		}
	}
	if ignoreRoom {
		return nil
	}
	return RoomBusy(assignments, idx, m)
}

// RoomBusy — аудитория пары m занята другой парой в тот же слот и неделю.
func RoomBusy(assignments []domain.Assignment, idx int, m domain.Assignment) *ConflictError {
	for j, a := range assignments {
		if j != idx && a.RoomID == m.RoomID && SameWeekSlot(m, a) {
			return &ConflictError{Type: ConflictRoomBusy, ResourceID: m.RoomID, ConflictWith: j}
		}
	}
	return nil
}

// SameWeekSlot — пары стоят в одном слоте и идут хотя бы в одну общую неделю.
func SameWeekSlot(a, b domain.Assignment) bool {
	return a.TimeSlot == b.TimeSlot && WeeksOverlap(a.Parity, b.Parity)
}

// WeeksOverlap — пары с чётностями a и b идут хотя бы в одну общую неделю.
// Пустая чётность — «каждую неделю».
func WeeksOverlap(a, b domain.Parity) bool {
	return a == "" || b == "" || a == domain.Always || b == domain.Always || a == b
}

// SharedGroup — первая группа, которая есть в обоих списках; "" — общих нет.
func SharedGroup(a, b []string) string {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return x
			}
		}
	}
	return ""
}

// IsTeacherUnavailable — слот входит в недоступные слоты преподавателя.
func IsTeacherUnavailable(teachers []domain.Teacher, teacherID string, slot domain.TimeSlot) bool {
	t := FindTeacher(teachers, teacherID)
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

// ExternalPairAt — пара преподавателя на другом факультете в этом слоте, идущая хотя бы
// в одну неделю с парой чётности parity; nil — такой нет.
func ExternalPairAt(teachers []domain.Teacher, teacherID string, slot domain.TimeSlot, parity domain.Parity) *domain.ExternalPair {
	t := FindTeacher(teachers, teacherID)
	if t == nil {
		return nil
	}
	for k, ep := range t.ExternalPairs {
		if ep.TimeSlot == slot && WeeksOverlap(ep.Parity, parity) {
			return &t.ExternalPairs[k]
		}
	}
	return nil
}

// FindTeacher — преподаватель по id; nil — нет в справочнике.
func FindTeacher(teachers []domain.Teacher, id string) *domain.Teacher {
	for k := range teachers {
		if teachers[k].ID == id {
			return &teachers[k]
		}
	}
	return nil
}

// FreeRoom — аудитория для пары moved на месте пары idx: своя, если свободна, иначе первая
// свободная из rooms (подходящих по типу, вместимости и корпусам). false — свободной нет.
func FreeRoom(assignments []domain.Assignment, idx int, moved domain.Assignment, rooms []domain.Room) (domain.Room, bool) {
	if RoomBusy(assignments, idx, moved) == nil {
		return domain.Room{ID: moved.RoomID, BuildingID: moved.BuildingID}, true
	}
	for _, r := range rooms {
		moved.RoomID = r.ID
		if RoomBusy(assignments, idx, moved) == nil {
			return r, true
		}
	}
	return domain.Room{}, false
}
