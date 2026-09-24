// Package evaluateschedule — оценка готового расписания: из чего складывается score и
// показатели качества в штуках. Ничего не читает из хранилища: расписание и справочники
// передаёт вызывающий.
package evaluateschedule

import (
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Query — расписание и справочники, по которым его оценить. Справочники нужны: спортзал и
// стадион не штрафуются за переходы между корпусами.
type Query struct {
	Schedule *domain.Schedule
	Input    domain.InputData
}

// Result — оценка расписания.
type Result struct {
	Breakdown domain.FitnessBreakdown // из чего складывается score
	Quality   domain.WeekQuality      // окна, дни с одной парой, суббота — по неделям
}

// Handler выполняет оценку расписания.
type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) Handle(q Query) Result {
	input := q.Input
	input.Preferences = q.Schedule.Options
	return Result{
		Breakdown: solver.CalculateFitnessBreakdown(q.Schedule.Assignments, input),
		Quality:   solver.CalculateQuality(q.Schedule.Assignments),
	}
}
