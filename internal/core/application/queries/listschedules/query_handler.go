// Package listschedules — запрос списка расписаний (без пар, свежие сверху).
// Параметров нет, поэтому и отдельного типа Query нет.
package listschedules

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Handler выполняет запрос списка расписаний.
type Handler struct {
	input  ports.InputRepository
	output ports.OutputRepository
}

func NewHandler(input ports.InputRepository, output ports.OutputRepository) *Handler {
	return &Handler{input: input, output: output}
}

// Handle — список расписаний со score, пересчитанным по текущим правилам и справочникам,
// как на странице расписания (evaluateschedule). Сохранённый при генерации score мог быть
// посчитан по старым правилам (до ADR-0020 — среднее недель), и тогда список и страница
// расписания показывали бы разные числа.
func (h *Handler) Handle(ctx context.Context) ([]domain.ScheduleSummary, error) {
	list, err := h.output.ListSchedules(ctx)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return list, nil
	}
	input, err := h.input.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}
	for k := range list {
		sched, err := h.output.GetSchedule(ctx, list[k].ID)
		if err != nil {
			return nil, err
		}
		withPrefs := *input
		withPrefs.Preferences = sched.Options
		list[k].Score = solver.CalculateFitness(sched.Assignments, withPrefs)
	}
	return list, nil
}
