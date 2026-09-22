package solver

import (
	"testing"
	"time"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func TestLocalSearch_ImprovesGapSchedule(t *testing.T) {
	// G1: пары 1 и 3 в понедельник → окно +10000
	// после LocalSearch должен уменьшить или сохранить score
	assignments := []schedule.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S1", Type: schedule.Lecture,
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1},
			Parity:   schedule.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G1"}, TeacherID: "T2", RoomID: "R2",
			SubjectID: "S2", Type: schedule.Practice,
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 3},
			Parity:   schedule.Always, BuildingID: "A",
		},
	}

	before := calculateFitness(assignments, schedule.InputData{})
	result := LocalSearch(assignments, schedule.InputData{})
	after := calculateFitness(result, schedule.InputData{})

	if after > before {
		t.Fatalf("LocalSearch made schedule worse: before=%d after=%d", before, after)
	}
}

// TestLocalSearch_NeverWorseThanPlainConverge фиксирует главную гарантию iteratedLocalSearch:
// он стартует от результата обычного converge (twoOpt+orOpt до сходимости) и оставляет
// возмущённый вариант только если тот СТРОГО лучше — то есть итоговый score никогда не может
// оказаться хуже, чем у чистого 2-opt/or-opt без итераций.
func TestLocalSearch_NeverWorseThanPlainConverge(t *testing.T) {
	assignments := []schedule.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S1", Type: schedule.Lecture,
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1},
			Parity:   schedule.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G1"}, TeacherID: "T2", RoomID: "R2",
			SubjectID: "S2", Type: schedule.Practice,
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 4},
			Parity:   schedule.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G2"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S3", Type: schedule.Lecture,
			TimeSlot: schedule.TimeSlot{Day: schedule.Tuesday, PairNum: 2},
			Parity:   schedule.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G2"}, TeacherID: "T2", RoomID: "R2",
			SubjectID: "S4", Type: schedule.Practice,
			TimeSlot: schedule.TimeSlot{Day: schedule.Tuesday, PairNum: 5},
			Parity:   schedule.Always, BuildingID: "A",
		},
	}
	input := schedule.InputData{}

	convergedOnly := converge(assignments, input, time.Now().Add(5*time.Second), nil)
	full := LocalSearch(assignments, input)

	convergedScore := calculateFitness(convergedOnly, input)
	fullScore := calculateFitness(full, input)

	if fullScore > convergedScore {
		t.Fatalf("LocalSearch (с iterated-возмущением) хуже чистого converge: full=%d converge=%d",
			fullScore, convergedScore)
	}
	if !checkHardConstraints(full, nil) {
		t.Fatal("результат LocalSearch нарушает HC1-3")
	}
}

func TestCheckHardConstraints_NoConflict(t *testing.T) {
	assignments := []schedule.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1},
			Parity:   schedule.Always,
		},
		{
			GroupIDs: []string{"G2"}, TeacherID: "T2", RoomID: "R2",
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1},
			Parity:   schedule.Always,
		},
	}
	if !checkHardConstraints(assignments, nil) {
		t.Fatal("different teacher+group+room at same slot should not conflict")
	}
}

func TestCheckHardConstraints_TeacherConflict(t *testing.T) {
	slot := schedule.TimeSlot{Day: schedule.Monday, PairNum: 2}
	assignments := []schedule.Assignment{
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: schedule.Always},
		{GroupIDs: []string{"G2"}, TeacherID: "T1", RoomID: "R2", TimeSlot: slot, Parity: schedule.Always},
	}
	if checkHardConstraints(assignments, nil) {
		t.Fatal("same teacher at same slot should conflict (HC1)")
	}
}

func TestCheckHardConstraints_EvenOddNoConflict(t *testing.T) {
	slot := schedule.TimeSlot{Day: schedule.Tuesday, PairNum: 2}
	assignments := []schedule.Assignment{
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: schedule.Even},
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: schedule.Odd},
	}
	if !checkHardConstraints(assignments, nil) {
		t.Fatal("even and odd at same slot should not conflict")
	}
}

// TestCheckHardConstraints_RejectsTeacherUnavailableSlot — основная регрессия на HC7.
// Раньше checkHardConstraints проверял только HC1-3, и локальный поиск мог переставить
// занятие в слот, отмеченный преподавателем как недоступный.
func TestCheckHardConstraints_RejectsTeacherUnavailableSlot(t *testing.T) {
	blocked := schedule.TimeSlot{Day: schedule.Monday, PairNum: 2}
	assignments := []schedule.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S1", Type: schedule.Lecture,
			TimeSlot: blocked, Parity: schedule.Always, BuildingID: "A",
		},
	}
	unavail := buildTeacherUnavailable(schedule.InputData{
		Teachers: []schedule.Teacher{{ID: "T1", UnavailableSlots: []schedule.TimeSlot{blocked}}},
	})

	if checkHardConstraints(assignments, unavail) {
		t.Error("занятие в недоступном слоте преподавателя должно быть отвергнуто")
	}
	if !checkHardConstraints(assignments, nil) {
		t.Error("без карты ограничений то же расписание должно проходить проверку")
	}

	// тот же преподаватель в соседнем слоте — допустимо
	assignments[0].TimeSlot = schedule.TimeSlot{Day: schedule.Monday, PairNum: 3}
	if !checkHardConstraints(assignments, unavail) {
		t.Error("доступный слот не должен отвергаться")
	}
}

// TestConverge_RespectsTeacherUnavailableSlots проверяет, что карта недоступных слотов
// реально доходит до операторов (LocalSearch -> converge -> twoOpt/orOpt), а не теряется
// по дороге.
//
// Слоты для блокировки подобраны не наугад: на этом входе converge детерминированно
// переносит T1 в Вт-1, а T2 в Вт-2 (проверено отдельным прогоном). Запрещая ровно эти два
// слота, получаем сценарий, в котором без проверки HC7 оба занятия гарантированно
// оказываются в запрещённых слотах.
func TestConverge_RespectsTeacherUnavailableSlots(t *testing.T) {
	assignments := []schedule.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S1", Type: schedule.Lecture,
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 1},
			Parity:   schedule.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G1"}, TeacherID: "T2", RoomID: "R2",
			SubjectID: "S2", Type: schedule.Practice,
			TimeSlot: schedule.TimeSlot{Day: schedule.Monday, PairNum: 3},
			Parity:   schedule.Always, BuildingID: "A",
		},
	}

	blockedT1 := schedule.TimeSlot{Day: schedule.Tuesday, PairNum: 1}
	blockedT2 := schedule.TimeSlot{Day: schedule.Tuesday, PairNum: 2}
	input := schedule.InputData{
		Teachers: []schedule.Teacher{
			{ID: "T1", UnavailableSlots: []schedule.TimeSlot{blockedT1}},
			{ID: "T2", UnavailableSlots: []schedule.TimeSlot{blockedT2}},
		},
	}

	unavail := buildTeacherUnavailable(input)
	result := converge(assignments, input, time.Now().Add(5*time.Second), unavail)

	for _, a := range result {
		if a.TeacherID == "T1" && a.TimeSlot == blockedT1 {
			t.Errorf("T1 поставлен в свой недоступный слот %v", blockedT1)
		}
		if a.TeacherID == "T2" && a.TimeSlot == blockedT2 {
			t.Errorf("T2 поставлен в свой недоступный слот %v", blockedT2)
		}
	}
}

// TestBuildTeacherUnavailable — карта строится только для преподавателей с ограничениями.
func TestBuildTeacherUnavailable(t *testing.T) {
	empty := buildTeacherUnavailable(schedule.InputData{
		Teachers: []schedule.Teacher{{ID: "T1"}, {ID: "T2"}},
	})
	if empty != nil {
		t.Errorf("без ограничений карта должна быть nil, получено %v", empty)
	}

	slot := schedule.TimeSlot{Day: schedule.Friday, PairNum: 4}
	m := buildTeacherUnavailable(schedule.InputData{
		Teachers: []schedule.Teacher{
			{ID: "T1", UnavailableSlots: []schedule.TimeSlot{slot}},
			{ID: "T2"},
		},
	})
	if !m["T1"][slot] {
		t.Errorf("слот T1 должен быть помечен недоступным")
	}
	if _, ok := m["T2"]; ok {
		t.Errorf("T2 без ограничений не должен попадать в карту")
	}
}
