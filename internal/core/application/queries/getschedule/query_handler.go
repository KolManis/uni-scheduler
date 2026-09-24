package getschedule

import (
	"context"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
)

// Handler выполняет запрос расписания.
type Handler struct {
	output ports.OutputRepository
}

func NewHandler(output ports.OutputRepository) *Handler {
	return &Handler{output: output}
}

// Handle возвращает расписание; domain.ErrNotFound — такого нет.
func (h *Handler) Handle(ctx context.Context, q Query) (*domain.Schedule, error) {
	return h.output.GetSchedule(ctx, q.ScheduleID)
}
