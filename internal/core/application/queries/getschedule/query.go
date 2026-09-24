package getschedule

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Query — получить расписание целиком.
type Query struct {
	ScheduleID int64
}

func NewQuery(scheduleID int64) (Query, error) {
	if scheduleID <= 0 {
		return Query{}, fmt.Errorf("%w: id must be positive", domain.ErrInvalidInput)
	}
	return Query{ScheduleID: scheduleID}, nil
}
