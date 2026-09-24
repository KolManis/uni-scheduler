package suitablerooms

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Query — в какие аудитории можно поставить пару Index расписания ScheduleID.
type Query struct {
	ScheduleID int64
	Index      int
}

func NewQuery(scheduleID int64, index int) (Query, error) {
	if scheduleID <= 0 || index < 0 {
		return Query{}, fmt.Errorf("%w: schedule id and assignment index must be positive", domain.ErrInvalidInput)
	}
	return Query{ScheduleID: scheduleID, Index: index}, nil
}
