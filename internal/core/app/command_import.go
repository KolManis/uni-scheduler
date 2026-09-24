package app

import (
	"context"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// ImportExcel сохраняет справочники, разобранные из Excel-файла (разбор — во входящем адаптере).
func (s *Service) ImportExcel(ctx context.Context, data *domain.ImportedData) (*domain.ImportResult, error) {
	return s.importRepo.UpsertAll(ctx, data)
}
