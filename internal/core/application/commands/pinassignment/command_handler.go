package pinassignment

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
)

// Handler выполняет сценарий «закрепить пару».
type Handler struct {
	output ports.OutputRepository
}

func NewHandler(output ports.OutputRepository) *Handler {
	return &Handler{output: output}
}

func (h *Handler) Handle(ctx context.Context, cmd Command) (*domain.Schedule, error) {
	sched, err := h.output.GetSchedule(ctx, cmd.ScheduleID)
	if err != nil {
		return nil, err
	}
	if cmd.Index >= len(sched.Assignments) {
		return nil, fmt.Errorf("%w: assignment index %d out of range", domain.ErrInvalidInput, cmd.Index)
	}
	sched.Assignments[cmd.Index].Pinned = cmd.Pinned
	if err := h.output.UpdateSchedule(ctx, sched); err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	return sched, nil
}
