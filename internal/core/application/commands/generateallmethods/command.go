package generateallmethods

import "github.com/KolManis/uni-scheduler/internal/core/application/generation"

// Command — составить расписание всеми методами улучшения для сравнения.
// ImproveAlgo и ParallelStarts игнорируются.
type Command struct {
	generation.Request
}

// NewCommand проверяет параметры и подставляет значения по умолчанию.
func NewCommand(req generation.Request) (Command, error) {
	req, err := req.Validate()
	if err != nil {
		return Command{}, err
	}
	return Command{Request: req}, nil
}
