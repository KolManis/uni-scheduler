// Package evaluateschedule — оценка готового расписания: score за две недели, суммы по
// правилам, каждое нарушение отдельно и показатели качества в штуках.
package evaluateschedule

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Result — оценка расписания.
type Result struct {
	Schedule   *domain.Schedule
	Score      int                     // score за две недели (чётная + нечётная), пересчитан по текущим правилам
	Breakdown  domain.FitnessBreakdown // суммы по правилам
	Violations []domain.Violation      // каждое нарушение: у кого, когда, что и сколько стоит
	Quality    domain.WeekQuality      // окна, дни с одной парой, суббота — по неделям
}

// Handler выполняет запрос оценки расписания.
type Handler struct {
	input  ports.InputRepository
	output ports.OutputRepository
}

func NewHandler(input ports.InputRepository, output ports.OutputRepository) *Handler {
	return &Handler{input: input, output: output}
}

// Handle пересчитывает оценку по справочникам и правилам, с которыми расписание составлено.
// Справочники нужны: спортзал и стадион не штрафуются за переходы между корпусами.
func (h *Handler) Handle(ctx context.Context, q Query) (Result, error) {
	sched, err := h.output.GetSchedule(ctx, q.ScheduleID)
	if err != nil {
		return Result{}, err
	}
	input, err := h.input.LoadInput(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("load input: %w", err)
	}
	input.Preferences = sched.Options

	breakdown := solver.CalculateFitnessBreakdown(sched.Assignments, *input)
	return Result{
		Schedule:   sched,
		Score:      breakdown.Total(),
		Breakdown:  breakdown,
		Violations: solver.ExplainScore(sched.Assignments, *input),
		Quality:    solver.CalculateQuality(sched.Assignments),
	}, nil
}
