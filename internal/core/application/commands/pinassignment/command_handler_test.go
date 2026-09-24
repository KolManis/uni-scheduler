package pinassignment

import (
	"context"
	"errors"
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/application/testfakes"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestHandle_PinsAndUnpins(t *testing.T) {
	output := &testfakes.OutputRepo{Schedule: domain.Schedule{ID: 1, Assignments: []domain.Assignment{{TeacherID: "T1"}}}}
	handler := NewHandler(output)

	for _, pinned := range []bool{true, false} {
		cmd, err := NewCommand(1, 0, pinned)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := handler.Handle(context.Background(), cmd); err != nil {
			t.Fatal(err)
		}
		if got := output.Schedule.Assignments[0].Pinned; got != pinned {
			t.Errorf("pinned = %v, ожидалось %v", got, pinned)
		}
	}
}

func TestHandle_IndexOutOfRange(t *testing.T) {
	output := &testfakes.OutputRepo{Schedule: domain.Schedule{ID: 1}}
	cmd, _ := NewCommand(1, 5, true)
	_, err := NewHandler(output).Handle(context.Background(), cmd)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("ожидалась ErrInvalidInput, получено %v", err)
	}
	if output.Updated {
		t.Error("расписание не должно сохраняться")
	}
}

func TestNewCommand_RejectsBadIDs(t *testing.T) {
	for _, tc := range []struct {
		id  int64
		idx int
	}{{0, 0}, {1, -1}} {
		if _, err := NewCommand(tc.id, tc.idx, true); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("NewCommand(%d, %d): ожидалась ErrInvalidInput, получено %v", tc.id, tc.idx, err)
		}
	}
}
