package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

var ErrInvalidInput = errors.New("invalid input")

// GenerateInput — параметры запроса на генерацию расписания.
type GenerateInput struct {
	Name          string
	MaxIterations int
	SolverType    string              // "subject" | "teacher" (default)
	TimeoutSec    int                 // 0 → используется дефолт 30 сек
	SemesterHalf  domain.SemesterHalf // "" | "full" | "first" | "second"
	// "first"  → все планы (1-я половина семестра, лекции ещё идут)
	// "second" → исключить планы с semester_half="first" (2-я половина, лекции закончились)
	// "" / "full" → всё без фильтрации (по умолчанию)
	ImproveAlgo    string // "hillclimb" (default) | "sa" | "tabu" | "ga" | "lns"
	ParallelStarts int    // 0/1 — один запуск (по умолчанию), N>1 — многостартовый параллельный поиск
}

// PatchRequest — запрос на изменение одного назначения.
type PatchRequest struct {
	TimeSlot domain.TimeSlot
	RoomID   string
	Parity   domain.Parity
}

// ConflictError описывает нарушение жёсткого ограничения при PATCH.
type ConflictError struct {
	Type         string `json:"type"` // "teacher_busy" | "group_busy" | "room_busy"
	ResourceID   string `json:"resource_id"`
	ConflictWith int    `json:"conflict_with"` // индекс конфликтующего assignment
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict: %s resource=%s with_idx=%d", e.Type, e.ResourceID, e.ConflictWith)
}

// Service реализует бизнес-логику работы с расписаниями.
type Service struct {
	inputRepo  ports.InputRepository
	outputRepo ports.OutputRepository
	importRepo ports.ImportRepository
}

func NewService(inputRepo ports.InputRepository, outputRepo ports.OutputRepository, importRepo ports.ImportRepository) *Service {
	return &Service{
		inputRepo:  inputRepo,
		outputRepo: outputRepo,
		importRepo: importRepo,
	}
}

// Generate запускает генерацию расписания.
func (s *Service) Generate(ctx context.Context, in GenerateInput) (*domain.Schedule, error) {
	if in.Name == "" {
		in.Name = "Untitled"
	}
	if in.MaxIterations <= 0 {
		in.MaxIterations = 50000
	}
	if in.TimeoutSec <= 0 {
		in.TimeoutSec = 120
	}

	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}

	// Фильтрация планов по половине семестра
	if in.SemesterHalf == domain.HalfSecond {
		// 2-я половина: убираем планы, которые идут только в 1-й половине
		filtered := data.SubjectPlans[:0]
		for _, sp := range data.SubjectPlans {
			if sp.SemesterHalf != domain.HalfFirst {
				filtered = append(filtered, sp)
			}
		}
		data.SubjectPlans = filtered
	}

	solveCtx, cancel := context.WithTimeout(ctx, time.Duration(in.TimeoutSec)*time.Second)
	defer cancel()

	type solveResult struct {
		sched *domain.Schedule
		err   error
	}
	ch := make(chan solveResult, 1)

	go func() {
		var result *domain.Schedule
		var solveErr error

		improve := solver.ImproveAlgorithm(in.ImproveAlgo)
		switch in.SolverType {
		case "subject":
			result, solveErr = solver.SolveParallel(*data, in.MaxIterations, 4)
		default:
			starts := in.ParallelStarts
			if starts <= 1 {
				result, solveErr = solver.SolveTeacher(*data, in.MaxIterations, improve)
			} else {
				result, solveErr = solver.SolveTeacherMultiStart(*data, in.MaxIterations, improve, starts)
			}
		}
		ch <- solveResult{result, solveErr}
	}()

	select {
	case <-solveCtx.Done():
		return nil, fmt.Errorf("solver timeout after %ds", in.TimeoutSec)
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		res.sched.Name = in.Name
		res.sched.Unplaced = solver.ComputeUnplaced(res.sched.Assignments, *data)
		saved, err := s.outputRepo.SaveSchedule(ctx, res.sched)
		if err != nil {
			return nil, fmt.Errorf("save schedule: %w", err)
		}
		return saved, nil
	}
}

// GenerateAllMethods запускает все пять улучшающих алгоритмов параллельно и сохраняет
// пять расписаний с суффиксами имени. Каждая горутина строит своё расписание с нуля
// (общего состояния нет), поэтому они не мешают друг другу; вычисляется fitness по
// одному и тому же критерию, так что результаты сравнимы.
//
// Смысл — дать возможность увидеть, какой метод даёт лучшее решение на конкретных
// данных: разброс между методами велик и стохастичен, «победитель» плавает от прогона
// к прогону. Пользователь сам смотрит список и выбирает наилучший.
//
// Общий таймаут применяется ко ВСЕМ пяти прогонам: если задан 120 сек, каждый метод
// имеет 120 сек, а горутины идут параллельно — весь вызов уложится в те же 120 сек.
func (s *Service) GenerateAllMethods(ctx context.Context, in GenerateInput) ([]*domain.Schedule, error) {
	if in.Name == "" {
		in.Name = "Untitled"
	}
	if in.MaxIterations <= 0 {
		in.MaxIterations = 50000
	}
	if in.TimeoutSec <= 0 {
		in.TimeoutSec = 120
	}

	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}
	if in.SemesterHalf == domain.HalfSecond {
		filtered := data.SubjectPlans[:0]
		for _, sp := range data.SubjectPlans {
			if sp.SemesterHalf != domain.HalfFirst {
				filtered = append(filtered, sp)
			}
		}
		data.SubjectPlans = filtered
	}

	solveCtx, cancel := context.WithTimeout(ctx, time.Duration(in.TimeoutSec)*time.Second)
	defer cancel()

	methods := []struct {
		algo   solver.ImproveAlgorithm
		suffix string
	}{
		{solver.ImproveHillClimb, "hillclimb"},
		{solver.ImproveSimulatedAnnealing, "SA"},
		{solver.ImproveTabuSearch, "tabu"},
		{solver.ImproveGeneticAlgorithm, "GA"},
		{solver.ImproveLNS, "LNS"},
	}

	type genResult struct {
		sched *domain.Schedule
		err   error
	}
	results := make([]genResult, len(methods))
	done := make(chan int, len(methods))

	// При «все методы» игнорируем ParallelStarts: 5 методов уже дают 5 параллельных
	// горутин. Если умножать на 3-8 стартов внутри каждого, получаем 15-40 горутин,
	// конкурирующих за ~4-8 ядер CPU. Каждая метаэвристика с таймером 60 сек не
	// успевает за общий wall-clock, converge не досходится, score деградирует в 5-7 раз
	// (проверено: 8 стартов × 5 методов = score 170k-213k против одиночных ~30k).
	for i, m := range methods {
		go func(idx int, algo solver.ImproveAlgorithm, suffix string) {
			sched, solveErr := solver.SolveTeacher(*data, in.MaxIterations, algo)
			results[idx] = genResult{sched: sched, err: solveErr}
			done <- idx
		}(i, m.algo, m.suffix)
	}

	// Ждём завершения всех горутин ЛИБО общего таймаута.
	completed := 0
	for completed < len(methods) {
		select {
		case <-solveCtx.Done():
			// Дождёмся уже запущенных горутин снаружи цикла: они всё равно допишут в results.
			// Но мы прекращаем ждать новых — то, что успело, то и сохраним.
			completed = len(methods)
		case <-done:
			completed++
		}
	}

	// Сохраняем всё, что успело сойтись.
	var saved []*domain.Schedule
	var firstErr error
	for i, r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", methods[i].suffix, r.err)
			}
			continue
		}
		if r.sched == nil {
			continue // горутина не успела до таймаута
		}
		r.sched.Name = in.Name + " — " + methods[i].suffix
		r.sched.Unplaced = solver.ComputeUnplaced(r.sched.Assignments, *data)
		out, err := s.outputRepo.SaveSchedule(ctx, r.sched)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("save %s: %w", methods[i].suffix, err)
			}
			continue
		}
		saved = append(saved, out)
	}
	if len(saved) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("no method finished within %ds", in.TimeoutSec)
	}
	return saved, nil
}

// GetByID возвращает расписание по ID.
func (s *Service) GetByID(ctx context.Context, id int64) (*domain.Schedule, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	return s.outputRepo.GetSchedule(ctx, id)
}

// List возвращает краткий список расписаний (без assignments).
func (s *Service) List(ctx context.Context) ([]domain.ScheduleSummary, error) {
	return s.outputRepo.ListSchedules(ctx)
}

// Breakdown раскладывает штраф расписания по категориям.
//
// Сценарий нужен как входная точка ядра: веб-интерфейс не должен вызывать алгоритм напрямую,
// иначе адаптер начинает зависеть от внутренностей солвера в обход слоя сценариев.
func (s *Service) Breakdown(sched *domain.Schedule) domain.FitnessBreakdown {
	return solver.CalculateFitnessBreakdown(sched.Assignments, domain.InputData{})
}

// Delete удаляет расписание по ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	return s.outputRepo.DeleteSchedule(ctx, id)
}

// PatchAssignment изменяет одно назначение и проверяет HC1–HC3.
func (s *Service) PatchAssignment(ctx context.Context, schedID int64, idx int, req PatchRequest) (*domain.Schedule, error) {
	sched, err := s.outputRepo.GetSchedule(ctx, schedID)
	if err != nil {
		return nil, err
	}
	if idx < 0 || idx >= len(sched.Assignments) {
		return nil, fmt.Errorf("%w: assignment index %d out of range", ErrInvalidInput, idx)
	}

	original := sched.Assignments[idx]
	sched.Assignments[idx].TimeSlot = req.TimeSlot
	if req.RoomID != "" {
		sched.Assignments[idx].RoomID = req.RoomID
	}
	if req.Parity != "" {
		sched.Assignments[idx].Parity = req.Parity
	}

	// Проверяем HC1–HC3 для изменённого назначения
	modified := sched.Assignments[idx]
	for j, a := range sched.Assignments {
		if j == idx {
			continue
		}
		if conflict := checkConflict(modified, a, j); conflict != nil {
			// откат
			sched.Assignments[idx] = original
			return nil, conflict
		}
	}

	// Пересчитываем score
	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}
	sched.Score = solver.CalculateFitness(sched.Assignments, *data)

	if err := s.outputRepo.UpdateSchedule(ctx, sched); err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	return sched, nil
}

// ImportExcel парсит xlsx-файл и сохраняет данные в БД.
func (s *Service) ImportExcel(ctx context.Context, data *domain.ImportedData) (*domain.ImportResult, error) {
	return s.importRepo.UpsertAll(ctx, data)
}

// checkConflict проверяет HC1-HC3 между двумя назначениями.
func checkConflict(a, b domain.Assignment, bIdx int) *ConflictError {
	if !slotsConflict(a, b) {
		return nil
	}
	// HC1: преподаватель
	if a.TeacherID == b.TeacherID {
		return &ConflictError{Type: "teacher_busy", ResourceID: a.TeacherID, ConflictWith: bIdx}
	}
	// HC2: группы
	for _, ga := range a.GroupIDs {
		for _, gb := range b.GroupIDs {
			if ga == gb {
				return &ConflictError{Type: "group_busy", ResourceID: ga, ConflictWith: bIdx}
			}
		}
	}
	// HC3: аудитория
	if a.RoomID == b.RoomID {
		return &ConflictError{Type: "room_busy", ResourceID: a.RoomID, ConflictWith: bIdx}
	}
	return nil
}

// slotsConflict возвращает true, если два назначения конфликтуют по слоту и чётности.
func slotsConflict(a, b domain.Assignment) bool {
	if a.TimeSlot != b.TimeSlot {
		return false
	}
	pa := a.Parity
	if pa == "" {
		pa = domain.Always
	}
	pb := b.Parity
	if pb == "" {
		pb = domain.Always
	}
	if pa == domain.Always || pb == domain.Always {
		return true
	}
	return pa == pb
}
