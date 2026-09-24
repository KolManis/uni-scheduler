// Package listschedules — запрос списка расписаний (без пар, свежие сверху).
// Параметров нет, поэтому и отдельного типа Query нет.
package listschedules

import (
	"context"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
)

// Handler выполняет запрос списка расписаний.
type Handler struct {
	output ports.OutputRepository
}

func NewHandler(output ports.OutputRepository) *Handler {
	return &Handler{output: output}
}

func (h *Handler) Handle(ctx context.Context) ([]domain.ScheduleSummary, error) {
	return h.output.ListSchedules(ctx)
}
