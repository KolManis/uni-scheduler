package app

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// GetSchedule — расписание целиком.
func (s *Service) GetSchedule(ctx context.Context, id int64) (*domain.Schedule, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	return s.outputRepo.GetSchedule(ctx, id)
}

// ListSchedules — список расписаний без пар, свежие сверху.
func (s *Service) ListSchedules(ctx context.Context) ([]domain.ScheduleSummary, error) {
	return s.outputRepo.ListSchedules(ctx)
}

// Breakdown — из чего складывается score расписания.
// Нужны справочники: спортзал и стадион не штрафуются за переходы между корпусами.
func (s *Service) Breakdown(sched *domain.Schedule, input domain.InputData) domain.FitnessBreakdown {
	input.Preferences = sched.Options
	return solver.CalculateFitnessBreakdown(sched.Assignments, input)
}

// Quality — показатели расписания в штуках: окна, дни с одной парой, суббота.
func (s *Service) Quality(sched *domain.Schedule) domain.WeekQuality {
	return solver.CalculateQuality(sched.Assignments)
}
