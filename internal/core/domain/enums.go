package domain

// ClassType — вид занятия.
type ClassType string

const (
	Lecture  ClassType = "lecture"
	Practice ClassType = "practice"
	Lab      ClassType = "lab"
)

func (c ClassType) String() string { return string(c) }

// IsValid сообщает, является ли значение одним из известных видов занятий.
func (c ClassType) IsValid() bool {
	switch c {
	case Lecture, Practice, Lab:
		return true
	default:
		return false
	}
}

// Parity — чётность недели, по которой идёт занятие.
type Parity string

const (
	Even   Parity = "even"
	Odd    Parity = "odd"
	Always Parity = "always"
)

func (p Parity) String() string { return string(p) }

// IsValid сообщает, является ли значение одной из известных чётностей.
// Пустая строка допустима: исторически она означает «каждую неделю».
func (p Parity) IsValid() bool {
	switch p {
	case Even, Odd, Always, "":
		return true
	default:
		return false
	}
}

// Day — учебный день недели.
type Day string

const (
	Monday    Day = "monday"
	Tuesday   Day = "tuesday"
	Wednesday Day = "wednesday"
	Thursday  Day = "thursday"
	Friday    Day = "friday"
	Saturday  Day = "saturday"
)

// AllDays — учебные дни в порядке недели.
var AllDays = []Day{Monday, Tuesday, Wednesday, Thursday, Friday, Saturday}

func (d Day) String() string { return string(d) }

// IsValid сообщает, является ли значение учебным днём недели.
func (d Day) IsValid() bool {
	switch d {
	case Monday, Tuesday, Wednesday, Thursday, Friday, Saturday:
		return true
	default:
		return false
	}
}

// SemesterHalf указывает, в какой половине семестра идёт дисциплина.
//   - "full"   — весь семестр (практики, лабы, постоянные лекции);
//   - "first"  — только 1-я половина (1–8 нед.), например вводные лекционные курсы;
//   - "second" — только 2-я половина (9–16 нед.), например курсовые, защиты.
type SemesterHalf string

const (
	HalfFull   SemesterHalf = "full"
	HalfFirst  SemesterHalf = "first"
	HalfSecond SemesterHalf = "second"
)

func (h SemesterHalf) String() string { return string(h) }

// IsValid сообщает, является ли значение известной половиной семестра.
// Пустая строка допустима: она означает «весь семестр».
func (h SemesterHalf) IsValid() bool {
	switch h {
	case HalfFull, HalfFirst, HalfSecond, "":
		return true
	default:
		return false
	}
}
