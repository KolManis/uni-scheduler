package importexcel

import (
	"context"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
)

// Handler выполняет сценарий «загрузить справочники из Excel».
type Handler struct {
	imports ports.ImportRepository
}

func NewHandler(imports ports.ImportRepository) *Handler {
	return &Handler{imports: imports}
}

// Handle добавляет новые записи и обновляет существующие.
func (h *Handler) Handle(ctx context.Context, cmd Command) (*domain.ImportResult, error) {
	return h.imports.UpsertAll(ctx, cmd.Data)
}
