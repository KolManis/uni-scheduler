package rest

import (
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Слот занимает две строки, если хоть у одной колонки по чётным и нечётным неделям разные
// пары. Клетки остальных колонок объединяются по этим строкам, и рамка у объединённой
// клетки должна быть целой: нижняя половина покрашена тем же стилем, что и верхняя.
func TestWriteTimetable_MergedCellKeepsBorders(t *testing.T) {
	monday1 := domain.MustNewTimeSlot(domain.Monday, 1)
	split := []domain.Assignment{
		{
			GroupIDs:  []string{"G-split"},
			TeacherID: "T1",
			SubjectID: "S1",
			Type:      domain.Practice,
			TimeSlot:  monday1,
			Parity:    domain.Even,
		},
		{
			GroupIDs:  []string{"G-split"},
			TeacherID: "T2",
			SubjectID: "S2",
			Type:      domain.Practice,
			TimeSlot:  monday1,
			Parity:    domain.Odd,
		},
	}

	tests := []struct {
		name   string
		column []domain.Assignment // пары второй колонки в понедельник на 1-й паре
	}{
		{
			name: "пустая клетка",
		},
		{
			name: "пара каждую неделю",
			column: []domain.Assignment{
				{
					GroupIDs:  []string{"G-other"},
					TeacherID: "T3",
					SubjectID: "S3",
					Type:      domain.Lecture,
					TimeSlot:  monday1,
					Parity:    domain.Always,
				},
			},
		},
		{
			name: "пара на другом факультете",
			column: []domain.Assignment{
				{
					GroupIDs:  []string{"G-other"},
					TeacherID: "T3",
					Type:      externalPairType,
					TimeSlot:  monday1,
					Parity:    domain.Always,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := excelize.NewFile()
			cols := []timetableColumn{
				columnOf("разные недели", split, byGroup("G-split")),
				columnOf("проверяемая", tt.column, byGroup("G-other")),
			}
			writeTimetable(f, "Лист", cols, wideSheet, excelNames{})

			// Понедельник, 1-я пара — строки 2 и 3; проверяемая колонка — D.
			top, err := f.GetCellStyle("Лист", "D2")
			if err != nil {
				t.Fatal(err)
			}
			bottom, err := f.GetCellStyle("Лист", "D3")
			if err != nil {
				t.Fatal(err)
			}
			if bottom != top {
				t.Fatalf("стиль нижней половины %d, верхней %d: у объединённой клетки пропадёт рамка", bottom, top)
			}
			style, err := f.GetStyle(bottom)
			if err != nil {
				t.Fatal(err)
			}
			if len(style.Border) != 4 {
				t.Errorf("у нижней половины %d границ из 4", len(style.Border))
			}
		})
	}
}
