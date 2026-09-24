package deleteschedule

import (
	"context"

	"github.com/KolManis/uni-scheduler/internal/core/ports"
)

// Handler выполняет сценарий «удалить расписание».
type Handler struct {
	output ports.OutputRepository
}

func NewHandler(output ports.OutputRepository) *Handler {
	return &Handler{output: output}
}

func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	return h.output.DeleteSchedule(ctx, cmd.ScheduleID)
}
