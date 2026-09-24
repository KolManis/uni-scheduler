package moveassignment

import (
	"context"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/application/rules"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

// Handler выполняет сценарий «перенести пару вручную».
type Handler struct {
	input  ports.InputRepository
	output ports.OutputRepository
}

func NewHandler(input ports.InputRepository, output ports.OutputRepository) *Handler {
	return &Handler{input: input, output: output}
}

// Handle переносит пару, если это не нарушает жёстких ограничений, и пересчитывает score.
// При конфликте расписание не меняется, ошибка — *rules.ConflictError.
func (h *Handler) Handle(ctx context.Context, cmd Command) (*domain.Schedule, error) {
	sched, err := h.output.GetSchedule(ctx, cmd.ScheduleID)
	if err != nil {
		return nil, err
	}
	if cmd.Index >= len(sched.Assignments) {
		return nil, fmt.Errorf("%w: assignment index %d out of range", domain.ErrInvalidInput, cmd.Index)
	}
	data, err := h.input.LoadInput(ctx)
	if err != nil {
		return nil, fmt.Errorf("load input: %w", err)
	}

	moved := sched.Assignments[cmd.Index]
	moved.TimeSlot = cmd.TimeSlot
	// Новая аудитория должна подходить паре по типу, вместимости и корпусу (HC4–HC6) —
	// те же правила, что при генерации. Если аудитория не меняется, не проверяем: у старых
	// расписаний она могла быть выбрана вручную, и перенос времени не должен из-за этого
	// стать невозможным.
	if cmd.RoomID != "" && cmd.RoomID != moved.RoomID {
		if conflict := roomConflict(moved, cmd.RoomID, *data); conflict != nil {
			return nil, conflict
		}
	}
	if cmd.RoomID != "" {
		moved.RoomID = cmd.RoomID
		// Корпус пары — корпус её аудитории: от него зависят переходы между корпусами.
		moved.BuildingID = roomBuilding(data.Rooms, cmd.RoomID, moved.BuildingID)
	}
	if cmd.Parity != "" {
		moved.Parity = cmd.Parity
	}

	if conflict := rules.Check(sched.Assignments, cmd.Index, moved, data.Teachers, false); conflict != nil {
		return nil, conflict
	}

	sched.Assignments[cmd.Index] = moved
	data.Preferences = sched.Options
	sched.Score = solver.CalculateFitness(sched.Assignments, *data)
	if err := h.output.UpdateSchedule(ctx, sched); err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	return sched, nil
}

// roomConflict — конфликт, если аудитория roomID не подходит паре a; nil — подходит.
// Detail — чем именно не подходит, для сообщения пользователю.
func roomConflict(a domain.Assignment, roomID string, data domain.InputData) *rules.ConflictError {
	for _, r := range solver.SuitableRooms(a, data) {
		if r.ID == roomID {
			return nil
		}
	}
	return &rules.ConflictError{
		Type:         rules.ConflictRoomUnsuitable,
		ResourceID:   roomID,
		ConflictWith: -1,
		Detail:       roomMismatch(a, roomID, data),
	}
}

// roomMismatch — чем аудитория не подходит паре: тип, мест меньше, чем студентов, или корпус.
func roomMismatch(a domain.Assignment, roomID string, data domain.InputData) string {
	var room *domain.Room
	for i := range data.Rooms {
		if data.Rooms[i].ID == roomID {
			room = &data.Rooms[i]
		}
	}
	if room == nil {
		return "аудитории нет в справочнике"
	}
	for _, sp := range data.SubjectPlans {
		if sp.ID == a.SubjectID && sp.RequiresRoomType != "" && sp.RequiresRoomType != room.Type {
			return fmt.Sprintf("нужна аудитория типа %s, а это %s", sp.RequiresRoomType, room.Type)
		}
	}
	students := 0
	for _, g := range data.Groups {
		for _, gid := range a.GroupIDs {
			if g.ID == gid {
				students += g.StudentCount
			}
		}
	}
	if room.Capacity < students {
		return fmt.Sprintf("мест %d, а студентов в группах %d", room.Capacity, students)
	}
	return "корпус аудитории не подходит группам, преподавателю или предмету"
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
