package app

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// MoveOption — можно ли перенести пару в этот слот, в какую аудиторию и что изменится.
type MoveOption struct {
	Slot     domain.TimeSlot `json:"time_slot"`
	Current  bool            `json:"current,omitempty"` // пара стоит здесь сейчас
	RoomID   string          `json:"room_id,omitempty"` // своя аудитория, если свободна, иначе другая подходящая
	Conflict *ConflictError  `json:"conflict,omitempty"`
	// Изменения при переносе, сумма по чётной и нечётной неделе; отрицательное — лучше.
	ScoreDelta      int `json:"score_delta"`
	GapsDelta       int `json:"gaps_delta"`
	LongGapsDelta   int `json:"long_gaps_delta"`
	SingleDaysDelta int `json:"single_days_delta"`
	SaturdayDelta   int `json:"saturday_delta"`
}

// MoveOptions — для каждого из 36 слотов: куда можно перенести пару index (с её
// чётностью) и как это изменит score и показатели качества.
func (s *Service) MoveOptions(ctx context.Context, scheduleID int64, index int) ([]MoveOption, error) {
	sched, err := s.loadAssignment(ctx, scheduleID, index)
	if err != nil {
		return nil, err
	}
	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}
	data.Preferences = sched.Options

	before := measure(sched.Assignments, *data)
	pair := sched.Assignments[index]
	rooms := solver.SuitableRooms(pair, *data)

	var options []MoveOption
	for _, day := range domain.AllDays {
		for num := domain.FirstPair; num <= domain.LastPair; num++ {
			slot := domain.MustNewTimeSlot(day, num)
			options = append(options, moveOption(sched.Assignments, index, slot, rooms, *data, before))
		}
	}
	return options, nil
}

// moveOption — один слот: сначала жёсткие ограничения, потом аудитория, потом изменения.
func moveOption(assignments []domain.Assignment, index int, slot domain.TimeSlot, rooms []domain.Room,
	data domain.InputData, before measurement) MoveOption {

	pair := assignments[index]
	opt := MoveOption{Slot: slot, RoomID: pair.RoomID}
	if slot == pair.TimeSlot {
		opt.Current = true
		return opt
	}

	moved := pair
	moved.TimeSlot = slot
	if c := moveConflict(assignments, index, moved, data.Teachers, true); c != nil {
		opt.Conflict = c
		return opt
	}
	room, ok := freeRoom(assignments, index, moved, rooms)
	if !ok {
		opt.Conflict = &ConflictError{Type: ConflictRoomBusy, ResourceID: pair.RoomID, ConflictWith: -1}
		return opt
	}
	moved.RoomID, moved.BuildingID = room.ID, room.BuildingID
	opt.RoomID = room.ID

	withMove := append([]domain.Assignment(nil), assignments...)
	withMove[index] = moved
	after := measure(withMove, data)
	opt.ScoreDelta = after.score - before.score
	opt.GapsDelta = after.gaps - before.gaps
	opt.LongGapsDelta = after.longGaps - before.longGaps
	opt.SingleDaysDelta = after.singleDays - before.singleDays
	opt.SaturdayDelta = after.saturday - before.saturday
	return opt
}

// freeRoom — своя аудитория пары, если свободна, иначе первая свободная из подходящих.
func freeRoom(assignments []domain.Assignment, index int, moved domain.Assignment, rooms []domain.Room) (domain.Room, bool) {
	if roomConflict(assignments, index, moved) == nil {
		return domain.Room{ID: moved.RoomID, BuildingID: moved.BuildingID}, true
	}
	for _, r := range rooms {
		moved.RoomID = r.ID
		if roomConflict(assignments, index, moved) == nil {
			return r, true
		}
	}
	return domain.Room{}, false
}

// measurement — score и показатели качества, сложенные по двум неделям.
type measurement struct {
	score, gaps, longGaps, singleDays, saturday int
}

func measure(assignments []domain.Assignment, data domain.InputData) measurement {
	q := solver.CalculateQuality(assignments)
	return measurement{
		score:      solver.CalculateFitness(assignments, data),
		gaps:       q.Even.GroupGaps + q.Odd.GroupGaps,
		longGaps:   q.Even.GroupLongGaps + q.Odd.GroupLongGaps,
		singleDays: q.Even.SingleClassDays + q.Odd.SingleClassDays,
		saturday:   q.Even.SaturdayPairs + q.Odd.SaturdayPairs,
	}
}
