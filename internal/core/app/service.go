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
	SolverType    string              // алгоритм построения: "" / "teacher" (по преподавателям) | "dsatur"
	TimeoutSec    int                 // 0 → используется дефолт 30 сек
	SemesterHalf  domain.SemesterHalf // "" | "full" | "first" | "second"
	// "first"  → все планы (1-я половина семестра, лекции ещё идут)
	// "second" → исключить планы с semester_half="first" (2-я половина, лекции закончились)
	// "" / "full" → всё без фильтрации (по умолчанию)
	ImproveAlgo    string                   // "hillclimb" (default) | "sa" | "tabu" | "ga" | "lns"
	ParallelStarts int                      // 0/1 — один запуск (по умолчанию), N>1 — многостартовый параллельный поиск
	Preferences    domain.SolverPreferences // необязательные правила, по умолчанию выключены
	// BaseScheduleID — перегенерация: закреплённые пары этого расписания остаются на
	// местах, остальное строится заново вокруг них. 0 — обычная генерация с нуля.
	BaseScheduleID int64
}

// PatchRequest — запрос на изменение одного назначения.
type PatchRequest struct {
	TimeSlot domain.TimeSlot
	RoomID   string
	Parity   domain.Parity
}

// ConflictTeacherUnavailable — слот отмечен преподавателем как недоступный (HC7).
// ConflictWith у такого конфликта равен -1: он не с другим занятием.
const ConflictTeacherUnavailable = "teacher_unavailable"

// ConflictTeacherExternalPair — в это время у преподавателя пара на другом факультете в ту же
// неделю. Detail — пометка этой пары (факультет, аудитория), Parity — её неделя.
const ConflictTeacherExternalPair = "teacher_external_pair"

// ConflictError описывает нарушение жёсткого ограничения при PATCH.
type ConflictError struct {
	Type         string        `json:"type"` // "teacher_busy" | "group_busy" | "room_busy" | "teacher_unavailable" | "teacher_external_pair"
	ResourceID   string        `json:"resource_id"`
	ConflictWith int           `json:"conflict_with"`    // индекс конфликтующего assignment
	Detail       string        `json:"detail,omitempty"` // пометка внешней пары
	Parity       domain.Parity `json:"parity,omitempty"` // неделя внешней пары
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
	if err := validateSolverType(in.SolverType); err != nil {
		return nil, err
	}
	construct, _ := solver.ParseConstruction(in.SolverType)
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
	fixed, err := s.pinnedOf(ctx, in.BaseScheduleID)
	if err != nil {
		return nil, err
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

	// Бюджет солвера — общий таймаут минус 15 секунд буфера на сохранение в БД
	// и накладные расходы. Раньше внутри солвера был захардкожен 60 сек, из-за чего
	// при timeout_sec=400 солвер завершался за минуту и не использовал оставшиеся 340.
	solverBudget := time.Duration(in.TimeoutSec-15) * time.Second
	if solverBudget < 30*time.Second {
		solverBudget = 30 * time.Second
	}

	// Солвер и сохранение работают с context.Background() — не зависят от того,
	// открыта ли ещё вкладка пользователя. Если пользователь ушёл со страницы,
	// запрос отменяется, но генерация продолжается на сервере, расписание всё равно
	// оказывается в БД и появится в списке при следующем открытии.
	data.Preferences = in.Preferences
	dataForSolver := *data
	go func() {
		var result *domain.Schedule
		var solveErr error

		improve := solver.ImproveAlgorithm(in.ImproveAlgo)
		result, solveErr = solver.SolveMultiStartFixed(dataForSolver, construct, in.MaxIterations, improve, in.ParallelStarts, solverBudget, fixed)
		if solveErr != nil {
			ch <- solveResult{err: solveErr}
			return
		}
		result.Name = in.Name
		result.Options = in.Preferences
		result.Options.Construction = string(construct)
		result.Unplaced = solver.ComputeUnplaced(result.Assignments, dataForSolver)
		saved, err := s.outputRepo.SaveSchedule(context.Background(), result)
		if err != nil {
			ch <- solveResult{err: fmt.Errorf("save schedule: %w", err)}
			return
		}
		ch <- solveResult{sched: saved}
	}()

	select {
	case <-solveCtx.Done():
		return nil, fmt.Errorf("solver timeout after %ds", in.TimeoutSec)
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		return res.sched, nil
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
	if err := validateSolverType(in.SolverType); err != nil {
		return nil, err
	}
	construct, _ := solver.ParseConstruction(in.SolverType)
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
	fixed, err := s.pinnedOf(ctx, in.BaseScheduleID)
	if err != nil {
		return nil, err
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

	// Бюджет солвера — общий таймаут минус 15 секунд буфера. Каждой из 5 горутин
	// достаётся столько же wall-clock, чтобы под конкуренцией за CPU успеть converge
	// и метаэвристику. При budget по умолчанию (60 сек) и 5 параллельных горутинах на
	// 4-8 ядрах каждая едва успевала converge, и все методы возвращали одинаковый
	// score чистого построения.
	solverBudget := time.Duration(in.TimeoutSec-15) * time.Second
	if solverBudget < 30*time.Second {
		solverBudget = 30 * time.Second
	}

	// Каждая горутина сохраняет своё расписание сама, с context.Background(): даже
	// если пользователь ушёл со страницы и запрос отменён, генерация продолжается
	// и результаты попадают в БД — пользователь увидит их в списке позже.
	// При «все методы» игнорируем ParallelStarts: 5 методов уже дают 5 параллельных
	// горутин. Если умножать на 3-8 стартов внутри каждого, получаем 15-40 горутин,
	// конкурирующих за ~4-8 ядер CPU.
	data.Preferences = in.Preferences
	dataForSolver := *data
	baseName := in.Name
	constructLabel := ""
	if construct == solver.ConstructDSatur {
		constructLabel = "DSatur, "
	}
	for i, m := range methods {
		go func(idx int, algo solver.ImproveAlgorithm, suffix string) {
			sched, solveErr := solver.SolveWithFixed(dataForSolver, construct, in.MaxIterations, algo, 0, solverBudget, fixed)
			if solveErr != nil {
				results[idx] = genResult{err: solveErr}
				done <- idx
				return
			}
			sched.Name = baseName + " — " + constructLabel + suffix
			sched.Options = in.Preferences
			sched.Options.Construction = string(construct)
			sched.Unplaced = solver.ComputeUnplaced(sched.Assignments, dataForSolver)
			out, err := s.outputRepo.SaveSchedule(context.Background(), sched)
			if err != nil {
				results[idx] = genResult{err: fmt.Errorf("save %s: %w", suffix, err)}
				done <- idx
				return
			}
			results[idx] = genResult{sched: out}
			done <- idx
		}(i, m.algo, m.suffix)
	}

	// Ждём завершения всех горутин, пока клиент нас слушает. Если solveCtx отменён
	// (клиент ушёл или сработал общий таймаут), прекращаем ждать — но горутины
	// продолжают работать в фоне и сами дописывают результат в БД.
	completed := 0
	for completed < len(methods) {
		select {
		case <-solveCtx.Done():
			completed = len(methods)
		case <-done:
			completed++
		}
	}

	// То, что успело сойтись К МОМЕНТУ ВОЗВРАТА, — возвращаем клиенту как success.
	// Остальное успеет сохраниться в фоне.
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
			continue
		}
		saved = append(saved, r.sched)
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
// Входные данные нужны: оценка смотрит на типы аудиторий (спортзал и стадион
// в переходах между корпусами не штрафуются). С пустыми данными разбивка
// расходилась бы с сохранённым score.
func (s *Service) Breakdown(sched *domain.Schedule, input domain.InputData) domain.FitnessBreakdown {
	input.Preferences = sched.Options
	return solver.CalculateFitnessBreakdown(sched.Assignments, input)
}

// Quality — показатели расписания в штуках (окна, одиночные дни, суббота).
func (s *Service) Quality(sched *domain.Schedule) domain.WeekQuality {
	return solver.CalculateQuality(sched.Assignments)
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

	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}

	// HC1–HC3, HC7: преподаватель, группы и аудитория свободны, преподаватель доступен.
	modified := sched.Assignments[idx]
	for _, r := range data.Rooms {
		if r.ID == modified.RoomID {
			// Корпус пары — корпус её аудитории: от него зависят переходы между корпусами.
			modified.BuildingID = r.BuildingID
			sched.Assignments[idx].BuildingID = r.BuildingID
		}
	}
	if conflict := moveConflict(sched.Assignments, idx, modified, data.Teachers, false); conflict != nil {
		sched.Assignments[idx] = original
		return nil, conflict
	}

	// Пересчитываем score
	data.Preferences = sched.Options
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

// validateSolverType — остался один солвер (teacher-driven). Старый поиск с возвратом
// ("subject") удалён: на реальных данных он давал score ~918000 против ~13000.
func validateSolverType(solverType string) error {
	if _, ok := solver.ParseConstruction(solverType); ok {
		return nil
	}
	return fmt.Errorf("%w: solver_type %q не поддерживается, доступны \"teacher\" и \"dsatur\"", ErrInvalidInput, solverType)
}

// isTeacherUnavailable — слот входит в недоступные слоты преподавателя.
func isTeacherUnavailable(teachers []domain.Teacher, teacherID string, slot domain.TimeSlot) bool {
	for _, t := range teachers {
		if t.ID != teacherID {
			continue
		}
		for _, s := range t.UnavailableSlots {
			if s == slot {
				return true
			}
		}
		return false
	}
	return false
}

// externalPairAt — пара преподавателя на другом факультете в этом слоте, идущая хотя бы
// в одну неделю с парой чётности parity; nil, если такой нет.
func externalPairAt(teachers []domain.Teacher, teacherID string, slot domain.TimeSlot, parity domain.Parity) *domain.ExternalPair {
	for _, t := range teachers {
		if t.ID != teacherID {
			continue
		}
		for k, ep := range t.ExternalPairs {
			if ep.TimeSlot == slot && weeksOverlap(ep.Parity, parity) {
				return &t.ExternalPairs[k]
			}
		}
		return nil
	}
	return nil
}

// weeksOverlap — пары с чётностями a и b идут хотя бы в одну общую неделю.
func weeksOverlap(a, b domain.Parity) bool {
	return a == "" || b == "" || a == domain.Always || b == domain.Always || a == b
}
