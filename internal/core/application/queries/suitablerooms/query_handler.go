package suitablerooms

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Handler выполняет запрос «в какие аудитории можно поставить пару».
type Handler struct {
	input  ports.InputRepository
	output ports.OutputRepository
}

func NewHandler(input ports.InputRepository, output ports.OutputRepository) *Handler {
	return &Handler{input: input, output: output}
}

// Handle — аудитории, подходящие паре по типу, вместимости и корпусу (HC4–HC6), лучшие по
// вместимости первыми. Время не учитывается: занята ли аудитория, проверяет перенос.
func (h *Handler) Handle(ctx context.Context, q Query) ([]domain.Room, error) {
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
	return solver.SuitableRooms(sched.Assignments[q.Index], *data), nil
}
