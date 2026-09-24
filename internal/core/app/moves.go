package app

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// pinnedOf — закреплённые пары расписания id (для перегенерации вокруг них). id == 0 — нет.
func (s *Service) pinnedOf(ctx context.Context, id int64) ([]domain.Assignment, error) {
	if id <= 0 {
		return nil, nil
	}
	base, err := s.outputRepo.GetSchedule(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("base schedule %d: %w", id, err)
	}
	var fixed []domain.Assignment
	for _, a := range base.Assignments {
		if a.Pinned {
			fixed = append(fixed, a)
		}
	}
	return fixed, nil
}

// SetPinned закрепляет пару (pinned = true) или снимает закрепление.
func (s *Service) SetPinned(ctx context.Context, schedID int64, idx int, pinned bool) (*domain.Schedule, error) {
	sched, err := s.outputRepo.GetSchedule(ctx, schedID)
	if err != nil {
		return nil, err
	}
	if idx < 0 || idx >= len(sched.Assignments) {
		return nil, fmt.Errorf("%w: assignment index %d out of range", ErrInvalidInput, idx)
	}
	sched.Assignments[idx].Pinned = pinned
	if err := s.outputRepo.UpdateSchedule(ctx, sched); err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	return sched, nil
}

// MoveOption — куда можно перенести пару: слот, аудитория и как изменится расписание.
type MoveOption struct {
	Slot     domain.TimeSlot `json:"time_slot"`
	Current  bool            `json:"current,omitempty"` // пара стоит здесь сейчас
	RoomID   string          `json:"room_id,omitempty"` // аудитория для переноса: своя, если свободна, иначе другая подходящая
	Conflict *ConflictError  `json:"conflict,omitempty"`
	// Изменения при переносе, сумма по чётной и нечётной неделе; отрицательное — лучше.
	ScoreDelta      int `json:"score_delta"`
	GapsDelta       int `json:"gaps_delta"`
	LongGapsDelta   int `json:"long_gaps_delta"`
	SingleDaysDelta int `json:"single_days_delta"`
	SaturdayDelta   int `json:"saturday_delta"`
}

// MoveOptions — для каждого слота сетки: можно ли перенести туда пару idx (с той же
// чётностью) и как это изменит score и показатели качества.
func (s *Service) MoveOptions(ctx context.Context, schedID int64, idx int) ([]MoveOption, error) {
	sched, err := s.outputRepo.GetSchedule(ctx, schedID)
	if err != nil {
		return nil, err
	}
	if idx < 0 || idx >= len(sched.Assignments) {
		return nil, fmt.Errorf("%w: assignment index %d out of range", ErrInvalidInput, idx)
	}
	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}
	data.Preferences = sched.Options

	orig := sched.Assignments[idx]
	baseScore := solver.CalculateFitness(sched.Assignments, *data)
	baseQ := solver.CalculateQuality(sched.Assignments)
	rooms := solver.SuitableRooms(orig, *data)

	var out []MoveOption
	for _, day := range domain.AllDays {
		for pair := domain.FirstPair; pair <= domain.LastPair; pair++ {
			slot := domain.MustNewTimeSlot(day, pair)
			opt := MoveOption{Slot: slot, RoomID: orig.RoomID}
			if slot == orig.TimeSlot {
				opt.Current = true
				out = append(out, opt)
				continue
			}
			m := orig
			m.TimeSlot = slot
			opt.Conflict = moveConflict(sched.Assignments, idx, m, data.Teachers, true)
			if opt.Conflict == nil && roomConflict(sched.Assignments, idx, m) != nil {
				opt.Conflict = &ConflictError{Type: "room_busy", ResourceID: m.RoomID, ConflictWith: -1}
				for _, r := range rooms {
					m.RoomID, m.BuildingID = r.ID, r.BuildingID
					if roomConflict(sched.Assignments, idx, m) == nil {
						opt.Conflict, opt.RoomID = nil, r.ID
						break
					}
				}
			}
			if opt.Conflict == nil {
				moved := append([]domain.Assignment(nil), sched.Assignments...)
				moved[idx] = m
				q := solver.CalculateQuality(moved)
				opt.ScoreDelta = solver.CalculateFitness(moved, *data) - baseScore
				opt.GapsDelta = q.Even.GroupGaps + q.Odd.GroupGaps - baseQ.Even.GroupGaps - baseQ.Odd.GroupGaps
				opt.LongGapsDelta = q.Even.GroupLongGaps + q.Odd.GroupLongGaps - baseQ.Even.GroupLongGaps - baseQ.Odd.GroupLongGaps
				opt.SingleDaysDelta = q.Even.SingleClassDays + q.Odd.SingleClassDays - baseQ.Even.SingleClassDays - baseQ.Odd.SingleClassDays
				opt.SaturdayDelta = q.Even.SaturdayPairs + q.Odd.SaturdayPairs - baseQ.Even.SaturdayPairs - baseQ.Odd.SaturdayPairs
			}
			out = append(out, opt)
		}
	}
	return out, nil
}

// moveConflict — нарушит ли пара m (новое положение пары idx) жёсткие ограничения:
// недоступность преподавателя, его пары на других факультетах, занятость преподавателя
// и групп и — если ignoreRoom == false — аудитории.
func moveConflict(assignments []domain.Assignment, idx int, m domain.Assignment, teachers []domain.Teacher, ignoreRoom bool) *ConflictError {
	if isTeacherUnavailable(teachers, m.TeacherID, m.TimeSlot) {
		return &ConflictError{Type: ConflictTeacherUnavailable, ResourceID: m.TeacherID, ConflictWith: -1}
	}
	if ep := externalPairAt(teachers, m.TeacherID, m.TimeSlot, m.Parity); ep != nil {
		return &ConflictError{Type: ConflictTeacherExternalPair, ResourceID: m.TeacherID,
			ConflictWith: -1, Detail: ep.Note, Parity: ep.Parity}
	}
	// Сначала преподаватель и группы по всем парам: занятая аудитория — поправимый
	// конфликт (можно взять другую), и она не должна заслонять неустранимый.
	for j, a := range assignments {
		if j == idx {
			continue
		}
		if c := checkConflict(m, a, j); c != nil && c.Type != "room_busy" {
			return c
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
		if j != idx && a.RoomID == m.RoomID && slotsConflict(m, a) {
			return &ConflictError{Type: "room_busy", ResourceID: m.RoomID, ConflictWith: j}
		}
	}
	return nil
}
