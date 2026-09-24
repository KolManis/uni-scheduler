package pinassignment

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Command — закрепить пару (Pinned = true) или снять закрепление. Закреплённая пара
// не двигается при перегенерации (generation.Request.BaseScheduleID).
type Command struct {
	ScheduleID int64
	Index      int
	Pinned     bool
}

func NewCommand(scheduleID int64, index int, pinned bool) (Command, error) {
	if scheduleID <= 0 || index < 0 {
		return Command{}, fmt.Errorf("%w: schedule id and assignment index must be positive", domain.ErrInvalidInput)
	}
	return Command{ScheduleID: scheduleID, Index: index, Pinned: pinned}, nil
}
