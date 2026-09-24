package replayschedule

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Command — повторить запуск, которым получено расписание: те же данные, алгоритмы,
// сиды и число раундов улучшения (ADR-0023).
type Command struct {
	ScheduleID int64
}

func NewCommand(scheduleID int64) (Command, error) {
	if scheduleID <= 0 {
		return Command{}, fmt.Errorf("%w: id must be positive", domain.ErrInvalidInput)
	}
	return Command{ScheduleID: scheduleID}, nil
}
