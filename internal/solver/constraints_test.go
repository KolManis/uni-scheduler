package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func TestIsSlotFree_EmptyOccupied(t *testing.T) {
	occupied := map[schedule.TimeSlot]map[string]schedule.Parity{}
	slot := schedule.TimeSlot{Day: schedule.Monday, PairNum: 1}
	if !isSlotFree(slot, "T1", schedule.Always, occupied) {
		t.Fatal("expected slot to be free")
	}
}

func TestIsSlotFree_AlwaysConflict(t *testing.T) {
	slot := schedule.TimeSlot{Day: schedule.Monday, PairNum: 1}
	occupied := map[schedule.TimeSlot]map[string]schedule.Parity{
		slot: {"T1": schedule.Always},
	}
	if isSlotFree(slot, "T1", schedule.Even, occupied) {
		t.Fatal("always should conflict with even")
	}
	if isSlotFree(slot, "T1", schedule.Always, occupied) {
		t.Fatal("always should conflict with always")
	}
}

func TestIsSlotFree_EvenOddNoConflict(t *testing.T) {
	slot := schedule.TimeSlot{Day: schedule.Monday, PairNum: 2}
	occupied := map[schedule.TimeSlot]map[string]schedule.Parity{
		slot: {"T1": schedule.Even},
	}
	if !isSlotFree(slot, "T1", schedule.Odd, occupied) {
		t.Fatal("even and odd should not conflict")
	}
}

func TestIsSlotFree_SameParityConflict(t *testing.T) {
	slot := schedule.TimeSlot{Day: schedule.Tuesday, PairNum: 3}
	occupied := map[schedule.TimeSlot]map[string]schedule.Parity{
		slot: {"G1": schedule.Odd},
	}
	if isSlotFree(slot, "G1", schedule.Odd, occupied) {
		t.Fatal("odd vs odd should conflict")
	}
}

func TestIsRoomBigEnough_ExactFit(t *testing.T) {
	room := schedule.Room{ID: "R1", Capacity: 30}
	groupMap := map[string]schedule.Group{
		"G1": {StudentCount: 15},
		"G2": {StudentCount: 15},
	}
	ok, _ := isRoomBigEnoughWithOverflow(room, []string{"G1", "G2"}, groupMap)
	if !ok {
		t.Fatal("30 students should fit in capacity 30")
	}
}

func TestIsRoomBigEnough_OverCapacity(t *testing.T) {
	room := schedule.Room{ID: "R1", Capacity: 20}
	groupMap := map[string]schedule.Group{
		"G1": {StudentCount: 25},
	}
	ok, _ := isRoomBigEnoughWithOverflow(room, []string{"G1"}, groupMap)
	if ok {
		t.Fatal("25 students should not fit in capacity 20")
	}
}

func TestIsRoomBigEnough_3GroupsOverflow(t *testing.T) {
	room := schedule.Room{ID: "R1", Capacity: 30}
	groupMap := map[string]schedule.Group{
		"G1": {StudentCount: 15},
		"G2": {StudentCount: 15},
		"G3": {StudentCount: 10},
	}
	// 40 > 30 но ≤ 45 (1.5×30), три группы → допустимо
	ok, _ := isRoomBigEnoughWithOverflow(room, []string{"G1", "G2", "G3"}, groupMap)
	if !ok {
		t.Fatal("3 groups with 40 students should fit in capacity 30 with 1.5x overflow")
	}
}
