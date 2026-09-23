package rest

import "github.com/KolManis/uni-scheduler/internal/core/domain"

// GenerateRequest — тело POST /schedules/generate.
type GenerateRequest struct {
	Name          string              `json:"name"`
	MaxIterations int                 `json:"max_iterations"`
	SolverType    string              `json:"solver_type"` // "" или "teacher" (старый "subject" удалён)
	TimeoutSec    int                 `json:"timeout_sec"`
	SemesterHalf  domain.SemesterHalf `json:"semester_half"` // "" | "first" | "second"
	ImproveAlgo    string              `json:"improve_algo"`    // "hillclimb" | "sa" | "tabu" | "ga" | "lns"
	ParallelStarts int                 `json:"parallel_starts"` // 0/1 — один запуск, N>1 — многостартовый
	LectureBeforePractice bool         `json:"lecture_before_practice"`
	SameSubjectSameDay    bool         `json:"same_subject_same_day"`
}

// PatchAssignmentRequest — тело PATCH /schedules/{id}/assignments/{idx}.
type PatchAssignmentRequest struct {
	TimeSlot domain.TimeSlot `json:"time_slot"`
	RoomID   string          `json:"room_id"`
	Parity   string          `json:"parity"`
}
