package domain

import "time"

type Building struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

type Department struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Group struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	StudentCount int      `json:"student_count"`
	BuildingIDs  []string `json:"building_ids"`
}

type Teacher struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	DepartmentID       string     `json:"department_id"`
	MaxWeeklyHours     int        `json:"max_weekly_hours"`
	UnavailableSlots   []TimeSlot `json:"unavailable_slots,omitempty"`
	PreferredBuildings []string   `json:"preferred_buildings,omitempty"`
}

type Room struct {
	ID         string `json:"id"`
	Number     string `json:"number"`
	BuildingID string `json:"building_id"`
	Capacity   int    `json:"capacity"`
	Type       string `json:"type"`
}

type SubjectPlan struct {
	ID                 string       `json:"id"`
	Name               string       `json:"name"`
	DepartmentID       string       `json:"department_id"`
	LectureHours       int          `json:"lecture_hours"`
	PracticeHours      int          `json:"practice_hours"`
	LabHours           int          `json:"lab_hours"`
	RequiresRoomType   string       `json:"requires_room_type"`
	RequiredBuildingID string       `json:"required_building_id"` // если задан, занятие только в этом корпусе
	TeacherID          string       `json:"teacher_id"`
	GroupIDs           []string     `json:"group_ids"`
	Parity             Parity       `json:"parity"`
	SemesterHalf       SemesterHalf `json:"semester_half"`
}

type Assignment struct {
	GroupIDs   []string  `json:"group_ids"`
	TeacherID  string    `json:"teacher_id"`
	RoomID     string    `json:"room_id"`
	SubjectID  string    `json:"subject_id"`
	Type       ClassType `json:"type"`
	TimeSlot   TimeSlot  `json:"time_slot"`
	Parity     Parity    `json:"parity"`
	BuildingID string    `json:"building_id"`
}

// UnplacedItem описывает часть учебного плана, которую солвер не смог поставить в расписание
// (не хватило слотов/аудиторий с учётом всех ограничений).
type UnplacedItem struct {
	SubjectID    string    `json:"subject_id"`
	Type         ClassType `json:"type"`
	MissingHours int       `json:"missing_hours"`
}

type Schedule struct {
	ID          int64             `json:"id,omitempty"`
	Name        string            `json:"name"`
	Assignments []Assignment      `json:"assignments"`
	Score       int               `json:"score"`
	Unplaced    []UnplacedItem    `json:"unplaced,omitempty"`
	Options     SolverPreferences `json:"options"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
}

// ScheduleSummary — расписание без тела assignments: для списка расписаний.
type ScheduleSummary struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Score         int       `json:"score"`
	TotalPairs    int       `json:"total_pairs"`
	UnplacedCount int       `json:"unplaced_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// FitnessBreakdown — расшифровка итогового score по категориям штрафов.
// Нужна, чтобы объяснить, откуда взялась конкретная цифра score, а не только её значение.
// Считается солвером, но описывает качество расписания — поэтому живёт в домене,
// как и UnplacedItem.
type FitnessBreakdown struct {
	Saturday              int // суббота + одиночная суббота у группы
	GroupDayOverload      int // перегрузка дня у группы (4+ / 5+ пар)
	GroupLongDay          int // длинный день (>4 пар подряд)
	GroupTooFewDays       int // мало дней при большой нагрузке (группы)
	TeacherDayOverload    int // день >2 пар у преподавателя
	TeacherConcentration  int // <3 активных дней у преподавателя при загрузке
	GroupGaps             int // окна у групп (дороже всего — 10000 за окно)
	TeacherGaps           int // окна у преподавателей
	BuildingTransitions   int // переходы между корпусами вплотную/через окно
	SingleClassDay        int // «форточка» — всего 1 пара в день у группы
	PracticeBeforeLecture int // практика раньше лекции по предмету (если включено)
	SubjectSpread         int // пары одного плана разнесены по разным дням (если включено)
}

// Total суммирует все категории — должно совпадать с итоговым score.
func (b FitnessBreakdown) Total() int {
	return b.Saturday + b.GroupDayOverload + b.GroupLongDay + b.GroupTooFewDays +
		b.TeacherDayOverload + b.TeacherConcentration + b.GroupGaps + b.TeacherGaps +
		b.BuildingTransitions + b.SingleClassDay + b.PracticeBeforeLecture + b.SubjectSpread
}

// QualityStats — показатели расписания в «человеческих» единицах (штуки, пары),
// а не в баллах штрафа. По ним видно, что именно плохо, без знания весов fitness.
type QualityStats struct {
	GroupGaps            int // окон у групп (пустых пар между занятиями за день)
	SingleClassDays      int // дней, где у группы ровно одна пара
	SaturdayPairs        int // пар в субботу
	MaxGroupPairsPerDay  int // максимум пар в день у одной группы
	MaxTeacherPairsInDay int // максимум пар в день у одного преподавателя
}

// SolverPreferences — необязательные правила, включаемые при генерации.
// Все выключены по умолчанию: без них алгоритм работает как раньше.
type SolverPreferences struct {
	// LectureBeforePractice — практика и лабораторная по предмету у группы не должны
	// стоять в неделе раньше лекции по нему. Лекция и практика — разные учебные планы,
	// связь между ними — название предмета без пометки типа в скобках.
	LectureBeforePractice bool `json:"lecture_before_practice"`
	// SameSubjectSameDay — несколько пар одного плана (например, две лабораторные
	// в неделю) у группы ставить в один день, а не разносить по неделе.
	SameSubjectSameDay bool `json:"same_subject_same_day"`
}

type InputData struct {
	Buildings    []Building        `json:"buildings"`
	Departments  []Department      `json:"departments"`
	Groups       []Group           `json:"groups"`
	Teachers     []Teacher         `json:"teachers"`
	Rooms        []Room            `json:"rooms"`
	SubjectPlans []SubjectPlan     `json:"subject_plans"`
	Preferences  SolverPreferences `json:"-"`
}
