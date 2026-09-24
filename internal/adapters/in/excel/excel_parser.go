package excel

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/xuri/excelize/v2"
)

var dayMap = map[string]domain.Day{
	"ПОНЕДЕЛЬНИК": domain.Monday,
	"ВТОРНИК":     domain.Tuesday,
	"СРЕДА":       domain.Wednesday,
	"ЧЕТВЕРГ":     domain.Thursday,
	"ПЯТНИЦА":     domain.Friday,
	"СУББОТА":     domain.Saturday,
}

var timeToSlot = map[string]int{
	"8:00":  1,
	"9:40":  2,
	"11:30": 3,
	"13:20": 4,
	"15:00": 5,
	"16:40": 6,
	"18:20": 7,
	"20:00": 8,
}

// teacherNameRe извлекает имя преподавателя из строки вида «Преподаватель Доц. Петренко А.А.»
var teacherNameRe = regexp.MustCompile(`(?i)преподаватель\s+(.+)`)

// ParseReader разбирает xlsx из []byte (для HTTP multipart upload).
func ParseReader(content []byte) (*domain.ImportedData, []string, error) {
	f, err := excelize.OpenReader(strings.NewReader(string(content)))
	if err != nil {
		return nil, nil, fmt.Errorf("open xlsx reader: %w", err)
	}
	defer f.Close()

	var data domain.ImportedData
	var warnings []string

	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("лист %q: %v", sheetName, err))
			continue
		}
		teacherName := extractTeacherName(rows)
		if teacherName == "" {
			teacherName = sheetName
		}
		lessons, w := parseSheet(rows, teacherName)
		data.Lessons = append(data.Lessons, lessons...)
		warnings = append(warnings, w...)
	}
	return &data, warnings, nil
}

func extractTeacherName(rows [][]string) string {
	for i := 0; i < 3 && i < len(rows); i++ {
		for _, cell := range rows[i] {
			cell = strings.TrimSpace(strings.ReplaceAll(cell, "\xa0", " "))
			if m := teacherNameRe.FindStringSubmatch(cell); m != nil {
				return strings.TrimSpace(m[1])
			}
		}
	}
	return ""
}

func parseSheet(rows [][]string, teacherName string) ([]domain.ImportedLesson, []string) {
	var lessons []domain.ImportedLesson
	var warnings []string

	var currentDay domain.Day

	for rowIdx, row := range rows {
		if rowIdx < 3 {
			continue // пропускаем заголовки
		}
		if len(row) < 3 {
			continue
		}

		col0 := strings.TrimSpace(strings.ReplaceAll(row[0], "\xa0", " "))
		col1 := strings.TrimSpace(strings.ReplaceAll(row[1], "\xa0", " "))
		col2 := ""
		if len(row) > 2 {
			col2 = strings.TrimSpace(strings.ReplaceAll(row[2], "\xa0", " "))
		}

		// forward-fill дня
		if d, ok := dayMap[strings.ToUpper(col0)]; ok {
			currentDay = d
		}

		if currentDay == "" || col1 == "" || col2 == "" {
			continue
		}

		pairNum, ok := timeToSlot[col1]
		if !ok {
			continue
		}
		if pairNum > 6 {
			// пары 7 и 8 (вечерние) пропускаем — не входят в стандартное расписание
			continue
		}

		entries := ParseCell(col2)
		for _, e := range entries {
			if len(e.GroupNames) == 0 {
				warnings = append(warnings, fmt.Sprintf("строка %d: не удалось разобрать группы: %q", rowIdx+1, col2))
				continue
			}
			lessons = append(lessons, domain.ImportedLesson{
				TeacherName: teacherName,
				SubjectName: e.SubjectName,
				ClassType:   e.ClassType,
				Parity:      e.Parity,
				GroupNames:  e.GroupNames,
				RoomNumber:  e.RoomNumber,
				BuildingID:  e.BuildingID,
				Day:         currentDay,
				PairNum:     pairNum,
			})
		}
	}

	return lessons, warnings
}
