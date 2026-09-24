package web

import (
	"context"

	"github.com/KolManis/uni-scheduler/internal/core/application/queries/getschedule"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// schedule — запрос getschedule: расписание по id (страницы вызывают его чаще всего).
func (h *Handler) schedule(ctx context.Context, id int64) (*domain.Schedule, error) {
	q, err := getschedule.NewQuery(id)
	if err != nil {
		return nil, err
	}
	return h.uc.GetSchedule.Handle(ctx, q)
}
