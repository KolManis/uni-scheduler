package rest

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Лист расписания в Excel: строки — дни и пары, колонки — преподаватели или группы.
// Если в клетке по чётным и нечётным неделям разные пары, слот занимает две строки:
// чётная сверху, нечётная снизу. Клетки колонок, где пара одна на обе недели,
// объединяются по этим двум строкам.

var (
	pairTimes = [...]string{"", "8:00", "9:40", "11:30", "13:20", "15:00", "16:40"}
	dayNames  = map[domain.Day]string{
		domain.Monday: "понедельник", domain.Tuesday: "вторник", domain.Wednesday: "среда",
		domain.Thursday: "четверг", domain.Friday: "пятница", domain.Saturday: "суббота",
	}
	classTypeShort = map[domain.ClassType]string{domain.Lecture: "лек", domain.Practice: "пр", domain.Lab: "лаб"}
)

// sheetLayout — ширина колонок с парами: узкая, когда колонок много, и широкая для одной.
type sheetLayout struct{ columnWidth float64 }

var (
	wideSheet   = sheetLayout{columnWidth: 28}
	singleSheet = sheetLayout{columnWidth: 55}
)

// excelNames — подписи для клеток: названия предметов, преподавателей, групп, аудиторий.
type excelNames struct {
	subjects, teachers, groups, rooms map[string]string
}

func newExcelNames(input domain.InputData) excelNames {
	n := excelNames{subjects: map[string]string{}, teachers: map[string]string{}, groups: map[string]string{}, rooms: map[string]string{}}
	for _, s := range input.SubjectPlans {
		n.subjects[s.ID] = s.Name
	}
	for _, t := range input.Teachers {
		n.teachers[t.ID] = t.Name
	}
	for _, g := range input.Groups {
		n.groups[g.ID] = g.Name
	}
	buildings := map[string]string{}
	for _, b := range input.Buildings {
		buildings[b.ID] = b.Name
	}
	// «номер, корпус»: по одному номеру аудитории не понять, в каком она корпусе.
	for _, r := range input.Rooms {
		n.rooms[r.ID] = r.Number
		if b := buildings[r.BuildingID]; b != "" {
			n.rooms[r.ID] = r.Number + ", " + b
		}
	}
	return n
}

// weekPair — пары одной клетки по неделям; одна и та же пара в обеих — идёт каждую неделю.
type weekPair struct{ even, odd *domain.Assignment }

func (p weekPair) everyWeek() bool { return p.even != nil && p.even == p.odd }

// timetableColumn — колонка листа: заголовок и пары по слотам.
type timetableColumn struct {
	title string
	slots map[domain.TimeSlot]weekPair
}

// columnOf — колонка из пар, для которых belongs возвращает true.
func columnOf(title string, assignments []domain.Assignment, belongs func(domain.Assignment) bool) timetableColumn {
	col := timetableColumn{title: title, slots: map[domain.TimeSlot]weekPair{}}
	for i := range assignments {
		a := &assignments[i]
		if !belongs(*a) {
			continue
		}
		p := col.slots[a.TimeSlot]
		switch a.Parity {
		case domain.Even:
			p.even = a
		case domain.Odd:
			p.odd = a
		default:
			p.even, p.odd = a, a
		}
		col.slots[a.TimeSlot] = p
	}
	return col
}

// rowsFor — сколько строк нужно клетке: 2, если по неделям идут разные пары или пара
// только в одну неделю; иначе 1.
func rowsFor(p weekPair) int {
	if (p.even != nil || p.odd != nil) && !p.everyWeek() {
		return 2
	}
	return 1
}

// writeTimetable рисует лист sheet: колонки «День», «Время» и по колонке на cols.
func writeTimetable(f *excelize.File, sheet string, cols []timetableColumn, layout sheetLayout, names excelNames) {
	if f.GetSheetName(0) == "Sheet1" {
		f.SetSheetName("Sheet1", sheet)
	} else {
		f.NewSheet(sheet)
	}
	styles := newExcelStyles(f)

	f.SetCellValue(sheet, "A1", "День")
	f.SetCellValue(sheet, "B1", "Время")
	for c, col := range cols {
		f.SetCellValue(sheet, cellName(c+3, 1), col.title)
	}

	row := 2
	for dayIdx, day := range domain.AllDays {
		style := styles.plain
		if dayIdx%2 == 0 {
			style = styles.green // дни чередуются цветом, чтобы глазу было за что держаться
		}
		for num := domain.FirstPair; num <= domain.LastPair; num++ {
			slot := domain.MustNewTimeSlot(day, num)
			rows := 1
			for _, col := range cols {
				rows = max(rows, rowsFor(col.slots[slot]))
			}

			for c, col := range cols {
				writeSlot(f, sheet, c+3, row, rows, col.slots[slot], style, styles, names)
			}
			f.SetCellValue(sheet, cellName(1, row), dayNames[day])
			f.SetCellValue(sheet, cellName(2, row), pairTimes[num])
			f.SetCellStyle(sheet, cellName(1, row), cellName(2, row+rows-1), style)
			if rows == 2 {
				f.MergeCell(sheet, cellName(1, row), cellName(1, row+1))
				f.MergeCell(sheet, cellName(2, row), cellName(2, row+1))
			}
			for r := row; r < row+rows; r++ {
				f.SetRowHeight(sheet, r, 60)
			}
			row += rows
		}
	}

	lastCol, _ := excelize.ColumnNumberToName(len(cols) + 2)
	f.SetColWidth(sheet, "A", "A", 16)
	f.SetColWidth(sheet, "B", "B", 8)
	f.SetColWidth(sheet, "C", lastCol, layout.columnWidth)
}

// writeSlot заполняет клетку колонки col в строке row. rows — сколько строк у слота на
// листе (1 или 2): если клетке нужна одна, а слоту две, строки объединяются.
func writeSlot(f *excelize.File, sheet string, col, row, rows int, p weekPair, dayStyle int, styles excelStyles, names excelNames) {
	if rows == 1 || p.everyWeek() || (p.even == nil && p.odd == nil) {
		style := writeCell(f, sheet, cellName(col, row), p.even, "", dayStyle, styles, names)
		if rows == 2 {
			// Рамку объединённой клетки Excel рисует по обеим клеткам: без стиля у нижней
			// пропадают её нижняя и боковые границы.
			f.SetCellStyle(sheet, cellName(col, row+1), cellName(col, row+1), style)
			f.MergeCell(sheet, cellName(col, row), cellName(col, row+1))
		}
		return
	}
	writeCell(f, sheet, cellName(col, row), p.even, "(чётная неделя)", dayStyle, styles, names)
	writeCell(f, sheet, cellName(col, row+1), p.odd, "(нечётная неделя)", dayStyle, styles, names)
}

// writeCell — текст одной клетки: «предмет (вид)», преподаватель, аудитория и, если пара
// не каждую неделю, пометка недели. Пара на другом факультете — серым.
// Возвращает стиль, которым покрашена клетка.
func writeCell(f *excelize.File, sheet, cell string, a *domain.Assignment, week string, dayStyle int, styles excelStyles, names excelNames) int {
	style := dayStyle
	if a != nil && a.Type == externalPairType {
		style = styles.external
	}
	f.SetCellStyle(sheet, cell, cell, style)
	if a == nil {
		return style
	}
	var text string
	if a.Type == externalPairType {
		text = "Другой факультет"
		if a.SubjectID != "" {
			text += "\n" + a.SubjectID // пометка пары
		}
	} else {
		text = fmt.Sprintf("%s (%s)\n%s\nауд.%s",
			orID(names.subjects, a.SubjectID), orID(classTypeShort, a.Type), names.teachers[a.TeacherID], orID(names.rooms, a.RoomID))
	}
	if week != "" {
		text += "\n" + week
	}
	f.SetCellValue(sheet, cell, text)
	return style
}

// orID — подпись из справочника, а если её нет — сам идентификатор.
func orID[K ~string](labels map[K]string, id K) string {
	if s := labels[id]; s != "" {
		return s
	}
	return string(id)
}

func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

// excelStyles — стили клеток: обычный день, «зелёный» день и пара на другом факультете.
type excelStyles struct{ plain, green, external int }

func newExcelStyles(f *excelize.File) excelStyles {
	border := []excelize.Border{
		{Type: "left", Color: "000000", Style: 1},
		{Type: "right", Color: "000000", Style: 1},
		{Type: "top", Color: "000000", Style: 1},
		{Type: "bottom", Color: "000000", Style: 1},
	}
	align := &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"}
	fill := func(color string) excelize.Fill {
		return excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{color}}
	}
	plain, _ := f.NewStyle(&excelize.Style{Alignment: align, Border: border})
	green, _ := f.NewStyle(&excelize.Style{Alignment: align, Border: border, Fill: fill("E2EFDA")})
	external, _ := f.NewStyle(&excelize.Style{Alignment: align, Border: border, Fill: fill("EDEDED"),
		Font: &excelize.Font{Italic: true, Color: "595959"}})
	return excelStyles{plain: plain, green: green, external: external}
}
