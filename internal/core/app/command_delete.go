package app

import (
	"context"
	"fmt"
)

// DeleteSchedule удаляет расписание.
func (s *Service) DeleteSchedule(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	return s.outputRepo.DeleteSchedule(ctx, id)
}
