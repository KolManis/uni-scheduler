package importer

import (
	"regexp"
	"strings"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

// Regex для разбора одного занятия в ячейке.
// Формат: [1н|2н] (тип) Название гр.ГРУППА[, ГРУППА2] АУДИТОРИЯ к.КОРПУС [(КАФЕДРА)]
// Комната всегда начинается с цифры, что позволяет однозначно отделить её от групп.
var cellRe = regexp.MustCompile(
	`(?i)^(1н|2н)?\s*\(([^)]+)\)\s*(.+?)\s+гр\.([\s\S]+?)\s+(\d+\w*)\s+к\.(\p{L}+)`,
)

var typeMap = map[string]schedule.ClassType{
	"лек":  schedule.Lecture,
	"лаб":  schedule.Lab,
	"пр":   schedule.Practice,
	"кср":  schedule.Practice,
	"у.л.": schedule.Lecture,
}

var parityMap = map[string]schedule.Parity{
	"1н": schedule.Odd,
	"2н": schedule.Even,
	"":   schedule.Always,
}

// ParseCell разбирает одну ячейку расписания.
// Ячейка может содержать несколько занятий, разделённых \n.
func ParseCell(raw string) []parsedEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	var result []parsedEntry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.ReplaceAll(line, " ", " ") // UTF-8 неразрывный пробел (U+00A0)
		if e, ok := parseLine(line); ok {
			result = append(result, e)
		}
	}
	return result
}

type parsedEntry struct {
	Parity      schedule.Parity
	ClassType   schedule.ClassType
	SubjectName string
	GroupNames  []string
	RoomNumber  string
	BuildingID  string
}

func parseLine(line string) (parsedEntry, bool) {
	m := cellRe.FindStringSubmatch(line)
	if m == nil {
		return parsedEntry{}, false
	}

	parityStr := strings.TrimSpace(m[1])
	typeStr := strings.TrimSpace(strings.ToLower(m[2]))
	subjectRaw := strings.TrimSpace(m[3])
	groupsRaw := strings.TrimSpace(m[4])
	roomNum := strings.TrimSpace(m[5])
	building := strings.TrimSpace(m[6])

	parity, ok := parityMap[parityStr]
	if !ok {
		parity = schedule.Always
	}

	ct, ok := typeMap[typeStr]
	if !ok {
		ct = schedule.Lecture
	}

	groups := parseGroups(groupsRaw)

	return parsedEntry{
		Parity:      parity,
		ClassType:   ct,
		SubjectName: subjectRaw,
		GroupNames:  groups,
		RoomNumber:  roomNum,
		BuildingID:  building,
	}, true
}

// parseGroups разбивает строку «РИС -23-1б, РИС -23-2б» в слайс имён групп.
func parseGroups(raw string) []string {
	parts := strings.Split(raw, ",")
	var groups []string
	for _, p := range parts {
		name := strings.TrimSpace(p)
		// убираем лишние пробелы внутри имени группы (артефакт Excel)
		name = strings.Join(strings.Fields(name), " ")
		if name != "" {
			groups = append(groups, name)
		}
	}
	return groups
}
