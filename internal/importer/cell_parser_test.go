package importer

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func TestParseCell_Lab(t *testing.T) {
	raw := "(лаб) Дискретная математика и математическая логика гр.РИС -23-2б 226 к.А (ЭТФ)"
	entries := ParseCell(raw)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ClassType != schedule.Lab {
		t.Errorf("expected Lab, got %s", e.ClassType)
	}
	if e.Parity != schedule.Always {
		t.Errorf("expected Always parity, got %s", e.Parity)
	}
	if len(e.GroupNames) != 1 || e.GroupNames[0] != "РИС -23-2б" {
		t.Errorf("unexpected groups: %v", e.GroupNames)
	}
	if e.RoomNumber != "226" {
		t.Errorf("expected room 226, got %s", e.RoomNumber)
	}
	if e.BuildingID != "А" {
		t.Errorf("expected building А, got %s", e.BuildingID)
	}
}

func TestParseCell_OddWeek(t *testing.T) {
	raw := "1н (лаб) Объектно-ориентированное программирование гр.РИС -23-1б 128 к.А (ЭТФ)"
	entries := ParseCell(raw)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Parity != schedule.Odd {
		t.Errorf("expected Odd, got %s", entries[0].Parity)
	}
}

func TestParseCell_MultipleGroups(t *testing.T) {
	raw := "(лек) Сети и телекоммуникации гр.АСУ -22-1б, ИКС -22-1б, РИС -22-1б 402 к.А (ЭТФ)"
	entries := ParseCell(raw)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ClassType != schedule.Lecture {
		t.Errorf("expected Lecture, got %s", entries[0].ClassType)
	}
	if len(entries[0].GroupNames) != 3 {
		t.Errorf("expected 3 groups, got %d: %v", len(entries[0].GroupNames), entries[0].GroupNames)
	}
}

func TestParseCell_TwoLines(t *testing.T) {
	raw := "1н (лаб) Предмет А гр.РИС -23-1б 128 к.А (ЭТФ)\n2н (лаб) Предмет А гр.РИС -23-2б 128 к.А (ЭТФ)"
	entries := ParseCell(raw)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Parity != schedule.Odd {
		t.Errorf("first entry: expected Odd, got %s", entries[0].Parity)
	}
	if entries[1].Parity != schedule.Even {
		t.Errorf("second entry: expected Even, got %s", entries[1].Parity)
	}
}

func TestParseCell_Empty(t *testing.T) {
	entries := ParseCell("")
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for empty cell, got %d", len(entries))
	}
}
