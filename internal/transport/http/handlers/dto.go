package handlers

import (
	"time"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

type GenerateRequest struct {
	Name          string `json:"name"`
	MaxIterations int    `json:"max_iterations"`
	SolverType    string `json:"solver_type"`
}

type ScheduleResponse struct {
	ID          int64                 `json:"id"`
	Name        string                `json:"name"`
	Assignments []schedule.Assignment `json:"assignments"`
	Score       int                   `json:"score"`
	CreatedAt   time.Time             `json:"created_at"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func toScheduleResponse(s *schedule.Schedule) ScheduleResponse {
	return ScheduleResponse{
		ID:          s.ID,
		Name:        s.Name,
		Assignments: s.Assignments,
		Score:       s.Score,
		CreatedAt:   s.CreatedAt,
	}
}
