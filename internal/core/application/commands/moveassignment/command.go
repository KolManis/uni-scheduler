package moveassignment

import (
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Command — перенести пару вручную: новое время, аудитория, чётность.
// Пустые RoomID и Parity — оставить как было.
type Command struct {
	ScheduleID int64
	Index      int // номер пары в расписании
	TimeSlot   domain.TimeSlot
	RoomID     string
	Parity     domain.Parity
}

// NewCommand проверяет номер расписания и пары и чётность.
func NewCommand(scheduleID int64, index int, slot domain.TimeSlot, roomID string, parity domain.Parity) (Command, error) {
	if scheduleID <= 0 || index < 0 {
		return Command{}, fmt.Errorf("%w: schedule id and assignment index must be positive", domain.ErrInvalidInput)
	}
	if parity != "" && !parity.IsValid() {
		return Command{}, fmt.Errorf("%w: parity %q", domain.ErrInvalidInput, parity)
	}
	return Command{ScheduleID: scheduleID, Index: index, TimeSlot: slot, RoomID: roomID, Parity: parity}, nil
}
