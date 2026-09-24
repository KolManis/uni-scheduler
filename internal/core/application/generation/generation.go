// Package generation — общая часть команд generateschedule и generateallmethods:
// подготовить данные для солвера, составить расписание и сохранить его.
package generation

import (
	"context"
	"fmt"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Request — параметры генерации, одинаковые для обеих команд.
type Request struct {
	Name         string
	SolverType   string              // построение: "" или "dsatur" — самая трудная пара первой, "teacher" — по преподавателям
	TimeoutSec   int                 // сколько ждать результата; 0 — 120 секунд
	SemesterHalf domain.SemesterHalf // "second" — без планов, которые идут только в первой половине семестра
	ImproveAlgo  string              // метод улучшения: "hillclimb" (по умолчанию) | "sa" | "tabu" | "ga" | "lns"
	// ParallelStarts — сколько раз составить с разным порядком и взять лучшее; 0 и 1 — один раз.
	ParallelStarts int
	Preferences    domain.SolverPreferences // дополнительные правила, по умолчанию выключены
	// BaseScheduleID — перегенерация: закреплённые пары этого расписания остаются на
	// местах, остальное строится заново вокруг них. 0 — составить с нуля.
	BaseScheduleID int64
}

// Значения по умолчанию.
const (
	DefaultTimeoutSec = 120
	// saveReserve — сколько таймаута оставить на сохранение: солверу достаётся остальное.
	saveReserve = 15 * time.Second
	// minSolverBudget — меньше этого улучшение не успевает сойтись на реальных данных.
	minSolverBudget = 30 * time.Second
)

// Validate проверяет запрос и подставляет значения по умолчанию.
func (r Request) Validate() (Request, error) {
	if _, ok := solver.ParseConstruction(r.SolverType); !ok {
		return r, fmt.Errorf("%w: solver_type %q не поддерживается, доступны \"dsatur\" и \"teacher\"",
			domain.ErrInvalidInput, r.SolverType)
	}
	if r.Name == "" {
		r.Name = "Untitled"
	}
	if r.TimeoutSec <= 0 {
		r.TimeoutSec = DefaultTimeoutSec
	}
	return r, nil
}

// Timeout — сколько ждать результата.
func (r Request) Timeout() time.Duration {
	return time.Duration(r.TimeoutSec) * time.Second
}

// Generator готовит данные, запускает солвер и сохраняет результат.
type Generator struct {
	input  ports.InputRepository
	output ports.OutputRepository
}

func NewGenerator(input ports.InputRepository, output ports.OutputRepository) *Generator {
	return &Generator{input: input, output: output}
}

// Job — всё, что нужно солверу: данные, закреплённые пары, построение, бюджет времени.
type Job struct {
	Request   Request
	data      domain.InputData
	fixed     []domain.Assignment
	construct solver.Construction
	budget    time.Duration
}

// Construction — выбранный алгоритм построения.
func (j *Job) Construction() solver.Construction { return j.construct }

// Prepare загружает справочники и закреплённые пары и считает бюджет солвера.
// Запрос должен быть проверен (Request.Validate).
func (g *Generator) Prepare(ctx context.Context, req Request) (*Job, error) {
	construct, _ := solver.ParseConstruction(req.SolverType)

	data, err := g.input.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}
	if req.SemesterHalf == domain.HalfSecond {
		data.SubjectPlans = withoutFirstHalfOnly(data.SubjectPlans)
	}
	data.Preferences = req.Preferences

	fixed, err := g.pinnedOf(ctx, req.BaseScheduleID)
	if err != nil {
		return nil, err
	}

	budget := req.Timeout() - saveReserve
	if budget < minSolverBudget {
		budget = minSolverBudget
	}
	return &Job{Request: req, data: *data, fixed: fixed, construct: construct, budget: budget}, nil
}

// SolveAndSave составляет одно расписание методом improve (starts стартов) и сохраняет
// его под именем name. Сохраняет с context.Background(): расписание попадёт в список,
// даже если пользователь уже закрыл страницу.
func (g *Generator) SolveAndSave(job *Job, improve solver.ImproveAlgorithm, starts int, name string) (*domain.Schedule, error) {
	sched, err := solver.Solve(job.data, solver.Options{Construction: job.construct, Improve: improve, Budget: job.budget, Starts: starts, Fixed: job.fixed})
	if err != nil {
		return nil, err
	}
	sched.Name = name
	sched.Options = job.Request.Preferences
	sched.Options.Construction = string(job.construct)
	sched.Unplaced = explainUnplaced(solver.ComputeUnplaced(sched.Assignments, job.data), sched.Assignments, job.data)

	saved, err := g.output.SaveSchedule(context.Background(), sched)
	if err != nil {
		return nil, fmt.Errorf("save schedule: %w", err)
	}
	return saved, nil
}

// pinnedOf — закреплённые пары расписания id; id == 0 — перегенерации нет.
func (g *Generator) pinnedOf(ctx context.Context, id int64) ([]domain.Assignment, error) {
	if id <= 0 {
		return nil, nil
	}
	base, err := g.output.GetSchedule(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("base schedule %d: %w", id, err)
	}
	var fixed []domain.Assignment
	for _, a := range base.Assignments {
		if a.Pinned {
			fixed = append(fixed, a)
		}
	}
	return fixed, nil
}

// withoutFirstHalfOnly — планы без тех, что идут только в первой половине семестра.
func withoutFirstHalfOnly(plans []domain.SubjectPlan) []domain.SubjectPlan {
	var out []domain.SubjectPlan
	for _, sp := range plans {
		if sp.SemesterHalf != domain.HalfFirst {
			out = append(out, sp)
		}
	}
	return out
}
