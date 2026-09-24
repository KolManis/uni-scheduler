package app

import (
	"context"
	"fmt"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// GenerateCommand — составить расписание.
type GenerateCommand struct {
	Name          string
	MaxIterations int
	SolverType    string              // построение: "" или "dsatur" — самая трудная пара первой, "teacher" — по преподавателям
	TimeoutSec    int                 // сколько ждать результата; 0 — 120 секунд
	SemesterHalf  domain.SemesterHalf // "second" — без планов, которые идут только в первой половине семестра
	ImproveAlgo   string              // метод улучшения: "hillclimb" (по умолчанию) | "sa" | "tabu" | "ga" | "lns"
	// ParallelStarts — сколько раз составить с разным порядком и взять лучшее; 0 и 1 — один раз.
	ParallelStarts int
	Preferences    domain.SolverPreferences // дополнительные правила, по умолчанию выключены
	// BaseScheduleID — перегенерация: закреплённые пары этого расписания остаются на
	// местах, остальное строится заново вокруг них. 0 — составить с нуля.
	BaseScheduleID int64
}

// Значения по умолчанию.
const (
	defaultTimeoutSec    = 120
	defaultMaxIterations = 50000
	// saveReserve — сколько секунд таймаута оставить на сохранение: солверу достаётся остальное.
	saveReserve = 15 * time.Second
	// minSolverBudget — меньше этого улучшение не успевает сойтись на реальных данных.
	minSolverBudget = 30 * time.Second
)

// generation — всё, что нужно солверу: данные, закреплённые пары, алгоритм построения, бюджет.
type generation struct {
	cmd       GenerateCommand
	data      domain.InputData
	fixed     []domain.Assignment
	construct solver.Construction
	budget    time.Duration
}

// prepareGeneration проверяет команду, подставляет значения по умолчанию и загружает данные.
func (s *Service) prepareGeneration(ctx context.Context, cmd GenerateCommand) (*generation, error) {
	construct, ok := solver.ParseConstruction(cmd.SolverType)
	if !ok {
		return nil, fmt.Errorf("%w: solver_type %q не поддерживается, доступны \"dsatur\" и \"teacher\"", ErrInvalidInput, cmd.SolverType)
	}
	if cmd.Name == "" {
		cmd.Name = "Untitled"
	}
	if cmd.MaxIterations <= 0 {
		cmd.MaxIterations = defaultMaxIterations
	}
	if cmd.TimeoutSec <= 0 {
		cmd.TimeoutSec = defaultTimeoutSec
	}

	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}
	if cmd.SemesterHalf == domain.HalfSecond {
		data.SubjectPlans = withoutFirstHalfOnly(data.SubjectPlans)
	}
	data.Preferences = cmd.Preferences

	fixed, err := s.pinnedOf(ctx, cmd.BaseScheduleID)
	if err != nil {
		return nil, err
	}

	budget := time.Duration(cmd.TimeoutSec)*time.Second - saveReserve
	if budget < minSolverBudget {
		budget = minSolverBudget
	}
	return &generation{cmd: cmd, data: *data, fixed: fixed, construct: construct, budget: budget}, nil
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

// pinnedOf — закреплённые пары расписания id; id == 0 — перегенерации нет.
func (s *Service) pinnedOf(ctx context.Context, id int64) ([]domain.Assignment, error) {
	if id <= 0 {
		return nil, nil
	}
	base, err := s.outputRepo.GetSchedule(ctx, id)
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

// solveAndSave составляет одно расписание методом improve и сохраняет его под именем name.
func (s *Service) solveAndSave(g *generation, improve solver.ImproveAlgorithm, starts int, name string) (*domain.Schedule, error) {
	sched, err := solver.SolveMultiStartFixed(g.data, g.construct, g.cmd.MaxIterations, improve, starts, g.budget, g.fixed)
	if err != nil {
		return nil, err
	}
	sched.Name = name
	sched.Options = g.cmd.Preferences
	sched.Options.Construction = string(g.construct)
	sched.Unplaced = explainUnplaced(solver.ComputeUnplaced(sched.Assignments, g.data), sched.Assignments, g.data)
	// context.Background(): расписание сохраняется, даже если пользователь закрыл страницу.
	saved, err := s.outputRepo.SaveSchedule(context.Background(), sched)
	if err != nil {
		return nil, fmt.Errorf("save schedule: %w", err)
	}
	return saved, nil
}

// GenerateSchedule составляет одно расписание выбранным методом и сохраняет его.
//
// Солвер работает в фоне: если пользователь ушёл со страницы или истёк таймаут, ответ
// приходит с ошибкой, но генерация доходит до конца и расписание появится в списке.
func (s *Service) GenerateSchedule(ctx context.Context, cmd GenerateCommand) (*domain.Schedule, error) {
	g, err := s.prepareGeneration(ctx, cmd)
	if err != nil {
		return nil, err
	}

	type result struct {
		sched *domain.Schedule
		err   error
	}
	done := make(chan result, 1)
	go func() {
		sched, err := s.solveAndSave(g, solver.ImproveAlgorithm(g.cmd.ImproveAlgo), g.cmd.ParallelStarts, g.cmd.Name)
		done <- result{sched, err}
	}()

	timeout := time.After(time.Duration(g.cmd.TimeoutSec) * time.Second)
	select {
	case r := <-done:
		return r.sched, r.err
	case <-timeout:
		return nil, fmt.Errorf("solver timeout after %ds", g.cmd.TimeoutSec)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// allMethods — методы улучшения для сравнения и подпись в названии расписания.
var allMethods = []struct {
	algo  solver.ImproveAlgorithm
	label string
}{
	{solver.ImproveHillClimb, "hillclimb"},
	{solver.ImproveSimulatedAnnealing, "SA"},
	{solver.ImproveTabuSearch, "tabu"},
	{solver.ImproveGeneticAlgorithm, "GA"},
	{solver.ImproveLNS, "LNS"},
}

// GenerateAllMethods составляет расписание всеми методами улучшения параллельно и
// сохраняет каждое отдельно («<имя> — <метод>»), чтобы пользователь выбрал лучшее.
// Возвращает те, что успели до таймаута; остальные сохранятся в фоне.
// ParallelStarts игнорируется: пять методов уже занимают процессор.
func (s *Service) GenerateAllMethods(ctx context.Context, cmd GenerateCommand) ([]*domain.Schedule, error) {
	g, err := s.prepareGeneration(ctx, cmd)
	if err != nil {
		return nil, err
	}
	prefix := g.cmd.Name + " — "
	if g.construct == solver.ConstructDSatur {
		prefix += "DSatur, "
	}

	type result struct {
		sched *domain.Schedule
		err   error
	}
	done := make(chan result, len(allMethods))
	for _, m := range allMethods {
		go func() {
			sched, err := s.solveAndSave(g, m.algo, 1, prefix+m.label)
			if err != nil {
				err = fmt.Errorf("%s: %w", m.label, err)
			}
			done <- result{sched, err}
		}()
	}

	var saved []*domain.Schedule
	var firstErr error
	timeout := time.After(time.Duration(g.cmd.TimeoutSec) * time.Second)
wait:
	for range allMethods {
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
		return nil, fmt.Errorf("no method finished within %ds", g.cmd.TimeoutSec)
	}
	return saved, nil
}
