package handlers

import "github.com/KolManis/uni-scheduler/internal/domain/schedule"

// GenerateRequest — тело POST /schedules/generate.
type GenerateRequest struct {
	Name          string `json:"name"`
	MaxIterations int    `json:"max_iterations"`
	SolverType    string `json:"solver_type"` // "teacher" | "subject"
	TimeoutSec    int    `json:"timeout_sec"`
}

// PatchAssignmentRequest — тело PATCH /schedules/{id}/assignments/{idx}.
type PatchAssignmentRequest struct {
	TimeSlot schedule.TimeSlot `json:"time_slot"`
	RoomID   string            `json:"room_id"`
	Parity   string            `json:"parity"`
}
