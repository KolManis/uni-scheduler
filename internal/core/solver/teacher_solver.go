package solver

import (
	"log/slog"
	"math/rand"
	"sort"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// scheduleDraft — черновик расписания во время построения: поставленные пары и занятость
// преподавателей, групп и аудиторий по слотам. Меняет его только addAssignment (через
// placePair и placeFixed); остальные функции построения его только читают.
type scheduleDraft struct {
	input            domain.InputData
	groupMap         map[string]domain.Group
	roomMap          map[string]domain.Room
	assignments      []domain.Assignment
	subjectCount     map[string]map[domain.ClassType]int
	occupiedGroups   map[domain.TimeSlot]map[string]domain.Parity
	occupiedTeachers map[domain.TimeSlot]map[string]domain.Parity
	occupiedRooms    map[domain.TimeSlot]map[string]domain.Parity
	planKeys         map[string]string // id плана → subjectKey, считается один раз
	logger           *slog.Logger
}

func newDraft(input domain.InputData) *scheduleDraft {
	gm := make(map[string]domain.Group)
	for _, g := range input.Groups {
		gm[g.ID] = g
	}
	rm := make(map[string]domain.Room)
	for _, r := range input.Rooms {
		rm[r.ID] = r
	}
	pk := make(map[string]string, len(input.SubjectPlans))
	for _, sp := range input.SubjectPlans {
		pk[sp.ID] = subjectKey(sp.Name)
	}
	// Пары преподавателей на других факультетах занимают их заранее, в свою чётность:
	// построение обходит их так же, как уже поставленные пары (HC7).
	occupiedTeachers := make(map[domain.TimeSlot]map[string]domain.Parity)
	for _, t := range input.Teachers {
		for _, ep := range t.ExternalPairs {
			p := ep.Parity
			if p == "" {
				p = domain.Always
			}
			if occupiedTeachers[ep.TimeSlot] == nil {
				occupiedTeachers[ep.TimeSlot] = make(map[string]domain.Parity)
			}
			occupiedTeachers[ep.TimeSlot][t.ID] = mergeParity(occupiedTeachers[ep.TimeSlot][t.ID], p)
		}
	}
	return &scheduleDraft{
		input:            input,
		groupMap:         gm,
		roomMap:          rm,
		assignments:      []domain.Assignment{},
		subjectCount:     make(map[string]map[domain.ClassType]int),
		occupiedGroups:   make(map[domain.TimeSlot]map[string]domain.Parity),
		occupiedTeachers: occupiedTeachers,
		occupiedRooms:    make(map[domain.TimeSlot]map[string]domain.Parity),
		planKeys:         pk,
		logger:           slog.Default(),
	}
}

// Construction — алгоритм построения начального расписания.
type Construction string

const (
	ConstructTeacher Construction = "teacher" // по преподавателям, от самых ограниченных
	ConstructDSatur  Construction = "dsatur"  // самая трудная пара первой (DSatur, раскраска графа; по умолчанию)
)

// ParseConstruction — алгоритм построения по значению solver_type. Пустое — DSatur:
// с ним улучшение даёт лучший и самый стабильный результат (ADR-0019).
func ParseConstruction(s string) (Construction, bool) {
	switch Construction(s) {
	case "", ConstructDSatur:
		return ConstructDSatur, true
	case ConstructTeacher:
		return ConstructTeacher, true
	}
	return "", false
}

// Options — как составлять расписание. Нулевые значения — разумные умолчания.
type Options struct {
	Construction Construction     // алгоритм построения; пусто — DSatur
	Improve      ImproveAlgorithm // метод улучшения; пусто — iterated local search
	Budget       time.Duration    // время на улучшение; 0 — localSearchTotalBudget (60 с)
	// Seed — 0: построение детерминированное; иначе пары с равным приоритетом
	// перемешиваются этим зерном (для нескольких запусков).
	Seed int64
	// Starts — сколько запусков с разными зёрнами сделать параллельно и взять лучший;
	// 0 и 1 — один запуск.
	Starts int
	// Fixed — закреплённые пары: стоят заранее и не двигаются, построение ставит остальные
	// вокруг них, их часы засчитываются в план.
	Fixed []domain.Assignment
}

// Solve составляет расписание: построение, вставка непоставленных, улучшение.
// Непоставленные пары не ошибка: они видны через ComputeUnplaced.
func Solve(input domain.InputData, opt Options) (*domain.Schedule, error) {
	if opt.Construction == "" {
		opt.Construction = ConstructDSatur
	}
	if opt.Starts > 1 {
		return solveMultiStart(input, opt)
	}
	return solveOnce(input, opt)
}

// solveOnce — один запуск. Весь путь от входных данных до расписания — здесь, по шагам.
func solveOnce(input domain.InputData, opt Options) (*domain.Schedule, error) {
	input = normalizeInput(input) // порядок справочников не должен влиять на результат (ADR-0008)
	unavail := buildTeacherUnavailable(input)
	budget := opt.Budget
	if budget <= 0 {
		budget = localSearchTotalBudget
	}
	deadline := time.Now().Add(budget)

	// 1. Построение: пары ставятся по одной, каждая — в лучший на этот момент слот.
	pairs := construct(input, opt)
	// 2. Пары, не поместившиеся при построении, — ещё попытка, с вытеснением соседей.
	pairs = insertUnplaced(pairs, input, unavail)
	// 3. Сходимость: простые перестановки, пока расписание улучшается.
	pairs = converge(pairs, input, deadline, unavail)
	// 4. Улучшение выбранным методом — выход из локального оптимума.
	pairs = improve(opt.Improve, pairs, input, deadline, unavail)
	// 5. Улучшение могло освободить место — последняя попытка поставить оставшиеся пары.
	pairs = insertUnplaced(pairs, input, unavail)

	score := calculateFitness(pairs, input)
	slog.Default().Info("solve complete",
		"construction", opt.Construction, "improve", opt.Improve,
		"assignments", len(pairs), "score", score)
	return &domain.Schedule{Assignments: pairs, Score: score}, nil
}

// construct — начальное расписание выбранным алгоритмом, вокруг закреплённых пар.
// Непоставленные пары не ошибка: они попадут в отчёт «Не размещено».
func construct(input domain.InputData, opt Options) []domain.Assignment {
	draft := newDraft(input)
	for _, a := range opt.Fixed {
		placeFixed(draft, a)
	}

	var rng *rand.Rand // nil — без перемешивания, построение детерминированное
	if opt.Seed != 0 {
		rng = rand.New(rand.NewSource(opt.Seed))
	}
	if opt.Construction == ConstructDSatur {
		constructDSatur(draft, rng)
	} else {
		constructByTeacher(draft, rng)
	}

	if !allSubjectsPlacedTeacher(draft) {
		draft.logger.Warn("not all subjects placed, continuing; see unplaced report")
	}
	return draft.assignments
}

// placeFixed ставит закреплённую пару как есть.
func placeFixed(draft *scheduleDraft, a domain.Assignment) {
	a.Pinned = true
	if a.Parity == "" {
		a.Parity = domain.Always
	}
	addAssignment(draft, a)
}

// addAssignment добавляет пару в расписание: занимает преподавателя, группы и аудиторию
// в её слот и чётность и засчитывает её часы в план.
func addAssignment(draft *scheduleDraft, a domain.Assignment) {
	draft.assignments = append(draft.assignments, a)
	if draft.subjectCount[a.SubjectID] == nil {
		draft.subjectCount[a.SubjectID] = make(map[domain.ClassType]int)
	}
	draft.subjectCount[a.SubjectID][a.Type] += 2 // пара — 2 часа

	for _, gid := range a.GroupIDs {
		occupy(draft.occupiedGroups, a.TimeSlot, gid, a.Parity)
	}
	occupy(draft.occupiedTeachers, a.TimeSlot, a.TeacherID, a.Parity)
	occupy(draft.occupiedRooms, a.TimeSlot, a.RoomID, a.Parity)
}

// occupy отмечает, что ресурс id (группа, преподаватель или аудитория) занят в slot в
// недели parity; если он уже был занят в другую неделю — теперь занят в обе.
func occupy(occupied map[domain.TimeSlot]map[string]domain.Parity, slot domain.TimeSlot, id string, parity domain.Parity) {
	if occupied[slot] == nil {
		occupied[slot] = make(map[string]domain.Parity)
	}
	occupied[slot][id] = mergeParity(occupied[slot][id], parity)
}

// constructByTeacher — построение по преподавателям: от самых ограниченных к самым
// гибким, пары преподавателя ставятся подряд.
func constructByTeacher(draft *scheduleDraft, rng *rand.Rand) {
	teachers := orderTeachersByFlexibility(draft)
	if rng != nil {
		shuffleWithinBuckets(teachers, rng, func(t domain.Teacher) float64 {
			return teacherFlexibility(draft, t)
		})
	}

	for _, t := range teachers {
		draft.logger.Info("processing teacher", "id", t.ID, "name", t.Name, "max_hours", t.MaxWeeklyHours)
		tasks := collectRemaining(draft, draft.input, t)
		if rng != nil {
			shuffleTasksWithinPriority(tasks, rng)
		}
		for _, task := range tasks {
			placeTask(draft, task)
		}
	}
}

// placePair ставит одну пару задачи task: в допустимый слот с наименьшим штрафом, в
// лучшую по вместимости аудиторию, и возвращает этот слот. Состав плана не делится
// (ADR-0002): если общего свободного слота у преподавателя, всех групп и аудитории нет —
// false, пара остаётся непоставленной.
func placePair(draft *scheduleDraft, task placementTask) (domain.TimeSlot, bool) {
	slot, ok := findBestSlot(draft, task)
	if !ok {
		return slot, false
	}
	room := findBestRoom(draft, task, slot)
	if room == nil {
		return slot, false
	}
	addAssignment(draft, domain.Assignment{
		GroupIDs:   task.subject.GroupIDs,
		TeacherID:  task.subject.TeacherID,
		RoomID:     room.ID,
		SubjectID:  task.subject.ID,
		Type:       task.classType,
		TimeSlot:   slot,
		Parity:     task.parity,
		BuildingID: room.BuildingID,
	})
	return slot, true
}

// findBestSlot — допустимый слот (slotFeasible) с наименьшим штрафом slotPenalty.
// Дни перебираются от менее загруженных к более загруженным, суббота последней; при
// равном штрафе побеждает слот, найденный раньше, — так пары расходятся по неделе.
func findBestSlot(draft *scheduleDraft, task placementTask) (domain.TimeSlot, bool) {
	var best domain.TimeSlot
	bestPenalty, found := 0, false
	for _, day := range daysByLoad(draft) {
		for pairNum := domain.FirstPair; pairNum <= domain.LastPair; pairNum++ {
			slot := domain.MustNewTimeSlot(day, pairNum)
			if !slotFeasible(draft, task, slot) {
				continue
			}
			penalty := slotPenalty(draft, slot, task.subject, task.classType, task.parity, task.subject.GroupIDs, task.teacher.ID)
			if !found || penalty < bestPenalty {
				best, bestPenalty, found = slot, penalty, true
			}
		}
	}
	return best, found
}

// daysByLoad — дни от менее загруженных парами к более загруженным, суббота всегда последней.
func daysByLoad(draft *scheduleDraft) []domain.Day {
	load := make(map[domain.Day]int)
	for _, a := range draft.assignments {
		load[a.TimeSlot.Day()]++
	}
	days := append([]domain.Day(nil), domain.AllDays...)
	sort.SliceStable(days, func(i, j int) bool {
		if days[i] == domain.Saturday || days[j] == domain.Saturday {
			return days[j] == domain.Saturday && days[i] != domain.Saturday
		}
		return load[days[i]] < load[days[j]]
	})
	return days
}

// slotPenalty вычисляет штраф за постановку занятия в slot для групп groupIDs.
func slotPenalty(draft *scheduleDraft, slot domain.TimeSlot, subject domain.SubjectPlan, classType domain.ClassType, parity domain.Parity,
	groupIDs []string, teacherID string) int {
	day := slot.Day()

	satPenalty := 0
	if day == domain.Saturday {
		satPenalty = 25000
		for _, gid := range groupIDs {
			satCount := 0
			for _, a := range draft.assignments {
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

	// Окна, загрузка дня, переходы и нагрузка преподавателя — по каждой учебной неделе,
	// в которую идёт пара, и в среднем по двум неделям, как в итоговой оценке. Пара «только
	// по чётным» меняет только чётную неделю; поставленная туда, где у группы по нечётным
	// уже стоит другая пара, она закрывает чётной неделе дыру — так получается «мигалка».
	weekSum := 0
	for _, week := range []domain.Parity{domain.Even, domain.Odd} {
		if inWeek(parity, week) {
			weekSum += weekSlotPenalty(draft, slot, subject, groupIDs, day, teacherID, week)
		}
	}

	// Глобальный штраф за перегруженный день
	globalSpread := totalPairsInDay(draft, day) * 20

	prefPenalty := preferenceSlotPenalty(draft, subject, classType, slot, groupIDs)

	return satPenalty + weekSum/2 + globalSpread + prefPenalty
}

// weekSlotPenalty — штраф слота для групп и преподавателя в одну учебную неделю:
// окна, загрузка дня, переходы между корпусами, перегрузка преподавателя.
func weekSlotPenalty(draft *scheduleDraft, slot domain.TimeSlot, subject domain.SubjectPlan,
	groupIDs []string, day domain.Day, teacherID string, week domain.Parity) int {

	gapPenalty := 0
	groupLoadPenalty := 0
	buildingPenalty := 0

	// Корпус нового занятия определяется required_building_id или корпусом группы
	newBuilding := subject.RequiredBuildingID

	for _, gid := range groupIDs {
		existing := groupPairsInDay(draft, gid, day, week)
		n := len(existing)
		if n > 0 {
			all := append(existing, slot.PairNum())
			gapPenalty += calcGaps(all)
			sort.Ints(all)
			// HC8: окно в 2+ пары подряд — только если другого слота нет.
			groupLoadPenalty += longGapsIn(all) * longGapPenalty
		}
		// Пустой день дороже дня с одной парой: иначе построение раскидывает первые пары
		// группы по разным дням и само создаёт дни с единственной парой, которые потом
		// локальный поиск не может собрать без окон.
		switch {
		case n >= 4:
			groupLoadPenalty += 50000
		case n == 3:
			groupLoadPenalty += 15000
		case n == 2:
			groupLoadPenalty += 1000
		case n == 1:
			groupLoadPenalty += 0
		default:
			groupLoadPenalty += 1500
		}

		// Штраф за переход между корпусами. Физкультура не штрафуется: переход в зал
		// или на стадион для неё обычен и заложен в само занятие.
		if newBuilding != "" && !isSportRoomType(subject.RequiresRoomType) {
			buildingPenalty += calcBuildingTransitionPenalty(draft, gid, day, slot.PairNum(), newBuilding, week)
		}
	}

	// Нагрузка преподавателя за день: 3–4 пары — норма, лёгкое предпочтение разнести
	// занятия по неделе. Пятая пара — перегрузка, её избегаем почти любой ценой.
	teacherDayPairs := teacherPairsInDay(draft, teacherID, day, week)
	teacherSpread := len(teacherDayPairs) * 100
	if len(teacherDayPairs) >= teacherMaxPairsPerDay {
		teacherSpread += 20000
	}

	return gapPenalty*10000 + groupLoadPenalty + teacherSpread + buildingPenalty
}

// isSportRoomType — спортзал или открытая площадка. Физкультура проходит там, где решит
// преподаватель, и переход на неё из учебного корпуса не считается нарушением.
func isSportRoomType(roomType string) bool {
	return roomType == "gym" || roomType == "outdoor"
}

// calcBuildingTransitionPenalty начисляет штраф если новое занятие (pairNum, building)
// стоит вплотную или через одно окно к уже поставленным парам группы в другом корпусе.
func calcBuildingTransitionPenalty(draft *scheduleDraft, gid string, day domain.Day, pairNum int, newBuilding string, week domain.Parity) int {
	penalty := 0
	for _, a := range draft.assignments {
		if a.TimeSlot.Day() != day || !inWeek(a.Parity, week) {
			continue
		}
		found := false
		for _, agid := range a.GroupIDs {
			if agid == gid {
				found = true
				break
			}
		}
		if !found || a.BuildingID == newBuilding || isSportRoomType(draft.roomMap[a.RoomID].Type) {
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
func totalPairsInDay(draft *scheduleDraft, day domain.Day) int {
	count := 0
	for _, a := range draft.assignments {
		if a.TimeSlot.Day() == day {
			count++
		}
	}
	return count
}

// teacherPairsInDay — номера пар преподавателя в указанный день учебной недели week.
func teacherPairsInDay(draft *scheduleDraft, teacherID string, day domain.Day, week domain.Parity) []int {
	var pairs []int
	for _, a := range draft.assignments {
		if a.TeacherID == teacherID && a.TimeSlot.Day() == day && inWeek(a.Parity, week) {
			pairs = append(pairs, a.TimeSlot.PairNum())
		}
	}
	return pairs
}

// groupPairsInDay — номера пар группы в указанный день учебной недели week.
func groupPairsInDay(draft *scheduleDraft, groupID string, day domain.Day, week domain.Parity) []int {
	var pairs []int
	for _, a := range draft.assignments {
		if a.TimeSlot.Day() != day || !inWeek(a.Parity, week) {
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
func findBestRoom(draft *scheduleDraft, task placementTask, slot domain.TimeSlot) *domain.Room {
	subject, groupIDs, parity, teacher := task.subject, task.subject.GroupIDs, task.parity, task.teacher
	totalStudents := 0
	for _, gid := range groupIDs {
		if g, ok := draft.groupMap[gid]; ok {
			totalStudents += g.StudentCount
		}
	}

	var best *domain.Room
	bestDelta := -1
	for _, room := range draft.input.Rooms {
		if !isRoomSuitable(room, subject.RequiresRoomType) {
			continue
		}
		if !isSlotFree(slot, room.ID, parity, draft.occupiedRooms) {
			continue
		}
		ok, _ := isRoomBigEnoughWithOverflow(room, groupIDs, draft.groupMap)
		if !ok {
			continue
		}
		if !isRoomValidForSubject(room, subject, groupIDs, draft.groupMap, teacher) {
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
func orderTeachersByFlexibility(draft *scheduleDraft) []domain.Teacher {
	teachers := make([]domain.Teacher, len(draft.input.Teachers))
	copy(teachers, draft.input.Teachers)

	subjectCount := make(map[string]int)
	for _, sp := range draft.input.SubjectPlans {
		subjectCount[sp.TeacherID]++
	}

	sort.Slice(teachers, func(i, j int) bool {
		fi, fj := teacherFlexibility(draft, teachers[i]), teacherFlexibility(draft, teachers[j])
		if fi != fj {
			return fi < fj
		}
		return subjectCount[teachers[i].ID] > subjectCount[teachers[j].ID]
	})
	return teachers
}

// teacherFlexibility — доступных пар в неделю на один нужный час. Меньше = жёстче.
// Используется и в основной сортировке, и в bucketing для случайных перемешиваний.
func teacherFlexibility(draft *scheduleDraft, t domain.Teacher) float64 {
	const totalSlots = 36 // 6 дней × 6 пар

	need := 0
	for _, sp := range draft.input.SubjectPlans {
		if sp.TeacherID == t.ID {
			need += sp.LectureHours + sp.PracticeHours + sp.LabHours
		}
	}
	if need <= 0 {
		return 1e9 // без нагрузки — максимально гибкий, в конец
	}
	// Пара на другом факультете «всегда» занимает слот целиком, по чётности — наполовину.
	external := 0.0
	for _, ep := range t.ExternalPairs {
		if ep.Parity == domain.Even || ep.Parity == domain.Odd {
			external += 0.5
		} else {
			external++
		}
	}
	available := float64(totalSlots-len(t.UnavailableSlots)) - external
	if available <= 0 {
		return 0
	}
	return available / float64(need)
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

// getSubjectsForTeacher — учебные планы преподавателя в порядке входных данных.
func getSubjectsForTeacher(input domain.InputData, teacherID string) []domain.SubjectPlan {
	var plans []domain.SubjectPlan
	for _, sp := range input.SubjectPlans {
		if sp.TeacherID == teacherID {
			plans = append(plans, sp)
		}
	}
	return plans
}

// collectRemaining — пары этого преподавателя, которые ещё не поставлены. Порядок:
// одиночные лекции → практики → лабы. Внутри типа — предметы с большим потоком идут раньше.
func collectRemaining(draft *scheduleDraft, input domain.InputData, teacher domain.Teacher) []placementTask {
	subjects := getSubjectsForTeacher(input, teacher.ID)
	sortSubjectsByPriority(subjects)

	var tasks []placementTask
	for _, classType := range []domain.ClassType{domain.Lecture, domain.Practice, domain.Lab} {
		for _, sp := range subjects {
			remaining := getRemainingHours(draft, sp, classType)
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

// placeTask ставит одну пару задачи, если по плану она ещё нужна, и возвращает её слот.
// Не получилось — пишет в журнал, пара попадёт в отчёт «Не размещено».
func placeTask(draft *scheduleDraft, task placementTask) (domain.TimeSlot, bool) {
	if getRemainingHours(draft, task.subject, task.classType) <= 0 {
		return domain.TimeSlot{}, false
	}
	slot, ok := placePair(draft, task)
	if !ok {
		draft.logger.Warn("cannot place",
			"subject", task.subject.ID,
			"type", task.classType,
			"parity", task.parity,
			"teacher", task.teacher.ID,
			"groups", len(task.subject.GroupIDs),
		)
	}
	return slot, ok
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
func getRemainingHours(draft *scheduleDraft, subject domain.SubjectPlan, classType domain.ClassType) int {
	current := 0
	if draft.subjectCount[subject.ID] != nil {
		current = draft.subjectCount[subject.ID][classType]
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

// allSubjectsPlacedTeacher — проверка всех предметов.
// Мультигрупповые практики/лабы не требуются (best-effort).
func allSubjectsPlacedTeacher(draft *scheduleDraft) bool {
	for _, plan := range draft.input.SubjectPlans {
		for _, ct := range []domain.ClassType{domain.Lecture, domain.Practice, domain.Lab} {
			if ct != domain.Lecture && len(plan.GroupIDs) > 1 {
				continue
			}
			if getRemainingHours(draft, plan, ct) > 0 {
				return false
			}
		}
	}
	return true
}
