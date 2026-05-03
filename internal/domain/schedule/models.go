package schedule

import "time"

type ClassType string

const (
	Lecture  ClassType = "lecture"
	Practice ClassType = "practice"
	Lab      ClassType = "lab"
)

type Parity string

const (
	Even   Parity = "even"
	Odd    Parity = "odd"
	Always Parity = "always"
)

type Day string

const (
	Monday    Day = "monday"
	Tuesday   Day = "tuesday"
	Wednesday Day = "wednesday"
	Thursday  Day = "thursday"
	Friday    Day = "friday"
	Saturday  Day = "saturday"
)

var AllDays = []Day{Monday, Tuesday, Wednesday, Thursday, Friday, Saturday}

type TimeSlot struct {
	Day     Day `json:"day"`
	PairNum int `json:"pair_num"`
}

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

// SubjectPlan — теперь может быть для нескольких групп (поток)
type SubjectPlan struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	DepartmentID     string   `json:"department_id"`
	LectureHours     int      `json:"lecture_hours"`
	PracticeHours    int      `json:"practice_hours"`
	LabHours         int      `json:"lab_hours"`
	RequiresRoomType string   `json:"requires_room_type"`
	TeacherID        string   `json:"teacher_id"`
	GroupIDs         []string `json:"group_ids"` // ← ТЕПЕРЬ СПИСОК ГРУПП (поток!)
}

// Assignment — одна запись в расписании
type Assignment struct {
	GroupIDs   []string  `json:"group_ids"` // ← список групп (для потока)
	TeacherID  string    `json:"teacher_id"`
	RoomID     string    `json:"room_id"`
	SubjectID  string    `json:"subject_id"`
	Type       ClassType `json:"type"`
	TimeSlot   TimeSlot  `json:"time_slot"`
	Parity     Parity    `json:"parity"`
	BuildingID string    `json:"building_id"`
}

type Schedule struct {
	ID          int64        `json:"id,omitempty"`
	Name        string       `json:"name"`
	Assignments []Assignment `json:"assignments"`
	Score       int          `json:"score"`
	CreatedAt   time.Time    `json:"created_at,omitempty"`
}

type InputData struct {
	Buildings    []Building    `json:"buildings"`
	Departments  []Department  `json:"departments"`
	Groups       []Group       `json:"groups"`
	Teachers     []Teacher     `json:"teachers"`
	Rooms        []Room        `json:"rooms"`
	SubjectPlans []SubjectPlan `json:"subject_plans"`
}
