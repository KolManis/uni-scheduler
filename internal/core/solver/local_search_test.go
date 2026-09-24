package solver

import (
	"testing"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestImprove_ImprovesGapSchedule(t *testing.T) {
	// G1: пары 1 и 3 в понедельник → окно +10000
	// после улучшения score должен уменьшиться или остаться прежним
	assignments := []domain.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S1", Type: domain.Lecture,
			TimeSlot: domain.MustNewTimeSlot(domain.Monday, 1),
			Parity:   domain.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G1"}, TeacherID: "T2", RoomID: "R2",
			SubjectID: "S2", Type: domain.Practice,
			TimeSlot: domain.MustNewTimeSlot(domain.Monday, 3),
			Parity:   domain.Always, BuildingID: "A",
		},
	}

	before := calculateFitness(assignments, domain.InputData{})
	result := improve(ImproveHillClimb, converge(assignments, domain.InputData{}, time.Now().Add(time.Minute), nil), domain.InputData{}, time.Now().Add(time.Minute), nil)
	after := calculateFitness(result, domain.InputData{})

	if after > before {
		t.Fatalf("improve made schedule worse: before=%d after=%d", before, after)
	}
}

// TestLocalSearch_NeverWorseThanPlainConverge фиксирует главную гарантию iteratedLocalSearch:
// он стартует от результата обычного converge (twoOpt+orOpt до сходимости) и оставляет
// возмущённый вариант только если тот СТРОГО лучше — то есть итоговый score никогда не может
// оказаться хуже, чем у чистого 2-opt/or-opt без итераций.
func TestImprove_NeverWorseThanPlainConverge(t *testing.T) {
	assignments := []domain.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S1", Type: domain.Lecture,
			TimeSlot: domain.MustNewTimeSlot(domain.Monday, 1),
			Parity:   domain.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G1"}, TeacherID: "T2", RoomID: "R2",
			SubjectID: "S2", Type: domain.Practice,
			TimeSlot: domain.MustNewTimeSlot(domain.Monday, 4),
			Parity:   domain.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G2"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S3", Type: domain.Lecture,
			TimeSlot: domain.MustNewTimeSlot(domain.Tuesday, 2),
			Parity:   domain.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G2"}, TeacherID: "T2", RoomID: "R2",
			SubjectID: "S4", Type: domain.Practice,
			TimeSlot: domain.MustNewTimeSlot(domain.Tuesday, 5),
			Parity:   domain.Always, BuildingID: "A",
		},
	}
	input := domain.InputData{}

	convergedOnly := converge(assignments, input, time.Now().Add(5*time.Second), nil)
	full := improve(ImproveHillClimb, convergedOnly, input, time.Now().Add(time.Minute), nil)

	convergedScore := calculateFitness(convergedOnly, input)
	fullScore := calculateFitness(full, input)

	if fullScore > convergedScore {
		t.Fatalf("улучшение (iterated local search) хуже чистого converge: full=%d converge=%d",
			fullScore, convergedScore)
	}
	if !checkHardConstraints(full, nil) {
		t.Fatal("результат улучшения нарушает HC1-3")
	}
}

func TestCheckHardConstraints_NoConflict(t *testing.T) {
	assignments := []domain.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			TimeSlot: domain.MustNewTimeSlot(domain.Monday, 1),
			Parity:   domain.Always,
		},
		{
			GroupIDs: []string{"G2"}, TeacherID: "T2", RoomID: "R2",
			TimeSlot: domain.MustNewTimeSlot(domain.Monday, 1),
			Parity:   domain.Always,
		},
	}
	if !checkHardConstraints(assignments, nil) {
		t.Fatal("different teacher+group+room at same slot should not conflict")
	}
}

func TestCheckHardConstraints_TeacherConflict(t *testing.T) {
	slot := domain.MustNewTimeSlot(domain.Monday, 2)
	assignments := []domain.Assignment{
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: domain.Always},
		{GroupIDs: []string{"G2"}, TeacherID: "T1", RoomID: "R2", TimeSlot: slot, Parity: domain.Always},
	}
	if checkHardConstraints(assignments, nil) {
		t.Fatal("same teacher at same slot should conflict (HC1)")
	}
}

func TestCheckHardConstraints_EvenOddNoConflict(t *testing.T) {
	slot := domain.MustNewTimeSlot(domain.Tuesday, 2)
	assignments := []domain.Assignment{
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: domain.Even},
		{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: domain.Odd},
	}
	if !checkHardConstraints(assignments, nil) {
		t.Fatal("even and odd at same slot should not conflict")
	}
}

// TestCheckHardConstraints_RejectsTeacherUnavailableSlot — основная регрессия на HC7.
// Раньше checkHardConstraints проверял только HC1-3, и локальный поиск мог переставить
// занятие в слот, отмеченный преподавателем как недоступный.
func TestCheckHardConstraints_RejectsTeacherUnavailableSlot(t *testing.T) {
	blocked := domain.MustNewTimeSlot(domain.Monday, 2)
	assignments := []domain.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S1", Type: domain.Lecture,
			TimeSlot: blocked, Parity: domain.Always, BuildingID: "A",
		},
	}
	unavail := buildTeacherUnavailable(domain.InputData{
		Teachers: []domain.Teacher{{ID: "T1", UnavailableSlots: []domain.TimeSlot{blocked}}},
	})

	if checkHardConstraints(assignments, unavail) {
		t.Error("занятие в недоступном слоте преподавателя должно быть отвергнуто")
	}
	if !checkHardConstraints(assignments, nil) {
		t.Error("без карты ограничений то же расписание должно проходить проверку")
	}

	// тот же преподаватель в соседнем слоте — допустимо
	assignments[0].TimeSlot = domain.MustNewTimeSlot(domain.Monday, 3)
	if !checkHardConstraints(assignments, unavail) {
		t.Error("доступный слот не должен отвергаться")
	}
}

// TestConverge_RespectsTeacherUnavailableSlots проверяет, что карта недоступных слотов
// реально доходит до операторов (solveOnce -> converge -> twoOptPass/orOptPass), а не теряется
// по дороге.
//
// Слоты для блокировки подобраны не наугад: на этом входе converge детерминированно
// переносит T1 в Вт-1, а T2 в Вт-2 (проверено отдельным прогоном). Запрещая ровно эти два
// слота, получаем сценарий, в котором без проверки HC7 оба занятия гарантированно
// оказываются в запрещённых слотах.
func TestConverge_RespectsTeacherUnavailableSlots(t *testing.T) {
	assignments := []domain.Assignment{
		{
			GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1",
			SubjectID: "S1", Type: domain.Lecture,
			TimeSlot: domain.MustNewTimeSlot(domain.Monday, 1),
			Parity:   domain.Always, BuildingID: "A",
		},
		{
			GroupIDs: []string{"G1"}, TeacherID: "T2", RoomID: "R2",
			SubjectID: "S2", Type: domain.Practice,
			TimeSlot: domain.MustNewTimeSlot(domain.Monday, 3),
			Parity:   domain.Always, BuildingID: "A",
		},
	}

	blockedT1 := domain.MustNewTimeSlot(domain.Tuesday, 1)
	blockedT2 := domain.MustNewTimeSlot(domain.Tuesday, 2)
	input := domain.InputData{
		Teachers: []domain.Teacher{
			{ID: "T1", UnavailableSlots: []domain.TimeSlot{blockedT1}},
			{ID: "T2", UnavailableSlots: []domain.TimeSlot{blockedT2}},
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
	empty := buildTeacherUnavailable(domain.InputData{
		Teachers: []domain.Teacher{{ID: "T1"}, {ID: "T2"}},
	})
	if empty != nil {
		t.Errorf("без ограничений карта должна быть nil, получено %v", empty)
	}

	slot := domain.MustNewTimeSlot(domain.Friday, 4)
	m := buildTeacherUnavailable(domain.InputData{
		Teachers: []domain.Teacher{
			{ID: "T1", UnavailableSlots: []domain.TimeSlot{slot}},
			{ID: "T2"},
		},
	})
	if m["T1"][slot] != domain.Always {
		t.Errorf("слот T1 должен быть помечен недоступным")
	}
	if _, ok := m["T2"]; ok {
		t.Errorf("T2 без ограничений не должен попадать в карту")
	}
}

// TestExternalPairs_RespectParity — пара на другом факультете по нечётным неделям занимает
// преподавателя только в нечётную неделю: пара «всегда» в этот слот нельзя, по чётным — можно.
func TestExternalPairs_RespectParity(t *testing.T) {
	slot := domain.MustNewTimeSlot(domain.Monday, 2)
	input := domain.InputData{Teachers: []domain.Teacher{{
		ID:            "T1",
		ExternalPairs: []domain.ExternalPair{{TimeSlot: slot, Parity: domain.Odd, Note: "ФИТ"}},
	}}}
	unavail := buildTeacherUnavailable(input)
	mk := func(p domain.Parity) []domain.Assignment {
		return []domain.Assignment{{GroupIDs: []string{"G1"}, TeacherID: "T1", RoomID: "R1", TimeSlot: slot, Parity: p}}
	}
	if checkHardConstraints(mk(domain.Always), unavail) {
		t.Error("пара «всегда» пересекается с внешней нечётной")
	}
	if checkHardConstraints(mk(domain.Odd), unavail) {
		t.Error("нечётная пара пересекается с внешней нечётной")
	}
	if !checkHardConstraints(mk(domain.Even), unavail) {
		t.Error("чётная пара с внешней нечётной не пересекается")
	}

	for _, tc := range []struct {
		parity domain.Parity
		ok     bool
	}{{domain.Even, true}, {domain.Always, false}} {
		elsewhere := mk(tc.parity)
		elsewhere[0].TimeSlot = domain.MustNewTimeSlot(domain.Tuesday, 1)
		e := newEvaluator(elsewhere, input, unavail)
		if _, ok := e.relocate(0, slotIndex(slot)); ok != tc.ok {
			t.Errorf("оценщик, пара %s: перенос в слот внешней нечётной — %v, ожидалось %v", tc.parity, ok, tc.ok)
		}
	}
}

// TestSolve_AvoidsExternalPairs — построение и улучшение не ставят пары преподавателя
// на время его пар на других факультетах.
func TestSolve_AvoidsExternalPairs(t *testing.T) {
	input := makeInput()
	var ext []domain.ExternalPair
	for _, d := range domain.AllDays[:5] {
		for p := 1; p <= 3; p++ {
			ext = append(ext, domain.ExternalPair{TimeSlot: domain.MustNewTimeSlot(d, p), Parity: domain.Always})
		}
	}
	input.Teachers[0].ExternalPairs = ext
	for _, c := range []Construction{ConstructTeacher, ConstructDSatur} {
		sched, err := Solve(input, Options{Construction: c, Improve: ImproveHillClimb, Budget: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range sched.Assignments {
			if a.TeacherID == "T1" && a.TimeSlot.PairNum() <= 3 && a.TimeSlot.Day() != domain.Saturday {
				t.Errorf("%s: пара T1 в %v — там у него пара на другом факультете", c, a.TimeSlot)
			}
		}
		if n := len(ComputeUnplaced(sched.Assignments, input)); n != 0 {
			t.Errorf("%s: не поставлено %d", c, n)
		}
	}
}
