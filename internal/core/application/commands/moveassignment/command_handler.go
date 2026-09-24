package moveassignment

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/application/rules"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Handler выполняет сценарий «перенести пару вручную».
type Handler struct {
	input  ports.InputRepository
	output ports.OutputRepository
}

func NewHandler(input ports.InputRepository, output ports.OutputRepository) *Handler {
	return &Handler{input: input, output: output}
}

// Handle переносит пару, если это не нарушает жёстких ограничений, и пересчитывает score.
// При конфликте расписание не меняется, ошибка — *rules.ConflictError.
func (h *Handler) Handle(ctx context.Context, cmd Command) (*domain.Schedule, error) {
	sched, err := h.output.GetSchedule(ctx, cmd.ScheduleID)
	if err != nil {
		return nil, err
	}
	if cmd.Index >= len(sched.Assignments) {
		return nil, fmt.Errorf("%w: assignment index %d out of range", domain.ErrInvalidInput, cmd.Index)
	}
	data, err := h.input.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}

	moved := sched.Assignments[cmd.Index]
	moved.TimeSlot = cmd.TimeSlot
	if cmd.RoomID != "" {
		moved.RoomID = cmd.RoomID
		// Корпус пары — корпус её аудитории: от него зависят переходы между корпусами.
		moved.BuildingID = roomBuilding(data.Rooms, cmd.RoomID, moved.BuildingID)
	}
	if cmd.Parity != "" {
		moved.Parity = cmd.Parity
	}

	if conflict := rules.Check(sched.Assignments, cmd.Index, moved, data.Teachers, false); conflict != nil {
		return nil, conflict
	}

	sched.Assignments[cmd.Index] = moved
	data.Preferences = sched.Options
	sched.Score = solver.CalculateFitness(sched.Assignments, *data)
	if err := h.output.UpdateSchedule(ctx, sched); err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	return sched, nil
}

// roomBuilding — корпус аудитории roomID; если аудитории нет в справочнике — fallback.
func roomBuilding(rooms []domain.Room, roomID, fallback string) string {
	for _, r := range rooms {
		if r.ID == roomID {
			return r.BuildingID
		}
	}
	return fallback
}
