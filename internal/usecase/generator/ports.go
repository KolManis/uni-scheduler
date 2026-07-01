package generator

import (
	"context"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/KolManis/uni-scheduler/internal/importer"
	pgRepo "github.com/KolManis/uni-scheduler/internal/repository/postgres"
)

// InputRepository загружает исходные данные для генерации из БД.
type InputRepository interface {
	LoadInput(ctx context.Context) (*schedule.InputData, error)
}

// OutputRepository управляет готовыми расписаниями.
type OutputRepository interface {
	SaveSchedule(ctx context.Context, s *schedule.Schedule) (*schedule.Schedule, error)
	GetSchedule(ctx context.Context, id int64) (*schedule.Schedule, error)
	ListSchedules(ctx context.Context) ([]pgRepo.ScheduleSummary, error)
	DeleteSchedule(ctx context.Context, id int64) error
	UpdateSchedule(ctx context.Context, s *schedule.Schedule) error
}

// ImportRepository выполняет UPSERT данных, извлечённых из Excel.
type ImportRepository interface {
	UpsertAll(ctx context.Context, data *importer.ImportedData) (*importer.ImportResult, error)
}
