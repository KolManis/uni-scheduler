package app

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// MoveAssignmentCommand — перенести пару вручную: новое время, аудитория, чётность.
// Пустые RoomID и Parity — оставить как было.
type MoveAssignmentCommand struct {
	ScheduleID int64
	Index      int // номер пары в расписании
	TimeSlot   domain.TimeSlot
	RoomID     string
	Parity     domain.Parity
}

// MoveAssignment переносит пару, если это не нарушает жёстких ограничений, и
// пересчитывает score. При конфликте расписание не меняется, ошибка — *ConflictError.
func (s *Service) MoveAssignment(ctx context.Context, cmd MoveAssignmentCommand) (*domain.Schedule, error) {
	sched, err := s.loadAssignment(ctx, cmd.ScheduleID, cmd.Index)
	if err != nil {
		return nil, err
	}
	data, err := s.inputRepo.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}

	moved := sched.Assignments[cmd.Index]
	moved.TimeSlot = cmd.TimeSlot
	if cmd.RoomID != "" {
		moved.RoomID = cmd.RoomID
		// Корпус пары — корпус её аудитории: от него зависят переходы между корпусами.
		moved.BuildingID = roomBuilding(data.Rooms, cmd.RoomID, moved.BuildingID)
	}
	if cmd.Parity != "" {
		moved.Parity = cmd.Parity
	}

	if conflict := moveConflict(sched.Assignments, cmd.Index, moved, data.Teachers, false); conflict != nil {
		return nil, conflict
	}

	sched.Assignments[cmd.Index] = moved
	data.Preferences = sched.Options
	sched.Score = solver.CalculateFitness(sched.Assignments, *data)
	if err := s.outputRepo.UpdateSchedule(ctx, sched); err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	return sched, nil
}

// loadAssignment загружает расписание и проверяет, что пара с номером index в нём есть.
func (s *Service) loadAssignment(ctx context.Context, scheduleID int64, index int) (*domain.Schedule, error) {
	sched, err := s.outputRepo.GetSchedule(ctx, scheduleID)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(sched.Assignments) {
		return nil, fmt.Errorf("%w: assignment index %d out of range", ErrInvalidInput, index)
	}
	return sched, nil
}

// roomBuilding — корпус аудитории roomID; если аудитории нет в справочнике — fallback.
func roomBuilding(rooms []domain.Room, roomID, fallback string) string {
	for _, r := range rooms {
		if r.ID == roomID {
			return r.BuildingID
		}
	}
	return fallback
}
