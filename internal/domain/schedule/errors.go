package schedule

import "errors"

var (
	ErrConflict   = errors.New("schedule conflict detected")
	ErrNoSolution = errors.New("no valid schedule found")
	ErrNotFound   = errors.New("schedule not found")
)
