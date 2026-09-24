// Package testfakes — ручные заглушки портов хранилища для тестов сценариев.
package testfakes

import (
	"context"
	"errors"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// InputRepo отдаёт заданные справочники.
type InputRepo struct {
	Data domain.InputData
}

func (f *InputRepo) LoadInput(context.Context) (*domain.InputData, error) {
	data := f.Data
	return &data, nil
}

// OutputRepo хранит одно расписание и запоминает, было ли оно сохранено.
type OutputRepo struct {
	Schedule domain.Schedule
	Updated  bool
}

func (f *OutputRepo) SaveSchedule(context.Context, *domain.Schedule) (*domain.Schedule, error) {
	return nil, errors.New("not used")
}

// GetSchedule отдаёт копию, чтобы сценарий не менял расписание в заглушке в обход UpdateSchedule.
func (f *OutputRepo) GetSchedule(context.Context, int64) (*domain.Schedule, error) {
	s := f.Schedule
	s.Assignments = append([]domain.Assignment(nil), f.Schedule.Assignments...)
	return &s, nil
}

func (f *OutputRepo) ListSchedules(context.Context) ([]domain.ScheduleSummary, error) {
	return nil, nil
}

func (f *OutputRepo) DeleteSchedule(context.Context, int64) error { return nil }

func (f *OutputRepo) UpdateSchedule(_ context.Context, s *domain.Schedule) error {
	f.Schedule = *s
	f.Updated = true
	return nil
}
