package importexcel

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Command — сохранить справочники, разобранные из Excel (разбор — во входящем адаптере).
type Command struct {
	Data *domain.ImportedData
}

func NewCommand(data *domain.ImportedData) (Command, error) {
	if data == nil {
		return Command{}, fmt.Errorf("%w: no imported data", domain.ErrInvalidInput)
	}
	return Command{Data: data}, nil
}
