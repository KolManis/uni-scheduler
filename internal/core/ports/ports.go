package ports

import (
	"context"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// InputRepository загружает исходные данные для генерации из БД.
type InputRepository interface {
	LoadInput(ctx context.Context) (*domain.InputData, error)
}

// OutputRepository управляет готовыми расписаниями.
type OutputRepository interface {
	SaveSchedule(ctx context.Context, s *domain.Schedule) (*domain.Schedule, error)
	GetSchedule(ctx context.Context, id int64) (*domain.Schedule, error)
	ListSchedules(ctx context.Context) ([]domain.ScheduleSummary, error)
	DeleteSchedule(ctx context.Context, id int64) error
	UpdateSchedule(ctx context.Context, s *domain.Schedule) error
}

// ImportRepository выполняет UPSERT данных, извлечённых из Excel.
type ImportRepository interface {
	UpsertAll(ctx context.Context, data *domain.ImportedData) (*domain.ImportResult, error)
}
