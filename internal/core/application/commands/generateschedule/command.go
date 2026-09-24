package generateschedule

import "github.com/KolManis/uni-scheduler/internal/core/application/generation"

// Command — составить одно расписание выбранным методом.
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
