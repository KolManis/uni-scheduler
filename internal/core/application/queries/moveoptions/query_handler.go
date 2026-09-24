package moveoptions

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/application/rules"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Option — можно ли перенести пару в этот слот, в какую аудиторию и что изменится.
type Option struct {
	Slot     domain.TimeSlot      `json:"time_slot"`
	Current  bool                 `json:"current,omitempty"` // пара стоит здесь сейчас
	RoomID   string               `json:"room_id,omitempty"` // своя аудитория, если свободна, иначе другая подходящая
	Conflict *rules.ConflictError `json:"conflict,omitempty"`
	// Изменения при переносе, сумма по чётной и нечётной неделе; отрицательное — лучше.
	ScoreDelta      int `json:"score_delta"`
	GapsDelta       int `json:"gaps_delta"`
	LongGapsDelta   int `json:"long_gaps_delta"`
	SingleDaysDelta int `json:"single_days_delta"`
	SaturdayDelta   int `json:"saturday_delta"`
}

// Handler выполняет запрос «куда можно перенести пару».
type Handler struct {
	input  ports.InputRepository
	output ports.OutputRepository
}

func NewHandler(input ports.InputRepository, output ports.OutputRepository) *Handler {
	return &Handler{input: input, output: output}
}

// Handle — для каждого из 36 слотов: можно ли перенести туда пару (с её чётностью) и
// как это изменит score и показатели качества.
func (h *Handler) Handle(ctx context.Context, q Query) ([]Option, error) {
	sched, err := h.output.GetSchedule(ctx, q.ScheduleID)
	if err != nil {
		return nil, err
	}
	if q.Index >= len(sched.Assignments) {
		return nil, fmt.Errorf("%w: assignment index %d out of range", domain.ErrInvalidInput, q.Index)
	}
	data, err := h.input.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}
	data.Preferences = sched.Options

	before := measure(sched.Assignments, *data)
	pair := sched.Assignments[q.Index]
	rooms := solver.SuitableRooms(pair, *data)

	var options []Option
	for _, day := range domain.AllDays {
		for num := domain.FirstPair; num <= domain.LastPair; num++ {
			slot := domain.MustNewTimeSlot(day, num)
			options = append(options, option(sched.Assignments, q.Index, slot, rooms, *data, before))
		}
	}
	return options, nil
}

// option — один слот: сначала жёсткие ограничения, потом аудитория, потом изменения.
func option(assignments []domain.Assignment, index int, slot domain.TimeSlot, rooms []domain.Room,
	data domain.InputData, before measurement) Option {

	pair := assignments[index]
	opt := Option{Slot: slot, RoomID: pair.RoomID}
	if slot == pair.TimeSlot {
		opt.Current = true
		return opt
	}

	moved := pair
	moved.TimeSlot = slot
	if c := rules.Check(assignments, index, moved, data.Teachers, true); c != nil {
		opt.Conflict = c
		return opt
	}
	room, ok := rules.FreeRoom(assignments, index, moved, rooms)
	if !ok {
		opt.Conflict = &rules.ConflictError{Type: rules.ConflictRoomBusy, ResourceID: pair.RoomID, ConflictWith: -1}
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
