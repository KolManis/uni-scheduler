package replayschedule

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/application/generation"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Handler выполняет сценарий «повторить запуск».
type Handler struct {
	output    ports.OutputRepository
	generator *generation.Generator
}

func NewHandler(output ports.OutputRepository, generator *generation.Generator) *Handler {
	return &Handler{output: output, generator: generator}
}

// Handle составляет расписание заново с записанными сидами и числом раундов и сохраняет
// его как новое. На тех же справочниках получится то же расписание; если справочники с
// тех пор меняли — другое, и это видно по сравнению score.
//
// Закреплённые пары исходного расписания снова закрепляются: при исходном запуске это
// были пары его базового расписания, и в результате они стоят закреплёнными.
func (h *Handler) Handle(ctx context.Context, cmd Command) (*domain.Schedule, error) {
	orig, err := h.output.GetSchedule(ctx, cmd.ScheduleID)
	if err != nil {
		return nil, err
	}
	run := orig.Run
	if run.Rounds == 0 {
		return nil, fmt.Errorf("%w: у расписания не записан запуск (составлено до ADR-0023) — повторить нельзя",
			domain.ErrInvalidInput)
	}
	prefs := orig.Options
	prefs.Construction = ""
	req, err := generation.Request{
		Name:           orig.Name + " — повтор",
		SolverType:     run.Construction,
		SemesterHalf:   domain.SemesterHalf(run.SemesterHalf),
		ImproveAlgo:    run.Improve,
		Preferences:    prefs,
		BaseScheduleID: orig.ID,
		Replay:         &run,
	}.Validate()
	if err != nil {
		return nil, err
	}
	job, err := h.generator.Prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	return h.generator.SolveAndSave(job, solver.ImproveAlgorithm(run.Improve), 1, req.Name)
}
