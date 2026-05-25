package generator

import (
	"context"
	"errors"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/KolManis/uni-scheduler/internal/solver"
)

var ErrInvalidInput = errors.New("invalid input")

type GenerateInput struct {
	Name          string
	MaxIterations int
	SolverType    string // "subject" или "teacher" (по умолчанию "teacher")
}

type Service struct {
	inputRepo  InputRepository
	outputRepo OutputRepository
}

func NewService(inputRepo InputRepository, outputRepo OutputRepository) *Service {
	return &Service{
		inputRepo:  inputRepo,
		outputRepo: outputRepo,
	}
}

func (s *Service) Generate(ctx context.Context, input GenerateInput) (*schedule.Schedule, error) {
	if input.Name == "" {
		input.Name = "Untitled"
	}
	if input.MaxIterations <= 0 {
		input.MaxIterations = 50000
	}

	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}

	var result *schedule.Schedule

	// По умолчанию используем teacher-driven подход
	switch input.SolverType {
	case "subject":
		workers := 4
		result, err = solver.SolveParallel(*data, input.MaxIterations, workers)
	default:
		// teacher-driven (по умолчанию)
		result, err = solver.SolveTeacher(*data, input.MaxIterations)
	}

	if err != nil {
		return nil, err
	}

	result.Name = input.Name

	saved, err := s.outputRepo.SaveSchedule(ctx, result)
	if err != nil {
		return nil, fmt.Errorf("save schedule: %w", err)
	}

	return saved, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*schedule.Schedule, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	return s.outputRepo.GetSchedule(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]schedule.Schedule, error) {
	return s.outputRepo.ListSchedules(ctx)
}
