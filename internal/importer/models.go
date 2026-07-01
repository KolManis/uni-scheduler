package importer

import "github.com/KolManis/uni-scheduler/internal/domain/schedule"

// ImportedLesson — одно занятие, извлечённое из ячейки Excel.
type ImportedLesson struct {
	TeacherName string
	SubjectName string
	ClassType   schedule.ClassType
	Parity      schedule.Parity
	GroupNames  []string
	RoomNumber  string
	BuildingID  string
	Day         schedule.Day
	PairNum     int
}

// ImportedData — всё, что извлечено из одного xlsx-файла.
type ImportedData struct {
	Lessons []ImportedLesson
}

// ImportResult — итог импорта (возвращается в HTTP-ответе).
type ImportResult struct {
	TeachersCreated int      `json:"teachers_created"`
	GroupsCreated   int      `json:"groups_created"`
	SubjectsCreated int      `json:"subjects_created"`
	RoomsCreated    int      `json:"rooms_created"`
	Warnings        []string `json:"warnings"`
}
