package generator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/KolManis/uni-scheduler/internal/importer"
	pgRepo "github.com/KolManis/uni-scheduler/internal/repository/postgres"
	"github.com/KolManis/uni-scheduler/internal/solver"
)

var ErrInvalidInput = errors.New("invalid input")

// GenerateInput — параметры запроса на генерацию расписания.
type GenerateInput struct {
	Name          string
	MaxIterations int
	SolverType    string                 // "subject" | "teacher" (default)
	TimeoutSec    int                    // 0 → используется дефолт 30 сек
	SemesterHalf  schedule.SemesterHalf  // "" | "full" | "first" | "second"
	// "first"  → все планы (1-я половина семестра, лекции ещё идут)
	// "second" → исключить планы с semester_half="first" (2-я половина, лекции закончились)
	// "" / "full" → всё без фильтрации (по умолчанию)
}

// PatchRequest — запрос на изменение одного назначения.
type PatchRequest struct {
	TimeSlot schedule.TimeSlot
	RoomID   string
	Parity   schedule.Parity
}

// ConflictError описывает нарушение жёсткого ограничения при PATCH.
type ConflictError struct {
	Type         string `json:"type"`          // "teacher_busy" | "group_busy" | "room_busy"
	ResourceID   string `json:"resource_id"`
	ConflictWith int    `json:"conflict_with"` // индекс конфликтующего assignment
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict: %s resource=%s with_idx=%d", e.Type, e.ResourceID, e.ConflictWith)
}

// Service реализует бизнес-логику работы с расписаниями.
type Service struct {
	inputRepo  InputRepository
	outputRepo OutputRepository
	importRepo ImportRepository
}

func NewService(inputRepo InputRepository, outputRepo OutputRepository, importRepo ImportRepository) *Service {
	return &Service{
		inputRepo:  inputRepo,
		outputRepo: outputRepo,
		importRepo: importRepo,
	}
}

// Generate запускает генерацию расписания.
func (s *Service) Generate(ctx context.Context, in GenerateInput) (*schedule.Schedule, error) {
	if in.Name == "" {
		in.Name = "Untitled"
	}
	if in.MaxIterations <= 0 {
		in.MaxIterations = 50000
	}
	if in.TimeoutSec <= 0 {
		in.TimeoutSec = 30
	}

	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}

	// Фильтрация планов по половине семестра
	if in.SemesterHalf == schedule.HalfSecond {
		// 2-я половина: убираем планы, которые идут только в 1-й половине
		filtered := data.SubjectPlans[:0]
		for _, sp := range data.SubjectPlans {
			if sp.SemesterHalf != schedule.HalfFirst {
				filtered = append(filtered, sp)
			}
		}
		data.SubjectPlans = filtered
	}

	solveCtx, cancel := context.WithTimeout(ctx, time.Duration(in.TimeoutSec)*time.Second)
	defer cancel()

	type solveResult struct {
		sched *schedule.Schedule
		err   error
	}
	ch := make(chan solveResult, 1)

	go func() {
		var result *schedule.Schedule
		var solveErr error

		switch in.SolverType {
		case "subject":
			result, solveErr = solver.SolveParallel(*data, in.MaxIterations, 4)
		default:
			result, solveErr = solver.SolveTeacher(*data, in.MaxIterations)
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
		saved, err := s.outputRepo.SaveSchedule(ctx, res.sched)
		if err != nil {
			return nil, fmt.Errorf("save schedule: %w", err)
		}
		return saved, nil
	}
}

// GetByID возвращает расписание по ID.
func (s *Service) GetByID(ctx context.Context, id int64) (*schedule.Schedule, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	return s.outputRepo.GetSchedule(ctx, id)
}

// List возвращает краткий список расписаний (без assignments).
func (s *Service) List(ctx context.Context) ([]pgRepo.ScheduleSummary, error) {
	return s.outputRepo.ListSchedules(ctx)
}

// Delete удаляет расписание по ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	return s.outputRepo.DeleteSchedule(ctx, id)
}

// PatchAssignment изменяет одно назначение и проверяет HC1–HC3.
func (s *Service) PatchAssignment(ctx context.Context, schedID int64, idx int, req PatchRequest) (*schedule.Schedule, error) {
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
func (s *Service) ImportExcel(ctx context.Context, data *importer.ImportedData) (*importer.ImportResult, error) {
	return s.importRepo.UpsertAll(ctx, data)
}

// checkConflict проверяет HC1-HC3 между двумя назначениями.
func checkConflict(a, b schedule.Assignment, bIdx int) *ConflictError {
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
func slotsConflict(a, b schedule.Assignment) bool {
	if a.TimeSlot != b.TimeSlot {
		return false
	}
	pa := a.Parity
	if pa == "" {
		pa = schedule.Always
	}
	pb := b.Parity
	if pb == "" {
		pb = schedule.Always
	}
	if pa == schedule.Always || pb == schedule.Always {
		return true
	}
	return pa == pb
}
