package generateallmethods

import (
	"context"
	"fmt"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/application/generation"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// methods — методы улучшения для сравнения и подпись в названии расписания.
var methods = []struct {
	algo  solver.ImproveAlgorithm
	label string
}{
	{solver.ImproveHillClimb, "hillclimb"},
	{solver.ImproveSimulatedAnnealing, "SA"},
	{solver.ImproveTabuSearch, "tabu"},
	{solver.ImproveGeneticAlgorithm, "GA"},
	{solver.ImproveLNS, "LNS"},
}

// Handler выполняет сценарий «составить всеми методами и сравнить».
type Handler struct {
	generator *generation.Generator
}

func NewHandler(generator *generation.Generator) *Handler {
	return &Handler{generator: generator}
}

// Handle запускает все методы параллельно и сохраняет каждое расписание отдельно
// («<имя> — <метод>»). Возвращает те, что успели до таймаута; остальные сохранятся в фоне.
func (h *Handler) Handle(ctx context.Context, cmd Command) ([]*domain.Schedule, error) {
	job, err := h.generator.Prepare(ctx, cmd.Request)
	if err != nil {
		return nil, err
	}
	prefix := cmd.Name + " — "
	if job.Construction() == solver.ConstructDSatur {
		prefix += "DSatur, "
	}

	type result struct {
		sched *domain.Schedule
		err   error
	}
	done := make(chan result, len(methods))
	for _, m := range methods {
		go func() {
			// Один старт на метод: пять методов уже занимают процессор.
			sched, err := h.generator.SolveAndSave(job, m.algo, 1, prefix+m.label)
			if err != nil {
				err = fmt.Errorf("%s: %w", m.label, err)
			}
			done <- result{sched, err}
		}()
	}

	var saved []*domain.Schedule
	var firstErr error
	timeout := time.After(cmd.Timeout())
wait:
	for range methods {
		select {
		case r := <-done:
			if r.err != nil && firstErr == nil {
				firstErr = r.err
			}
			if r.sched != nil {
				saved = append(saved, r.sched)
			}
		case <-timeout:
			break wait
		case <-ctx.Done():
			break wait
		}
	}

	if len(saved) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("no method finished within %ds", cmd.TimeoutSec)
	}
	return saved, nil
}
