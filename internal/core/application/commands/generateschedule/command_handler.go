package generateschedule

import (
	"context"
	"fmt"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/application/generation"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Handler выполняет сценарий «составить расписание».
type Handler struct {
	generator *generation.Generator
}

func NewHandler(generator *generation.Generator) *Handler {
	return &Handler{generator: generator}
}

// Handle составляет расписание и сохраняет его.
//
// Солвер работает в фоне: если пользователь ушёл со страницы или истёк таймаут, ответ
// приходит с ошибкой, но генерация доходит до конца и расписание появится в списке.
func (h *Handler) Handle(ctx context.Context, cmd Command) (*domain.Schedule, error) {
	job, err := h.generator.Prepare(ctx, cmd.Request)
	if err != nil {
		return nil, err
	}

	type result struct {
		sched *domain.Schedule
		err   error
	}
	done := make(chan result, 1)
	go func() {
		sched, err := h.generator.SolveAndSave(job, solver.ImproveAlgorithm(cmd.ImproveAlgo), cmd.ParallelStarts, cmd.Name)
		done <- result{sched, err}
	}()

	select {
	case r := <-done:
		return r.sched, r.err
	case <-time.After(cmd.Timeout()):
		return nil, fmt.Errorf("solver timeout after %ds", cmd.TimeoutSec)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
