package solver

import (
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// evaluator — расписание с инкрементальной оценкой и индексом занятости.
//
// Раньше каждый пробный ход копировал весь массив пар, заново собирал карту занятости
// (checkHardConstraints) и пересчитывал штраф всего расписания (calculateFitness) — O(n)
// на ход при O(n²) ходов за проход 2-opt. На 362 парах один проход сходимости занимал
// ~35 секунд, и метаэвристикам почти не оставалось времени.
//
// Здесь всё, что считает calculateFitness, разложено по «владельцам»: штрафы группы
// (окна, форточки, перегрузки, переходы, одиночная суббота, мало дней, необязательные
// правила) зависят только от пар этой группы, штрафы преподавателя — только от его пар.
// Ход меняет слот 1–2 пар, поэтому пересчитываются только их группы и преподаватели.
// Результат совпадает с calculateFitness бит в бит (проверяется тестом).
//
// Пара может быть «снята» (slot == unplacedSlot): она не занимает слот и не участвует
// в оценке. Это нужно LNS для настоящего разрушения-восстановления.
type evaluator struct {
	input   domain.InputData
	asg     []domain.Assignment
	info    []asgInfo
	slot    []int // текущий слот каждой пары, 0..35 или unplacedSlot
	unavail [][numSlots]bool

	teacherOcc [][numSlots][2]uint8
	groupOcc   [][numSlots][2]uint8
	roomOcc    [][numSlots][2]uint8

	groups   []ownerCache
	teachers []ownerCache
	roomIDs  []string
	rooms    []roomInfo

	satCount [2]int                     // пар в субботу по неделям (200 за каждую)
	week     [2]domain.FitnessBreakdown // суммы групповых и преподавательских штрафов по неделям
	pref     prefPenalties

	prefsOn bool
	keyOf   map[string]string // план → название предмета без вида занятия

	stamp    int
	gStamp   []int
	tStamp   []int
	touchedG []int
	touchedT []int
}

const (
	numSlots     = 36 // 6 дней × 6 пар
	unplacedSlot = -1
	saturdayIdx  = 5
)

type asgInfo struct {
	teacher int
	groups  []int
	room    int
	weeks   [2]bool // 0 — чётная, 1 — нечётная
	cands   []int   // аудитории, допустимые для пары (HC4–HC6), лучшие по вместимости первыми
}

type roomInfo struct {
	building string
	sport    bool
}

// ownerCache — пары группы или преподавателя и их текущий вклад в штраф.
type ownerCache struct {
	members []int
	contrib [2]domain.FitnessBreakdown
	pref    prefPenalties
}

func slotIndex(s domain.TimeSlot) int {
	for i, d := range domain.AllDays {
		if d == s.Day() {
			return i*6 + s.PairNum() - 1
		}
	}
	return 0
}

func slotFromIndex(i int) domain.TimeSlot {
	return domain.MustNewTimeSlot(domain.AllDays[i/6], i%6+1)
}

func newEvaluator(assignments []domain.Assignment, input domain.InputData, unavail teacherUnavailable) *evaluator {
	return newEvaluatorWithPending(assignments, nil, input, unavail)
}

// newEvaluatorWithPending — как newEvaluator, но дополнительно держит пары pending
// снятыми: у них нет слота, и их можно поставить через apply/relocate. Аудитория
// снятой пары — лучшая из допустимых (если допустимых нет, поставить её нельзя).
func newEvaluatorWithPending(placed, pending []domain.Assignment, input domain.InputData,
	unavail teacherUnavailable) *evaluator {
	assignments := append(cloneAssignments(placed), pending...)
	e := &evaluator{
		input: input,
		asg:   assignments,
		info:  make([]asgInfo, len(assignments)),
		slot:  make([]int, len(assignments)),
	}
	isPending := func(i int) bool { return i >= len(placed) }

	teacherIdx := map[string]int{}
	groupIdx := map[string]int{}
	roomIdx := map[string]int{}
	idx := func(m map[string]int, id string) int {
		if i, ok := m[id]; ok {
			return i
		}
		m[id] = len(m)
		return m[id]
	}

	roomByID := make(map[string]domain.Room, len(input.Rooms))
	for _, r := range input.Rooms {
		roomByID[r.ID] = r
		idx(roomIdx, r.ID)
	}
	planByID := make(map[string]domain.SubjectPlan, len(input.SubjectPlans))
	for _, sp := range input.SubjectPlans {
		planByID[sp.ID] = sp
	}
	teacherByID := make(map[string]domain.Teacher, len(input.Teachers))
	for _, t := range input.Teachers {
		teacherByID[t.ID] = t
	}
	groupMap := make(map[string]domain.Group, len(input.Groups))
	for _, g := range input.Groups {
		groupMap[g.ID] = g
	}

	for i, a := range e.asg {
		inf := &e.info[i]
		inf.teacher = idx(teacherIdx, a.TeacherID)
		for _, gid := range a.GroupIDs {
			inf.groups = append(inf.groups, idx(groupIdx, gid))
		}
		inf.room = idx(roomIdx, a.RoomID)
		inf.weeks = [2]bool{inWeek(a.Parity, domain.Even), inWeek(a.Parity, domain.Odd)}
		e.slot[i] = slotIndex(a.TimeSlot)
		if isPending(i) {
			e.slot[i] = unplacedSlot
		}
	}

	// Аудитории: сначала известные из справочника, затем встреченные только в парах.
	e.roomIDs = make([]string, len(roomIdx))
	e.rooms = make([]roomInfo, len(roomIdx))
	for id, i := range roomIdx {
		e.roomIDs[i] = id
		if r, ok := roomByID[id]; ok {
			e.rooms[i] = roomInfo{building: r.BuildingID, sport: isSportRoomType(r.Type)}
		}
	}
	for i, a := range e.asg {
		if _, ok := roomByID[a.RoomID]; !ok {
			e.rooms[e.info[i].room].building = a.BuildingID
		}
	}

	// Допустимые аудитории пары — те же проверки, что при построении (findBestRoom).
	for i, a := range e.asg {
		plan, okPlan := planByID[a.SubjectID]
		teacher, okTeacher := teacherByID[a.TeacherID]
		if !okPlan || !okTeacher {
			continue
		}
		total := 0
		for _, gid := range a.GroupIDs {
			total += groupMap[gid].StudentCount
		}
		type cand struct{ room, delta int }
		var cands []cand
		for _, r := range input.Rooms {
			if !isRoomSuitable(r, plan.RequiresRoomType) {
				continue
			}
			if ok, _ := isRoomBigEnoughWithOverflow(r, a.GroupIDs, groupMap); !ok {
				continue
			}
			if !isRoomValidForSubject(r, plan, a.GroupIDs, groupMap, teacher) {
				continue
			}
			d := r.Capacity - total
			if d < 0 {
				d = -d * 3
			}
			cands = append(cands, cand{roomIdx[r.ID], d})
		}
		// Сортировка вставками: кандидатов немного, порядок стабилен.
		for x := 1; x < len(cands); x++ {
			for y := x; y > 0 && cands[y].delta < cands[y-1].delta; y-- {
				cands[y], cands[y-1] = cands[y-1], cands[y]
			}
		}
		for _, c := range cands {
			e.info[i].cands = append(e.info[i].cands, c.room)
		}
		if isPending(i) && len(cands) > 0 {
			e.info[i].room = cands[0].room
			e.asg[i].RoomID = e.roomIDs[cands[0].room]
			e.asg[i].BuildingID = e.rooms[cands[0].room].building
		}
	}

	e.unavail = make([][numSlots]bool, len(teacherIdx))
	for tid, slots := range unavail {
		ti, ok := teacherIdx[tid]
		if !ok {
			continue
		}
		for s := range slots {
			e.unavail[ti][slotIndex(s)] = true
		}
	}

	e.teacherOcc = make([][numSlots][2]uint8, len(teacherIdx))
	e.groupOcc = make([][numSlots][2]uint8, len(groupIdx))
	e.roomOcc = make([][numSlots][2]uint8, len(roomIdx))
	e.groups = make([]ownerCache, len(groupIdx))
	e.teachers = make([]ownerCache, len(teacherIdx))
	e.gStamp = make([]int, len(groupIdx))
	e.tStamp = make([]int, len(teacherIdx))

	for i := range e.asg {
		inf := &e.info[i]
		e.teachers[inf.teacher].members = append(e.teachers[inf.teacher].members, i)
		for _, g := range inf.groups {
			e.groups[g].members = append(e.groups[g].members, i)
		}
		e.occupy(i, e.slot[i], inf.room, 1)
		if e.slot[i] != unplacedSlot && e.slot[i]/6 == saturdayIdx {
			e.addSaturday(i, 1)
		}
	}

	p := input.Preferences
	e.prefsOn = p.SameSubjectSameDay || p.LectureBeforePractice || p.LecturePracticeSameDay
	if p.LectureBeforePractice || p.LecturePracticeSameDay {
		e.keyOf = make(map[string]string, len(input.SubjectPlans))
		for _, sp := range input.SubjectPlans {
			e.keyOf[sp.ID] = subjectKey(sp.Name)
		}
	}

	for g := range e.groups {
		e.refreshGroup(g)
	}
	for t := range e.teachers {
		e.refreshTeacher(t)
	}
	return e
}

// score — то же значение, что calculateFitness для текущих пар (снятые не учитываются).
func (e *evaluator) score() int {
	return e.breakdown().Total()
}

func (e *evaluator) breakdown() domain.FitnessBreakdown {
	even, odd := e.week[0], e.week[1]
	even.Saturday += 200 * e.satCount[0]
	odd.Saturday += 200 * e.satCount[1]
	b := averageBreakdown(even, odd)
	b.PracticeBeforeLecture = e.pref.PracticeBeforeLecture
	b.LecturePracticeApart = e.pref.LecturePracticeApart
	b.SubjectSpread = e.pref.SubjectSpread
	return b
}

// assignments — копия текущего расписания (снятые пары не входят).
func (e *evaluator) assignments() []domain.Assignment {
	out := make([]domain.Assignment, 0, len(e.asg))
	for i, a := range e.asg {
		if e.slot[i] == unplacedSlot {
			continue
		}
		out = append(out, a)
	}
	return out
}

func (e *evaluator) addSaturday(i, delta int) {
	for w := 0; w < 2; w++ {
		if e.info[i].weeks[w] {
			e.satCount[w] += delta
		}
	}
}

func (e *evaluator) occupy(i, slot, room int, delta int) {
	if slot == unplacedSlot {
		return
	}
	inf := &e.info[i]
	for w := 0; w < 2; w++ {
		if !inf.weeks[w] {
			continue
		}
		e.teacherOcc[inf.teacher][slot][w] = uint8(int(e.teacherOcc[inf.teacher][slot][w]) + delta)
		for _, g := range inf.groups {
			e.groupOcc[g][slot][w] = uint8(int(e.groupOcc[g][slot][w]) + delta)
		}
		e.roomOcc[room][slot][w] = uint8(int(e.roomOcc[room][slot][w]) + delta)
	}
}

// fits — можно ли поставить пару i в slot и room при текущей занятости (HC1–HC3, HC7).
func (e *evaluator) fits(i, slot, room int) bool {
	if slot == unplacedSlot {
		return true
	}
	inf := &e.info[i]
	if e.unavail[inf.teacher][slot] {
		return false
	}
	for w := 0; w < 2; w++ {
		if !inf.weeks[w] {
			continue
		}
		if e.teacherOcc[inf.teacher][slot][w] != 0 || e.roomOcc[room][slot][w] != 0 {
			return false
		}
		for _, g := range inf.groups {
			if e.groupOcc[g][slot][w] != 0 {
				return false
			}
		}
	}
	return true
}

// move — изменение одной пары: новый слот и аудитория.
type move struct {
	i, slot, room int
}

// apply применяет набор изменений атомарно: либо все с соблюдением жёстких ограничений,
// либо ни одного. Возвращает обратный набор (для отката) и признак успеха.
// Одна пара не должна встречаться в наборе дважды.
func (e *evaluator) apply(moves []move) ([]move, bool) {
	for _, m := range moves {
		e.occupy(m.i, e.slot[m.i], e.info[m.i].room, -1)
	}
	for k, m := range moves {
		if !e.fits(m.i, m.slot, m.room) {
			for _, prev := range moves[:k] {
				e.occupy(prev.i, prev.slot, prev.room, -1)
			}
			for _, prev := range moves {
				e.occupy(prev.i, e.slot[prev.i], e.info[prev.i].room, 1)
			}
			return nil, false
		}
		e.occupy(m.i, m.slot, m.room, 1)
	}

	undo := make([]move, len(moves))
	// HC8 проверяется только для переносов между слотами: снятие и постановка снятых
	// пар (LNS, вставка) могут временно открыть длинное окно, итог там судит score.
	strict := true
	for _, m := range moves {
		if m.slot == unplacedSlot || e.slot[m.i] == unplacedSlot {
			strict = false
		}
	}
	e.stamp++
	e.touchedG = e.touchedG[:0]
	e.touchedT = e.touchedT[:0]
	for k, m := range moves {
		undo[k] = move{m.i, e.slot[m.i], e.info[m.i].room}
		if e.slot[m.i] != unplacedSlot && e.slot[m.i]/6 == saturdayIdx {
			e.addSaturday(m.i, -1)
		}
		if m.slot != unplacedSlot && m.slot/6 == saturdayIdx {
			e.addSaturday(m.i, 1)
		}
		e.slot[m.i] = m.slot
		e.info[m.i].room = m.room
		a := &e.asg[m.i]
		if m.slot != unplacedSlot {
			a.TimeSlot = slotFromIndex(m.slot)
		}
		a.RoomID = e.roomIDs[m.room]
		a.BuildingID = e.rooms[m.room].building

		inf := &e.info[m.i]
		if e.tStamp[inf.teacher] != e.stamp {
			e.tStamp[inf.teacher] = e.stamp
			e.touchedT = append(e.touchedT, inf.teacher)
		}
		for _, g := range inf.groups {
			if e.gStamp[g] != e.stamp {
				e.gStamp[g] = e.stamp
				e.touchedG = append(e.touchedG, g)
			}
		}
	}
	longBefore := 0
	for _, g := range e.touchedG {
		longBefore += e.groups[g].contrib[0].GroupLongGaps + e.groups[g].contrib[1].GroupLongGaps
		e.refreshGroup(g)
	}
	for _, t := range e.touchedT {
		e.refreshTeacher(t)
	}
	if strict {
		longAfter := 0
		for _, g := range e.touchedG {
			longAfter += e.groups[g].contrib[0].GroupLongGaps + e.groups[g].contrib[1].GroupLongGaps
		}
		if longAfter > longBefore {
			e.apply(undo)
			return nil, false
		}
	}
	return undo, true
}

// relocate — перенос пары i в slot; если её аудитория там занята, пробуется другая
// допустимая аудитория. Возвращает откат и признак успеха.
func (e *evaluator) relocate(i, slot int) ([]move, bool) {
	if undo, ok := e.apply([]move{{i, slot, e.info[i].room}}); ok {
		return undo, true
	}
	for _, r := range e.info[i].cands {
		if r == e.info[i].room || e.roomOcc[r][slot][0]+e.roomOcc[r][slot][1] != 0 {
			continue
		}
		if undo, ok := e.apply([]move{{i, slot, r}}); ok {
			return undo, true
		}
	}
	return nil, false
}

// swap — обмен слотами пар i и j (аудитории остаются за парами).
func (e *evaluator) swap(i, j int) ([]move, bool) {
	return e.apply([]move{{i, e.slot[j], e.info[i].room}, {j, e.slot[i], e.info[j].room}})
}

func addBreakdown(dst *domain.FitnessBreakdown, src domain.FitnessBreakdown, sign int) {
	dst.Saturday += sign * src.Saturday
	dst.GroupDayOverload += sign * src.GroupDayOverload
	dst.GroupLongDay += sign * src.GroupLongDay
	dst.GroupTooFewDays += sign * src.GroupTooFewDays
	dst.TeacherDayOverload += sign * src.TeacherDayOverload
	dst.TeacherConcentration += sign * src.TeacherConcentration
	dst.GroupGaps += sign * src.GroupGaps
	dst.TeacherGaps += sign * src.TeacherGaps
	dst.BuildingTransitions += sign * src.BuildingTransitions
	dst.SingleClassDay += sign * src.SingleClassDay
	dst.GroupLongGaps += sign * src.GroupLongGaps
}

func (e *evaluator) refreshGroup(g int) {
	c := &e.groups[g]
	for w := 0; w < 2; w++ {
		addBreakdown(&e.week[w], c.contrib[w], -1)
		c.contrib[w] = e.groupWeek(c.members, w)
		addBreakdown(&e.week[w], c.contrib[w], 1)
	}
	if e.prefsOn {
		e.pref.PracticeBeforeLecture -= c.pref.PracticeBeforeLecture
		e.pref.LecturePracticeApart -= c.pref.LecturePracticeApart
		e.pref.SubjectSpread -= c.pref.SubjectSpread
		c.pref = e.groupPref(c.members)
		e.pref.PracticeBeforeLecture += c.pref.PracticeBeforeLecture
		e.pref.LecturePracticeApart += c.pref.LecturePracticeApart
		e.pref.SubjectSpread += c.pref.SubjectSpread
	}
}

func (e *evaluator) refreshTeacher(t int) {
	c := &e.teachers[t]
	for w := 0; w < 2; w++ {
		addBreakdown(&e.week[w], c.contrib[w], -1)
		c.contrib[w] = e.teacherWeek(c.members, w)
		addBreakdown(&e.week[w], c.contrib[w], 1)
	}
}

// groupWeek — штрафы одной группы в неделю w; те же правила, что в weekBreakdown.
func (e *evaluator) groupWeek(members []int, w int) domain.FitnessBreakdown {
	var b domain.FitnessBreakdown
	var busy [numSlots]bool
	var building [numSlots]string
	sat := 0
	for _, i := range members {
		s := e.slot[i]
		if s == unplacedSlot || !e.info[i].weeks[w] {
			continue
		}
		busy[s] = true
		if r := e.rooms[e.info[i].room]; !r.sport && building[s] == "" {
			building[s] = e.asg[i].BuildingID
		}
		if s/6 == saturdayIdx {
			sat++
		}
	}
	if sat == 1 {
		b.Saturday += 8000
	}

	days, total := 0, 0
	for d := 0; d < 6; d++ {
		n, first, last, consecutive, prev := 0, -1, -1, 0, -1
		for p := 0; p < 6; p++ {
			s := d*6 + p
			if !busy[s] {
				continue
			}
			n++
			if first < 0 {
				first = p
			}
			last = p
			if prev >= 0 {
				if p-prev == 1 {
					consecutive++
				}
				if p-prev > 2 {
					b.GroupLongGaps += longGapPenalty
				}
				b1, b2 := building[d*6+prev], building[s]
				if b1 != "" && b2 != "" && b1 != b2 {
					switch p - prev {
					case 1:
						b.BuildingTransitions += 2000
					case 2:
						b.BuildingTransitions += 700
					}
				}
			}
			prev = p
		}
		if n == 0 {
			continue
		}
		days++
		total += n
		switch {
		case n >= 5:
			b.GroupDayOverload += (n-4)*4000 + 2000
		case n == 4:
			b.GroupDayOverload += 800
		}
		if n > 4 {
			if consecutive >= 4 {
				b.GroupLongDay += 500
			} else {
				b.GroupLongDay += 300
			}
		}
		if n >= 2 {
			b.GroupGaps += ((last - first + 1) - n) * groupGapPenalty
		}
		if n == 1 {
			b.SingleClassDay += singleClassDayPenalty
		}
	}
	if total >= 4 && days < 2 {
		b.GroupTooFewDays += 600
	} else if total >= 4 && days < 3 {
		b.GroupTooFewDays += 200
	}
	return b
}

// teacherWeek — штрафы одного преподавателя в неделю w.
func (e *evaluator) teacherWeek(members []int, w int) domain.FitnessBreakdown {
	var b domain.FitnessBreakdown
	var busy [numSlots]bool
	for _, i := range members {
		if s := e.slot[i]; s != unplacedSlot && e.info[i].weeks[w] {
			busy[s] = true
		}
	}
	days, total := 0, 0
	for d := 0; d < 6; d++ {
		n, first, last := 0, -1, -1
		for p := 0; p < 6; p++ {
			if !busy[d*6+p] {
				continue
			}
			n++
			if first < 0 {
				first = p
			}
			last = p
		}
		if n == 0 {
			continue
		}
		days++
		total += n
		if n > teacherMaxPairsPerDay {
			b.TeacherDayOverload += (n - teacherMaxPairsPerDay) * 3000
		}
		if n >= 2 {
			b.TeacherGaps += ((last - first + 1) - n) * 60
		}
	}
	if total >= 4 && days < 3 {
		b.TeacherConcentration += (3 - days) * 400
	}
	return b
}

// groupPref — штрафы необязательных правил, относящиеся к одной группе.
// preferencePenalties считает их по парам (пара, группа), поэтому сумма по группам
// совпадает с общим значением.
func (e *evaluator) groupPref(members []int) prefPenalties {
	var p prefPenalties
	prefs := e.input.Preferences

	if prefs.SameSubjectSameDay {
		days := map[string]uint8{}
		for _, i := range members {
			if e.slot[i] == unplacedSlot || e.asg[i].Type == domain.Lecture {
				continue
			}
			days[e.asg[i].SubjectID] |= 1 << uint(e.slot[i]/6)
		}
		for _, mask := range days {
			n := 0
			for ; mask != 0; mask &= mask - 1 {
				n++
			}
			p.SubjectSpread += (n - 1) * subjectSpreadPenalty
		}
	}

	if !prefs.LectureBeforePractice && !prefs.LecturePracticeSameDay {
		return p
	}
	lectures := map[string][]domain.TimeSlot{}
	for _, i := range members {
		if e.slot[i] == unplacedSlot || e.asg[i].Type != domain.Lecture {
			continue
		}
		k := e.keyOf[e.asg[i].SubjectID]
		lectures[k] = append(lectures[k], e.asg[i].TimeSlot)
	}
	for _, i := range members {
		a := e.asg[i]
		if e.slot[i] == unplacedSlot || a.Type == domain.Lecture {
			continue
		}
		lecs, ok := lectures[e.keyOf[a.SubjectID]]
		if !ok {
			continue
		}
		if prefs.LectureBeforePractice && weekPosition(a.TimeSlot) < earliest(lecs) {
			p.PracticeBeforeLecture += practiceBeforeLecturePenalty
		}
		if prefs.LecturePracticeSameDay && !followsLectureSameDay(a.TimeSlot, lecs) {
			p.LecturePracticeApart += lecturePracticeApartPenalty
		}
	}
	return p
}
