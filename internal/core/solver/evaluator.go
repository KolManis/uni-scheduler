package solver

import (
	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// evaluator — расписание, которое умеет быстро пробовать ходы: «переставь пару», «поменяй
// две пары местами» — и сразу знает новый score. Им пользуются все методы улучшения
// (local_search.go, simulated_annealing.go, tabu_search.go, lns, вставка непоставленных).
//
// Зачем он нужен. Посчитать score с нуля (calculateFitness) — пройти все 362 пары. Методы
// улучшения пробуют сотни тысяч ходов, и полный пересчёт на каждом ходе занимал бы минуты.
// Но ход трогает 1–2 пары, а штрафы складываются по «владельцам»: окна группы зависят только
// от пар этой группы, окна преподавателя — только от его пар. Значит, после хода достаточно
// пересчитать только группы и преподавателей передвинутых пар.
//
// Как устроено состояние:
//
//	pairs[i], info[i], slot[i] — пара i: сама пара, её индексы и текущий слот (0..35).
//	    Все ID (преподаватель, группа, аудитория) заменены на номера 0, 1, 2… — так
//	    занятость хранится в массивах, а не в картах, и проверка «свободно ли» мгновенна.
//	teacherOcc / groupOcc / roomOcc — занятость: [кто][слот][неделя] = сколько пар там стоит.
//	groups[g], teachers[t] — какие пары у владельца и его текущий штраф по неделям.
//	week[w] — сумма штрафов всех владельцев в неделю w; score = week[чётная] + week[нечётная]
//	    + суббота + пожелания (breakdown).
//
// Один ход (apply):
//  1. освободить старые места передвигаемых пар;
//  2. проверить, что новые места свободны (fits), иначе вернуть всё как было;
//  3. записать новые слоты и аудитории;
//  4. пересчитать штраф только затронутых групп и преподавателей (refreshGroup/refreshTeacher);
//  5. вернуть «обратный ход» — применив его, можно откатиться.
//
// Результат всегда совпадает с calculateFitness (проверяет тест evaluator_test.go).
//
// Пара может быть «снята» (slot == unplacedSlot): она не занимает слот и не участвует в
// оценке. Так LNS разрушает часть расписания и собирает заново.
type evaluator struct {
	input       domain.InputData
	pairs       []domain.Assignment // пары расписания; меняются только в apply
	info        []pairInfo          // пара i в виде номеров: преподаватель, группы, аудитория
	slot        []int               // текущий слот пары i: 0..35 или unplacedSlot
	teacherBusy [][numSlots][2]bool // преподаватель занят на другом факультете или недоступен: [преп.][слот][неделя]

	// Занятость: [кто][слот][неделя] — сколько пар там стоит (больше 1 не бывает).
	teacherOcc [][numSlots][2]uint8
	groupOcc   [][numSlots][2]uint8
	roomOcc    [][numSlots][2]uint8

	groups   []ownerCache // пары и штраф каждой группы
	teachers []ownerCache // пары и штраф каждого преподавателя
	// teacherUndesired — нежелательные слоты каждого преподавателя (по номеру).
	teacherUndesired [][numSlots]bool
	roomIDs          []string   // номер аудитории → её ID
	rooms            []roomInfo // номер аудитории → корпус, спортзал ли

	saturdayPairs [2]int                     // пар в субботу по неделям (по 200 за каждую)
	week          [2]domain.FitnessBreakdown // сумма штрафов всех групп и преподавателей по неделям
	pref          prefPenalties              // сумма штрафов пожеланий по всем группам

	prefsEnabled bool              // включено хоть одно пожелание
	keyOf        map[string]string // план → название предмета без вида занятия

	// Какие группы и преподаватели затронуты текущим ходом: чтобы не пересчитывать одного
	// владельца дважды, ему ставится отметка moveNumber (номер хода), а не очищается список.
	moveNumber      int
	groupSeenAt     []int
	teacherSeenAt   []int
	changedGroups   []int
	changedTeachers []int
}

const (
	numSlots     = 36 // 6 дней × 6 пар
	unplacedSlot = -1 // пара снята: не стоит ни в каком слоте
	saturdayIdx  = 5  // номер субботы в domain.AllDays; день слота s — s/6
)

// pairInfo — пара в виде номеров вместо ID (см. evaluator).
type pairInfo struct {
	teacher     int
	groups      []int
	room        int
	weeks       [2]bool // в какие недели идёт: [0] — чётная, [1] — нечётная
	roomOptions []int   // аудитории, куда пару можно поставить (HC4–HC6), лучшие по вместимости первыми
}

type roomInfo struct {
	building string
	sport    bool // спортзал или стадион: не участвует в переходах между корпусами
}

// ownerCache — одна группа или один преподаватель: номера его пар и его текущий штраф.
type ownerCache struct {
	members []int
	penalty [2]domain.FitnessBreakdown // по неделям
	pref    prefPenalties              // штраф пожеланий (только у групп)
}

// slotIndex — номер слота 0..35: день × 6 + (пара − 1).
func slotIndex(s domain.TimeSlot) int {
	for i, d := range domain.AllDays {
		if d == s.Day() {
			return i*6 + s.PairNum() - 1
		}
	}
	return 0
}

// slotFromIndex — обратное к slotIndex.
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
		pairs: assignments,
		info:  make([]pairInfo, len(assignments)),
		slot:  make([]int, len(assignments)),
	}
	// Пары с номером от len(placed) и дальше — снятые (pending).

	teacherIdx := map[string]int{}
	groupIdx := map[string]int{}
	roomIdx := map[string]int{}

	roomByID := make(map[string]domain.Room, len(input.Rooms))
	for _, r := range input.Rooms {
		roomByID[r.ID] = r
		indexOf(roomIdx, r.ID)
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

	for i, a := range e.pairs {
		inf := &e.info[i]
		inf.teacher = indexOf(teacherIdx, a.TeacherID)
		for _, gid := range a.GroupIDs {
			inf.groups = append(inf.groups, indexOf(groupIdx, gid))
		}
		inf.room = indexOf(roomIdx, a.RoomID)
		inf.weeks = [2]bool{inWeek(a.Parity, domain.Even), inWeek(a.Parity, domain.Odd)}
		e.slot[i] = slotIndex(a.TimeSlot)
		if i >= len(placed) {
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
	for i, a := range e.pairs {
		if _, ok := roomByID[a.RoomID]; !ok {
			e.rooms[e.info[i].room].building = a.BuildingID
		}
	}

	// Допустимые аудитории пары — те же проверки, что при построении (slotFeasible, findBestRoom).
	for i, a := range e.pairs {
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
			e.info[i].roomOptions = append(e.info[i].roomOptions, c.room)
		}
		if i >= len(placed) && len(cands) > 0 {
			e.info[i].room = cands[0].room
			e.pairs[i].RoomID = e.roomIDs[cands[0].room]
			e.pairs[i].BuildingID = e.rooms[cands[0].room].building
		}
	}

	e.teacherBusy = make([][numSlots][2]bool, len(teacherIdx))
	for tid, slots := range unavail {
		ti, ok := teacherIdx[tid]
		if !ok {
			continue
		}
		for s, p := range slots {
			e.teacherBusy[ti][slotIndex(s)] = [2]bool{inWeek(p, domain.Even), inWeek(p, domain.Odd)}
		}
	}

	e.teacherOcc = make([][numSlots][2]uint8, len(teacherIdx))
	e.groupOcc = make([][numSlots][2]uint8, len(groupIdx))
	e.roomOcc = make([][numSlots][2]uint8, len(roomIdx))
	e.groups = make([]ownerCache, len(groupIdx))
	e.teachers = make([]ownerCache, len(teacherIdx))
	e.teacherUndesired = make([][numSlots]bool, len(teacherIdx))
	for id, mask := range undesiredSlots(input) {
		if t, ok := teacherIdx[id]; ok {
			e.teacherUndesired[t] = mask
		}
	}
	e.groupSeenAt = make([]int, len(groupIdx))
	e.teacherSeenAt = make([]int, len(teacherIdx))

	for i := range e.pairs {
		inf := &e.info[i]
		e.teachers[inf.teacher].members = append(e.teachers[inf.teacher].members, i)
		for _, g := range inf.groups {
			e.groups[g].members = append(e.groups[g].members, i)
		}
		e.occupy(i, e.slot[i], inf.room, 1)
		if isSaturday(e.slot[i]) {
			e.addSaturday(i, 1)
		}
	}

	p := input.Preferences
	e.prefsEnabled = p.SameSubjectSameDay || p.LectureBeforePractice || p.LecturePracticeSameDay
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

// breakdown — score по категориям: две недели плюс субботы плюс пожелания.
func (e *evaluator) breakdown() domain.FitnessBreakdown {
	even, odd := e.week[0], e.week[1]
	even.Saturday += saturdayPairPenalty * e.saturdayPairs[0]
	odd.Saturday += saturdayPairPenalty * e.saturdayPairs[1]
	b := sumWeeks(even, odd)
	b.PracticeBeforeLecture = e.pref.PracticeBeforeLecture
	b.LecturePracticeApart = e.pref.LecturePracticeApart
	b.SubjectSpread = e.pref.SubjectSpread
	return b
}

// assignments — копия текущего расписания (снятые пары не входят).
func (e *evaluator) assignments() []domain.Assignment {
	out := make([]domain.Assignment, 0, len(e.pairs))
	for i, a := range e.pairs {
		if e.slot[i] == unplacedSlot {
			continue
		}
		out = append(out, a)
	}
	return out
}

// addSaturday меняет счётчик суббот на delta в те недели, когда идёт пара i.
func (e *evaluator) addSaturday(i, delta int) {
	for w := 0; w < 2; w++ {
		if e.info[i].weeks[w] {
			e.saturdayPairs[w] += delta
		}
	}
}

// occupy меняет занятость преподавателя, групп и аудитории пары i в slot на delta
// (+1 — занять, −1 — освободить) в те недели, когда идёт пара.
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
	for w := 0; w < 2; w++ {
		if !inf.weeks[w] {
			continue
		}
		if e.teacherBusy[inf.teacher][slot][w] {
			return false
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
	if e.movesPinned(moves) {
		return nil, false
	}
	// 1–2. Освободить старые места и занять новые; не влезло — вернуть как было.
	if !e.reoccupy(moves) {
		return nil, false
	}
	// 3. Записать новые слоты и аудитории, запомнить обратный ход.
	undo := e.writeMoves(moves)
	// 4. Пересчитать штраф затронутых групп и преподавателей.
	longGapsBefore := e.changedLongGaps()
	e.refreshChanged()
	// HC8 (окно в 2+ пары) — жёсткое ограничение для переносов между слотами: ход, который
	// добавил такое окно, откатывается. Снятие и постановка снятых пар (LNS, вставка) могут
	// временно открыть длинное окно — там итог судит score.
	if !touchesUnplaced(moves, undo) && e.changedLongGaps() > longGapsBefore {
		e.apply(undo)
		return nil, false
	}
	// 5. Готово: undo откатит ход.
	return undo, true
}

// movesPinned — ход двигает закреплённую пару («ход на то же место» допустим).
func (e *evaluator) movesPinned(moves []move) bool {
	for _, m := range moves {
		if e.pairs[m.i].Pinned && (m.slot != e.slot[m.i] || m.room != e.info[m.i].room) {
			return true
		}
	}
	return false
}

// reoccupy переносит занятость пар хода со старых мест на новые. Если какое-то новое место
// занято (fits), занятость возвращается как была и результат — false.
func (e *evaluator) reoccupy(moves []move) bool {
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
			return false
		}
		e.occupy(m.i, m.slot, m.room, 1)
	}
	return true
}

// writeMoves записывает в пары новые слоты и аудитории, обновляет счётчик суббот и
// отмечает затронутых владельцев (changedGroups, changedTeachers). Возвращает обратный ход.
func (e *evaluator) writeMoves(moves []move) []move {
	undo := make([]move, len(moves))
	e.moveNumber++
	e.changedGroups = e.changedGroups[:0]
	e.changedTeachers = e.changedTeachers[:0]
	for k, m := range moves {
		undo[k] = move{m.i, e.slot[m.i], e.info[m.i].room}
		if isSaturday(e.slot[m.i]) {
			e.addSaturday(m.i, -1)
		}
		if isSaturday(m.slot) {
			e.addSaturday(m.i, 1)
		}
		e.slot[m.i] = m.slot
		e.info[m.i].room = m.room
		a := &e.pairs[m.i]
		if m.slot != unplacedSlot {
			a.TimeSlot = slotFromIndex(m.slot)
		}
		a.RoomID = e.roomIDs[m.room]
		a.BuildingID = e.rooms[m.room].building
		e.markChanged(m.i)
	}
	return undo
}

// markChanged отмечает преподавателя и группы пары i как затронутые текущим ходом.
func (e *evaluator) markChanged(i int) {
	inf := &e.info[i]
	if e.teacherSeenAt[inf.teacher] != e.moveNumber {
		e.teacherSeenAt[inf.teacher] = e.moveNumber
		e.changedTeachers = append(e.changedTeachers, inf.teacher)
	}
	for _, g := range inf.groups {
		if e.groupSeenAt[g] != e.moveNumber {
			e.groupSeenAt[g] = e.moveNumber
			e.changedGroups = append(e.changedGroups, g)
		}
	}
}

// refreshChanged пересчитывает штраф затронутых ходом групп и преподавателей.
func (e *evaluator) refreshChanged() {
	for _, g := range e.changedGroups {
		e.refreshGroup(g)
	}
	for _, t := range e.changedTeachers {
		e.refreshTeacher(t)
	}
}

// changedLongGaps — штраф за окна в 2+ пары у затронутых ходом групп (по их текущему кешу).
func (e *evaluator) changedLongGaps() int {
	sum := 0
	for _, g := range e.changedGroups {
		sum += e.groups[g].penalty[0].GroupLongGaps + e.groups[g].penalty[1].GroupLongGaps
	}
	return sum
}

// touchesUnplaced — ход снимает пару или ставит снятую (undo хранит прежние слоты).
func touchesUnplaced(moves, undo []move) bool {
	for k := range moves {
		if moves[k].slot == unplacedSlot || undo[k].slot == unplacedSlot {
			return true
		}
	}
	return false
}

// isSaturday — слот s стоит в субботу (снятая пара — нет).
func isSaturday(s int) bool {
	return s != unplacedSlot && s/6 == saturdayIdx
}

// relocate — перенос пары i в slot; если её аудитория там занята, пробуется другая
// допустимая аудитория. Возвращает откат и признак успеха.
func (e *evaluator) relocate(i, slot int) ([]move, bool) {
	if undo, ok := e.apply([]move{{i, slot, e.info[i].room}}); ok {
		return undo, true
	}
	for _, r := range e.info[i].roomOptions {
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

// addBreakdown прибавляет src к dst со знаком sign (+1 или −1).
func addBreakdown(dst *domain.FitnessBreakdown, src domain.FitnessBreakdown, sign int) {
	dst.Saturday += sign * src.Saturday
	dst.GroupDayOverload += sign * src.GroupDayOverload
	dst.GroupLongDay += sign * src.GroupLongDay
	dst.GroupTooFewDays += sign * src.GroupTooFewDays
	dst.GroupUnevenWeek += sign * src.GroupUnevenWeek
	dst.TeacherUndesired += sign * src.TeacherUndesired
	dst.TeacherDayOverload += sign * src.TeacherDayOverload
	dst.TeacherConcentration += sign * src.TeacherConcentration
	dst.GroupGaps += sign * src.GroupGaps
	dst.TeacherGaps += sign * src.TeacherGaps
	dst.BuildingTransitions += sign * src.BuildingTransitions
	dst.SingleClassDay += sign * src.SingleClassDay
	dst.GroupLongGaps += sign * src.GroupLongGaps
}

// refreshGroup пересчитывает штраф группы g: вычитает старый вклад из итога, считает
// новый по её текущим парам и прибавляет.
func (e *evaluator) refreshGroup(g int) {
	c := &e.groups[g]
	for w := 0; w < 2; w++ {
		addBreakdown(&e.week[w], c.penalty[w], -1)
		c.penalty[w] = e.groupWeek(c.members, w)
		addBreakdown(&e.week[w], c.penalty[w], 1)
	}
	if e.prefsEnabled {
		e.pref.PracticeBeforeLecture -= c.pref.PracticeBeforeLecture
		e.pref.LecturePracticeApart -= c.pref.LecturePracticeApart
		e.pref.SubjectSpread -= c.pref.SubjectSpread
		c.pref = e.groupPref(c.members)
		e.pref.PracticeBeforeLecture += c.pref.PracticeBeforeLecture
		e.pref.LecturePracticeApart += c.pref.LecturePracticeApart
		e.pref.SubjectSpread += c.pref.SubjectSpread
	}
}

// refreshTeacher — то же для преподавателя t.
func (e *evaluator) refreshTeacher(t int) {
	c := &e.teachers[t]
	for w := 0; w < 2; w++ {
		addBreakdown(&e.week[w], c.penalty[w], -1)
		c.penalty[w] = e.teacherWeek(t, w)
		addBreakdown(&e.week[w], c.penalty[w], 1)
	}
}

// groupWeek — штрафы одной группы в неделю w: собрать её неделю и применить правила.
func (e *evaluator) groupWeek(members []int, w int) domain.FitnessBreakdown {
	var wk week
	for _, i := range members {
		s := e.slot[i]
		if s == unplacedSlot || !e.info[i].weeks[w] {
			continue
		}
		building := e.pairs[i].BuildingID
		if e.rooms[e.info[i].room].sport {
			building = "" // спортзал и стадион не участвуют в переходах между корпусами
		}
		wk.add(s, building)
	}
	return groupPenalties(&wk)
}

// teacherWeek — штрафы одного преподавателя в неделю w.
func (e *evaluator) teacherWeek(t, w int) domain.FitnessBreakdown {
	wk := week{undesired: e.teacherUndesired[t]}
	for _, i := range e.teachers[t].members {
		if s := e.slot[i]; s != unplacedSlot && e.info[i].weeks[w] {
			wk.add(s, "")
		}
	}
	return teacherPenalties(&wk)
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
			if e.slot[i] == unplacedSlot || e.pairs[i].Type == domain.Lecture {
				continue
			}
			days[e.pairs[i].SubjectID] |= 1 << uint(e.slot[i]/6)
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
		if e.slot[i] == unplacedSlot || e.pairs[i].Type != domain.Lecture {
			continue
		}
		k := e.keyOf[e.pairs[i].SubjectID]
		lectures[k] = append(lectures[k], e.pairs[i].TimeSlot)
	}
	for _, i := range members {
		a := e.pairs[i]
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

// indexOf — номер идентификатора id в m; новый id получает следующий свободный номер.
func indexOf(m map[string]int, id string) int {
	if i, ok := m[id]; ok {
		return i
	}
	m[id] = len(m)
	return m[id]
}
