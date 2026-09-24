package app

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// PinAssignmentCommand — закрепить пару (Pinned = true) или снять закрепление.
// Закреплённая пара не двигается при перегенерации (GenerateCommand.BaseScheduleID).
type PinAssignmentCommand struct {
	ScheduleID int64
	Index      int
	Pinned     bool
}

func (s *Service) PinAssignment(ctx context.Context, cmd PinAssignmentCommand) (*domain.Schedule, error) {
	sched, err := s.loadAssignment(ctx, cmd.ScheduleID, cmd.Index)
	if err != nil {
		return nil, err
	}
	sched.Assignments[cmd.Index].Pinned = cmd.Pinned
	if err := s.outputRepo.UpdateSchedule(ctx, sched); err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	return sched, nil
}
