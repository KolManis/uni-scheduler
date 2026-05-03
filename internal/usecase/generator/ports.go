package generator

import (
	"context"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

type InputRepository interface {
	LoadInput(ctx context.Context) (*schedule.InputData, error)
}

type OutputRepository interface {
	SaveSchedule(ctx context.Context, sched *schedule.Schedule) (*schedule.Schedule, error)
	GetSchedule(ctx context.Context, id int64) (*schedule.Schedule, error)
	ListSchedules(ctx context.Context) ([]schedule.Schedule, error)
}

type Usecase interface {
	Generate(ctx context.Context, input GenerateInput) (*schedule.Schedule, error)
	GetByID(ctx context.Context, id int64) (*schedule.Schedule, error)
	List(ctx context.Context) ([]schedule.Schedule, error)
}
