package solver

import (
	"log/slog"
	"math/rand"
	"sort"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

type teacherState struct {
	input            domain.InputData
	teacherMap       map[string]domain.Teacher
	groupMap         map[string]domain.Group
	roomMap          map[string]domain.Room
	assignments      []domain.Assignment
	subjectCount     map[string]map[domain.ClassType]int
	occupiedGroups   map[domain.TimeSlot]map[string]domain.Parity
	occupiedTeachers map[domain.TimeSlot]map[string]domain.Parity
	occupiedRooms    map[domain.TimeSlot]map[string]domain.Parity
	teacherSlots     map[string][]domain.TimeSlot
	logger           *slog.Logger
}

func newTeacherState(input domain.InputData) *teacherState {
	tm := make(map[string]domain.Teacher)
	for _, t := range input.Teachers {
		tm[t.ID] = t
	}
	gm := make(map[string]domain.Group)
	for _, g := range input.Groups {
		gm[g.ID] = g
	}
	rm := make(map[string]domain.Room)
	for _, r := range input.Rooms {
		rm[r.ID] = r
	}
	return &teacherState{
		input:            input,
		teacherMap:       tm,
		groupMap:         gm,
		roomMap:          rm,
		assignments:      []domain.Assignment{},
		subjectCount:     make(map[string]map[domain.ClassType]int),
		occupiedGroups:   make(map[domain.TimeSlot]map[string]domain.Parity),
		occupiedTeachers: make(map[domain.TimeSlot]map[string]domain.Parity),
		occupiedRooms:    make(map[domain.TimeSlot]map[string]domain.Parity),
		teacherSlots:     make(map[string][]domain.TimeSlot),
		logger:           slog.Default(),
	}
}

// SolveTeacher — teacher-driven подход: преподаватели идут от самых «жёстких» к самым
// «гибким», их занятия ставятся подряд (компактный дневной график). Внутри преподавателя
// предметы сортируются так, чтобы потоковые лекции (широкий состав, много часов) шли
// первыми — им сложнее всего найти общее окно, поэтому им нужен первый выбор.
//
// Гибкость = (свободных пар в неделю) / (нужных часов). Мало пар в неделю и/или много
// недоступных слотов = ниже гибкость = раньше в очереди. Разбиение состава на подпотоки
// запрещено — см. placeGroupsSplit.
func SolveTeacher(input domain.InputData, maxIter int, improve ImproveAlgorithm) (*domain.Schedule, error) {
	return SolveTeacherWithBudget(input, maxIter, improve, 0, 0)
}

// SolveTeacherWithSeed — SolveTeacher со случайным зерном для многостартового поиска.
// Сохранён для обратной совместимости.
func SolveTeacherWithSeed(input domain.InputData, maxIter int, improve ImproveAlgorithm, seed int64) (*domain.Schedule, error) {
	return SolveTeacherWithBudget(input, maxIter, improve, seed, 0)
}

// SolveTeacherWithBudget — SolveTeacher с явно заданным бюджетом на локальный поиск.
//
// budget == 0 — используется дефолтный localSearchTotalBudget (60 сек). Для параллельных
// сценариев (несколько горутин конкурируют за CPU) вызывающая сторона должна передавать
// бюджет с запасом, иначе deadline срабатывает на converge и метаэвристика не успевает
// сделать ни одной итерации — все методы возвращают одинаковый score чистого построения.
//
// seed == 0 — детерминированное построение. Ненулевой seed перемешивает преподавателей
// и предметы в пределах одной и той же приоритетной группы.
func SolveTeacherWithBudget(input domain.InputData, maxIter int, improve ImproveAlgorithm, seed int64, budget time.Duration) (*domain.Schedule, error) {
	state := newTeacherState(input)

	var rng *rand.Rand
	if seed != 0 {
		rng = rand.New(rand.NewSource(seed))
	}

	teachers := orderTeachersByFlexibility(state)
	if rng != nil {
		shuffleWithinBuckets(teachers, rng, func(t domain.Teacher) float64 {
			return teacherFlexibility(state, t)
		})
	}

	for _, t := range teachers {
		state.logger.Info("processing teacher", "id", t.ID, "name", t.Name, "max_hours", t.MaxWeeklyHours)
		tasks := collectRemaining(state, input, t)
		if rng != nil {
			shuffleTasksWithinPriority(tasks, rng)
		}
		for _, task := range tasks {
			placeTask(state, task)
		}
	}

	if !allSubjectsPlacedTeacher(state) {
		state.logger.Warn("not all subjects placed, using fallback solver")
		return fallbackSolve(state)
	}

	improved := LocalSearch(state.assignments, state.input, improve, budget)
	score := calculateFitness(improved, state.input)

	state.logger.Info("teacher-driven solve complete",
		"assignments", len(improved),
		"score", score,
		"improve", improve,
	)

	return &domain.Schedule{
		Assignments: improved,
		Score:       score,
	}, nil
}

// placeGroupsSplit ставит занятие сразу для всех groupIDs одной парой. Если общего окна на весь
// состав нет — занятие уходит в unplaced. Раньше здесь состав делился пополам и ставился разными
// парами, но это было некорректно: одну и ту же лекцию/практику нельзя проводить дважды
// (потоковую лекцию читают одному потоку, практика по учебному плану — единое занятие).
func placeGroupsSplit(state *teacherState, subject domain.SubjectPlan, classType domain.ClassType,
	teacher domain.Teacher, parity domain.Parity, groupIDs []string) bool {

	slot, room, found := findBestSlot(state, subject, classType, teacher, parity, groupIDs)
	if !found {
		return false
	}
	assignTeacherSubject(state, subject, classType, *slot, room, parity, groupIDs)
	return true
}

// findBestSlot — ищет лучший слот для занятия с day-aware выбором (без окон).
// Порядок дней: пн→вт→ср→чт→пт, суббота только если нет альтернатив.
func findBestSlot(state *teacherState, subject domain.SubjectPlan, classType domain.ClassType,
	teacher domain.Teacher, parity domain.Parity, groupIDs []string) (*domain.TimeSlot, *domain.Room, bool) {

	type candidate struct {
		slot    domain.TimeSlot
		room    domain.Room
		penalty int // меньше = лучше
	}

	var candidates []candidate

	// Сортируем дни по текущей загрузке (меньше пар → пробуем первым)
	// Это гарантирует равномерный разброс по неделе
	dayLoad := make(map[domain.Day]int)
	for _, a := range state.assignments {
		dayLoad[a.TimeSlot.Day()]++
	}
	orderedDays := []domain.Day{
		domain.Monday, domain.Tuesday, domain.Wednesday,
		domain.Thursday, domain.Friday, domain.Saturday,
	}
	sort.SliceStable(orderedDays, func(i, j int) bool {
		di, dj := orderedDays[i], orderedDays[j]
		if di == domain.Saturday {
			return false
		}
		if dj == domain.Saturday {
			return true
		}
		return dayLoad[di] < dayLoad[dj]
	})

	for _, day := range orderedDays {
		for pairNum := 1; pairNum <= 6; pairNum++ {
			slot := domain.MustNewTimeSlot(day, pairNum)

			if !isTeacherAvailable(slot, teacher) {
				continue
			}
			if !isSlotFree(slot, teacher.ID, parity, state.occupiedTeachers) {
				continue
			}
			if !isSlotFreeForAllGroups(slot, groupIDs, parity, state.occupiedGroups) {
				continue
			}

			pen := slotPenalty(state, slot, subject, groupIDs, day, teacher.ID)

			for _, room := range state.input.Rooms {
				if !isRoomSuitable(room, subject.RequiresRoomType) {
					continue
				}
				if !isSlotFree(slot, room.ID, parity, state.occupiedRooms) {
					continue
				}
				ok, _ := isRoomBigEnoughWithOverflow(room, groupIDs, state.groupMap)
				if !ok {
					continue
				}
				if !isRoomValidForSubject(room, subject, groupIDs, state.groupMap, teacher) {
					continue
				}
				candidates = append(candidates, candidate{slot: slot, room: room, penalty: pen})
				break // одна аудитория на слот достаточно для сравнения
			}
		}
	}

	if len(candidates) == 0 {
		return nil, nil, false
	}

	// выбираем кандидата с минимальным штрафом
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.penalty < best.penalty {
			best = c
		}
	}

	// Теперь найдём лучшую аудиторию для выбранного слота (по вместимости)
	bestRoom := findBestRoom(state, subject, best.slot, groupIDs, parity, teacher)
	if bestRoom == nil {
		return nil, nil, false
	}
	return &best.slot, bestRoom, true
}

// slotPenalty вычисляет штраф за постановку занятия в slot для групп groupIDs.
func slotPenalty(state *teacherState, slot domain.TimeSlot, subject domain.SubjectPlan,
	groupIDs []string, day domain.Day, teacherID string) int {

	satPenalty := 0
	if day == domain.Saturday {
		satPenalty = 25000
		for _, gid := range groupIDs {
			satCount := 0
			for _, a := range state.assignments {
				if a.TimeSlot.Day() != domain.Saturday {
					continue
				}
				for _, agid := range a.GroupIDs {
					if agid == gid {
						satCount++
						break
					}
				}
			}
			if satCount == 0 {
				satPenalty += 40000
			}
		}
	}

	// Штраф за окна у групп
	gapPenalty := 0
	// Штраф за перегрузку дня у конкретных групп
	groupLoadPenalty := 0
	// Штраф за переход в другой корпус
	buildingPenalty := 0

	// Корпус нового занятия определяется required_building_id или корпусом группы
	newBuilding := subject.RequiredBuildingID

	for _, gid := range groupIDs {
		existing := groupPairsInDay(state, gid, day)
		n := len(existing)
		if n > 0 {
			all := append(existing, slot.PairNum())
			gapPenalty += calcGaps(all)
		}
		switch {
		case n >= 4:
			groupLoadPenalty += 50000
		case n == 3:
			groupLoadPenalty += 15000
		case n == 2:
			groupLoadPenalty += 4000
		case n == 1:
			groupLoadPenalty += 800
		}

		// Штраф за переход между корпусами
		if newBuilding != "" {
			buildingPenalty += calcBuildingTransitionPenalty(state, gid, day, slot.PairNum(), newBuilding)
		}
	}

	// Штраф за перегрузку дня у преподавателя
	teacherDayPairs := teacherPairsInDay(state, teacherID, day)
	teacherSpread := len(teacherDayPairs) * 400

	// Глобальный штраф за перегруженный день
	globalSpread := totalPairsInDay(state, day) * 20

	return satPenalty + gapPenalty*10000 + groupLoadPenalty + teacherSpread + globalSpread + buildingPenalty
}

// calcBuildingTransitionPenalty начисляет штраф если новое занятие (pairNum, building)
// стоит вплотную или через одно окно к уже поставленным парам группы в другом корпусе.
func calcBuildingTransitionPenalty(state *teacherState, gid string, day domain.Day, pairNum int, newBuilding string) int {
	penalty := 0
	for _, a := range state.assignments {
		if a.TimeSlot.Day() != day {
			continue
		}
		found := false
		for _, agid := range a.GroupIDs {
			if agid == gid {
				found = true
				break
			}
		}
		if !found || a.BuildingID == newBuilding {
			continue
		}
		diff := pairNum - a.TimeSlot.PairNum()
		if diff < 0 {
			diff = -diff
		}
		switch diff {
		case 1: // вплотную — критично
			penalty += 2000
		case 2: // через одно окно — менее критично
			penalty += 700
		}
	}
	return penalty
}

// totalPairsInDay возвращает общее число назначений в указанный день.
func totalPairsInDay(state *teacherState, day domain.Day) int {
	count := 0
	for _, a := range state.assignments {
		if a.TimeSlot.Day() == day {
			count++
		}
	}
	return count
}

// teacherPairsInDay возвращает номера пар преподавателя в указанный день.
func teacherPairsInDay(state *teacherState, teacherID string, day domain.Day) []int {
	var pairs []int
	for _, a := range state.assignments {
		if a.TeacherID == teacherID && a.TimeSlot.Day() == day {
			pairs = append(pairs, a.TimeSlot.PairNum())
		}
	}
	return pairs
}

// groupPairsInDay возвращает номера пар группы в указанный день.
func groupPairsInDay(state *teacherState, groupID string, day domain.Day) []int {
	var pairs []int
	for _, a := range state.assignments {
		if a.TimeSlot.Day() != day {
			continue
		}
		for _, gid := range a.GroupIDs {
			if gid == groupID {
				pairs = append(pairs, a.TimeSlot.PairNum())
				break
			}
		}
	}
	return pairs
}

// calcGaps возвращает количество «окон» в наборе пар (пропуски между занятиями).
func calcGaps(pairs []int) int {
	if len(pairs) < 2 {
		return 0
	}
	min, max := pairs[0], pairs[0]
	for _, p := range pairs[1:] {
		if p < min {
			min = p
		}
		if p > max {
			max = p
		}
	}
	return (max - min + 1) - len(pairs)
}

// findBestRoom ищет подходящую аудиторию для заданного слота.
// Выбирает аудиторию с минимальным превышением вместимости.
func findBestRoom(state *teacherState, subject domain.SubjectPlan, slot domain.TimeSlot,
	groupIDs []string, parity domain.Parity, teacher domain.Teacher) *domain.Room {
	totalStudents := 0
	for _, gid := range groupIDs {
		if g, ok := state.groupMap[gid]; ok {
			totalStudents += g.StudentCount
		}
	}

	var best *domain.Room
	bestDelta := -1
	for _, room := range state.input.Rooms {
		if !isRoomSuitable(room, subject.RequiresRoomType) {
			continue
		}
		if !isSlotFree(slot, room.ID, parity, state.occupiedRooms) {
			continue
		}
		ok, _ := isRoomBigEnoughWithOverflow(room, groupIDs, state.groupMap)
		if !ok {
			continue
		}
		if !isRoomValidForSubject(room, subject, groupIDs, state.groupMap, teacher) {
			continue
		}
		delta := room.Capacity - totalStudents
		if delta < 0 {
			delta = -delta * 3 // переполнение штрафуем сильнее, чем пустое место
		}
		if best == nil || delta < bestDelta {
			r := room
			best = &r
			bestDelta = delta
		}
	}
	return best
}

// placementTask — одна пара, которую надо разместить: единица работы для фаз 1–3.
type placementTask struct {
	teacher   domain.Teacher
	subject   domain.SubjectPlan
	classType domain.ClassType
	parity    domain.Parity
}

// orderTeachersByFlexibility — от самых «жёстких» к самым «гибким». Гибкость = сколько
// пар в неделю преподаватель реально может провести, делённое на сколько ему надо провести.
// Значение < 1 — преподаватель ограничен физически (у него меньше свободных пар, чем нагрузка).
// При равной гибкости — вперёд идут те, у кого больше предметов (сложнее собрать расписание).
func orderTeachersByFlexibility(state *teacherState) []domain.Teacher {
	teachers := make([]domain.Teacher, len(state.input.Teachers))
	copy(teachers, state.input.Teachers)

	subjectCount := make(map[string]int)
	for _, sp := range state.input.SubjectPlans {
		subjectCount[sp.TeacherID]++
	}

	sort.Slice(teachers, func(i, j int) bool {
		fi, fj := teacherFlexibility(state, teachers[i]), teacherFlexibility(state, teachers[j])
		if fi != fj {
			return fi < fj
		}
		return subjectCount[teachers[i].ID] > subjectCount[teachers[j].ID]
	})
	return teachers
}

// teacherFlexibility — доступных пар в неделю на один нужный час. Меньше = жёстче.
// Используется и в основной сортировке, и в bucketing для случайных перемешиваний.
func teacherFlexibility(state *teacherState, t domain.Teacher) float64 {
	const totalSlots = 36 // 6 дней × 6 пар

	need := 0
	for _, sp := range state.input.SubjectPlans {
		if sp.TeacherID == t.ID {
			need += sp.LectureHours + sp.PracticeHours + sp.LabHours
		}
	}
	if need <= 0 {
		return 1e9 // без нагрузки — максимально гибкий, в конец
	}
	available := totalSlots - len(t.UnavailableSlots)
	if available <= 0 {
		return 0
	}
	return float64(available) / float64(need)
}

// shuffleWithinBuckets перемешивает элементы уже отсортированного слайса, но только
// внутри непрерывных участков, где ключ одинаковый. Это ломает произвольный tie-break,
// не нарушая доминирующий порядок.
func shuffleWithinBuckets[T any](items []T, rng *rand.Rand, key func(T) float64) {
	i := 0
	for i < len(items) {
		j := i + 1
		for j < len(items) && key(items[j]) == key(items[i]) {
			j++
		}
		if j-i > 1 {
			bucket := items[i:j]
			rng.Shuffle(len(bucket), func(a, b int) { bucket[a], bucket[b] = bucket[b], bucket[a] })
		}
		i = j
	}
}

// shuffleTasksWithinPriority перемешивает задачи одного класса и близких приоритетов.
// Порядок «лекции первыми, потом практики, потом лабы» сохраняется — рандомизация только
// в пределах одного classType между разными предметами.
func shuffleTasksWithinPriority(tasks []placementTask, rng *rand.Rand) {
	i := 0
	for i < len(tasks) {
		j := i + 1
		for j < len(tasks) && tasks[j].classType == tasks[i].classType {
			j++
		}
		if j-i > 1 {
			bucket := tasks[i:j]
			rng.Shuffle(len(bucket), func(a, b int) { bucket[a], bucket[b] = bucket[b], bucket[a] })
		}
		i = j
	}
}

// collectRemaining — пары этого преподавателя, которые ещё не поставлены. Порядок:
// одиночные лекции → практики → лабы. Внутри типа — предметы с большим потоком идут раньше.
func collectRemaining(state *teacherState, input domain.InputData, teacher domain.Teacher) []placementTask {
	subjects := getSubjectsForTeacher(input, teacher.ID)
	sortSubjectsByPriority(subjects)

	var tasks []placementTask
	for _, classType := range []domain.ClassType{domain.Lecture, domain.Practice, domain.Lab} {
		for _, sp := range subjects {
			remaining := getRemainingHours(state, sp, classType)
			if remaining <= 0 {
				continue
			}
			parity := sp.Parity
			if parity == "" {
				parity = domain.Always
			}
			for h := 0; h < remaining; h += 2 {
				tasks = append(tasks, placementTask{teacher: teacher, subject: sp, classType: classType, parity: parity})
			}
		}
	}
	return tasks
}

// placeTask пытается поставить одну пару. Если общего окна на весь состав нет —
// пара уходит в unplaced (без дробления, см. placeGroupsSplit).
func placeTask(state *teacherState, task placementTask) {
	if getRemainingHours(state, task.subject, task.classType) <= 0 {
		return
	}
	if !placeGroupsSplit(state, task.subject, task.classType, task.teacher, task.parity, task.subject.GroupIDs) {
		state.logger.Warn("cannot place",
			"subject", task.subject.ID,
			"type", task.classType,
			"parity", task.parity,
			"teacher", task.teacher.ID,
			"groups", len(task.subject.GroupIDs),
		)
	}
}

// sortSubjectsByPriority — сортировка предметов
func sortSubjectsByPriority(subjects []domain.SubjectPlan) {
	sort.Slice(subjects, func(i, j int) bool {
		lenI := len(subjects[i].GroupIDs)
		lenJ := len(subjects[j].GroupIDs)
		if lenI != lenJ {
			return lenI > lenJ
		}
		totalI := subjects[i].LectureHours + subjects[i].PracticeHours + subjects[i].LabHours
		totalJ := subjects[j].LectureHours + subjects[j].PracticeHours + subjects[j].LabHours
		return totalI > totalJ
	})
}

// assignTeacherSubject — назначение предмета (groupIDs может быть подмножеством subject.GroupIDs,
// если поток был поделён на части через placeGroupsSplit).
func assignTeacherSubject(state *teacherState, subject domain.SubjectPlan, classType domain.ClassType,
	slot domain.TimeSlot, room *domain.Room, parity domain.Parity, groupIDs []string) {

	hours := 2

	assignment := domain.Assignment{
		GroupIDs:   groupIDs,
		TeacherID:  subject.TeacherID,
		RoomID:     room.ID,
		SubjectID:  subject.ID,
		Type:       classType,
		TimeSlot:   slot,
		Parity:     parity,
		BuildingID: room.BuildingID,
	}

	state.assignments = append(state.assignments, assignment)

	if state.subjectCount[subject.ID] == nil {
		state.subjectCount[subject.ID] = make(map[domain.ClassType]int)
	}
	state.subjectCount[subject.ID][classType] += hours

	for _, gid := range groupIDs {
		if state.occupiedGroups[slot] == nil {
			state.occupiedGroups[slot] = make(map[string]domain.Parity)
		}
		state.occupiedGroups[slot][gid] = mergeParity(state.occupiedGroups[slot][gid], parity)
	}

	if state.occupiedTeachers[slot] == nil {
		state.occupiedTeachers[slot] = make(map[string]domain.Parity)
	}
	state.occupiedTeachers[slot][subject.TeacherID] = mergeParity(state.occupiedTeachers[slot][subject.TeacherID], parity)

	if state.occupiedRooms[slot] == nil {
		state.occupiedRooms[slot] = make(map[string]domain.Parity)
	}
	state.occupiedRooms[slot][room.ID] = mergeParity(state.occupiedRooms[slot][room.ID], parity)

	state.teacherSlots[subject.TeacherID] = append(state.teacherSlots[subject.TeacherID], slot)
}

// mergeParity — объединение чётностей
func mergeParity(existing, new domain.Parity) domain.Parity {
	if existing == "" {
		return new
	}
	if existing == domain.Always || new == domain.Always {
		return domain.Always
	}
	if existing != new {
		return domain.Always
	}
	return existing
}

// getRemainingHours — оставшиеся часы
func getRemainingHours(state *teacherState, subject domain.SubjectPlan, classType domain.ClassType) int {
	current := 0
	if state.subjectCount[subject.ID] != nil {
		current = state.subjectCount[subject.ID][classType]
	}
	var total int
	switch classType {
	case domain.Lecture:
		total = subject.LectureHours
	case domain.Practice:
		total = subject.PracticeHours
	case domain.Lab:
		total = subject.LabHours
	}
	return total - current
}

// subjectRemainingTeacher — проверка остатка: пробовать ли ещё ставить этот предмет.
// В отличие от allSubjectsPlacedTeacher, здесь мультигрупповые практики/лабы НЕ пропускаются —
// иначе они вообще ни разу не попадут в findBestSlot/placeGroupsSplit и тихо останутся
// неразмещёнными, даже не попытавшись разъехаться по разным слотам (см. ComputeUnplaced).
func subjectRemainingTeacher(state *teacherState, subject domain.SubjectPlan) bool {
	for _, ct := range []domain.ClassType{domain.Lecture, domain.Practice, domain.Lab} {
		if getRemainingHours(state, subject, ct) > 0 {
			return true
		}
	}
	return false
}

// allSubjectsPlacedTeacher — проверка всех предметов.
// Мультигрупповые практики/лабы не требуются (best-effort).
func allSubjectsPlacedTeacher(state *teacherState) bool {
	for _, plan := range state.input.SubjectPlans {
		for _, ct := range []domain.ClassType{domain.Lecture, domain.Practice, domain.Lab} {
			if ct != domain.Lecture && len(plan.GroupIDs) > 1 {
				continue
			}
			if getRemainingHours(state, plan, ct) > 0 {
				return false
			}
		}
	}
	return true
}

// fallbackSolve — fallback на параллельный солвер
func fallbackSolve(state *teacherState) (*domain.Schedule, error) {
	state.logger.Info("using fallback parallel solver")
	return SolveParallel(state.input, 100000, 4)
}
