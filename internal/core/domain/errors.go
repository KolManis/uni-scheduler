package domain

import "errors"

var (
	ErrConflict   = errors.New("schedule conflict detected")
	ErrNoSolution = errors.New("no valid schedule found")
	ErrNotFound   = errors.New("schedule not found")
	// ErrInvalidInput — неверные параметры команды или запроса (адаптер отвечает 400).
	ErrInvalidInput = errors.New("invalid input")
)
