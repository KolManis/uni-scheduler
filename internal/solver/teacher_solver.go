package solver

import (
	"log/slog"
	"sort"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

type teacherState struct {
	input            schedule.InputData
	teacherMap       map[string]schedule.Teacher
	groupMap         map[string]schedule.Group
	roomMap          map[string]schedule.Room
	assignments      []schedule.Assignment
	subjectCount     map[string]map[schedule.ClassType]int
	occupiedGroups   map[schedule.TimeSlot]map[string]schedule.Parity
	occupiedTeachers map[schedule.TimeSlot]map[string]schedule.Parity
	occupiedRooms    map[schedule.TimeSlot]map[string]schedule.Parity
	teacherSlots     map[string][]schedule.TimeSlot
	logger           *slog.Logger
}

func newTeacherState(input schedule.InputData) *teacherState {
	tm := make(map[string]schedule.Teacher)
	for _, t := range input.Teachers {
		tm[t.ID] = t
	}
	gm := make(map[string]schedule.Group)
	for _, g := range input.Groups {
		gm[g.ID] = g
	}
	rm := make(map[string]schedule.Room)
	for _, r := range input.Rooms {
		rm[r.ID] = r
	}
	return &teacherState{
		input:            input,
		teacherMap:       tm,
		groupMap:         gm,
		roomMap:          rm,
		assignments:      []schedule.Assignment{},
		subjectCount:     make(map[string]map[schedule.ClassType]int),
		occupiedGroups:   make(map[schedule.TimeSlot]map[string]schedule.Parity),
		occupiedTeachers: make(map[schedule.TimeSlot]map[string]schedule.Parity),
		occupiedRooms:    make(map[schedule.TimeSlot]map[string]schedule.Parity),
		teacherSlots:     make(map[string][]schedule.TimeSlot),
		logger:           slog.Default(),
	}
}

// SolveTeacher — teacher-driven подход
func SolveTeacher(input schedule.InputData, maxIter int) (*schedule.Schedule, error) {
	state := newTeacherState(input)

	// Сортируем преподавателей по сложности
	teachers := orderTeachersByComplexity(state)

	for _, teacher := range teachers {
		state.logger.Info("processing teacher", "id", teacher.ID, "name", teacher.Name, "max_hours", teacher.MaxWeeklyHours)

		// Получаем все предметы этого преподавателя
		subjects := getSubjectsForTeacher(input, teacher.ID)

		// Сортируем предметы
		sortSubjectsByPriority(subjects)

		// Генерируем слоты для каждого предмета с учётом чётности
		for _, subject := range subjects {
			if !subjectRemainingTeacher(state, subject) {
				continue
			}

			// Обрабатываем каждый тип занятий отдельно
			for _, classType := range []schedule.ClassType{schedule.Lecture, schedule.Practice, schedule.Lab} {
				remaining := getRemainingHours(state, subject, classType)
				if remaining <= 0 {
					continue
				}

				// Для каждого часа (пары) ищем слот
				for hoursPlaced := 0; hoursPlaced < remaining; hoursPlaced += 2 {
					// Определяем чётность для этого конкретного занятия
					parity := subject.Parity
					if parity == "" {
						parity = schedule.Always
					}

					// Ищем слот
					slot, room, found := findBestSlot(state, subject, classType, teacher, parity)
					if !found {
						state.logger.Warn("cannot place subject",
							"subject", subject.ID,
							"type", classType,
							"parity", parity,
							"teacher", teacher.ID)
						continue
					}

					assignTeacherSubject(state, subject, classType, *slot, room, parity)
				}
			}
		}
	}

	// Проверяем, все ли предметы распределены
	if !allSubjectsPlacedTeacher(state) {
		state.logger.Warn("not all subjects placed, using fallback solver")
		return fallbackSolve(state)
	}

	// post-processing: local search 2-opt
	improved := LocalSearch(state.assignments, state.input)
	score := calculateFitness(improved, state.input)

	state.logger.Info("teacher-driven solve complete",
		"assignments", len(improved),
		"score", score,
	)

	return &schedule.Schedule{
		Assignments: improved,
		Score:       score,
	}, nil
}

// findBestSlot — ищет лучший слот для занятия с day-aware выбором (без окон).
// Порядок дней: пн→вт→ср→чт→пт, суббота только если нет альтернатив.
func findBestSlot(state *teacherState, subject schedule.SubjectPlan, classType schedule.ClassType,
	teacher schedule.Teacher, parity schedule.Parity) (*schedule.TimeSlot, *schedule.Room, bool) {

	groupIDs := subject.GroupIDs

	type candidate struct {
		slot    schedule.TimeSlot
		room    schedule.Room
		penalty int // меньше = лучше
	}

	var candidates []candidate

	// Сортируем дни по текущей загрузке (меньше пар → пробуем первым)
	// Это гарантирует равномерный разброс по неделе
	dayLoad := make(map[schedule.Day]int)
	for _, a := range state.assignments {
		dayLoad[a.TimeSlot.Day]++
	}
	orderedDays := []schedule.Day{
		schedule.Monday, schedule.Tuesday, schedule.Wednesday,
		schedule.Thursday, schedule.Friday, schedule.Saturday,
	}
	sort.SliceStable(orderedDays, func(i, j int) bool {
		di, dj := orderedDays[i], orderedDays[j]
		if di == schedule.Saturday {
			return false
		}
		if dj == schedule.Saturday {
			return true
		}
		return dayLoad[di] < dayLoad[dj]
	})

	for _, day := range orderedDays {
		for pairNum := 1; pairNum <= 6; pairNum++ {
			slot := schedule.TimeSlot{Day: day, PairNum: pairNum}

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
func slotPenalty(state *teacherState, slot schedule.TimeSlot, subject schedule.SubjectPlan,
	groupIDs []string, day schedule.Day, teacherID string) int {

	satPenalty := 0
	if day == schedule.Saturday {
		satPenalty = 25000
		for _, gid := range groupIDs {
			satCount := 0
			for _, a := range state.assignments {
				if a.TimeSlot.Day != schedule.Saturday {
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
			all := append(existing, slot.PairNum)
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
			buildingPenalty += calcBuildingTransitionPenalty(state, gid, day, slot.PairNum, newBuilding)
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
func calcBuildingTransitionPenalty(state *teacherState, gid string, day schedule.Day, pairNum int, newBuilding string) int {
	penalty := 0
	for _, a := range state.assignments {
		if a.TimeSlot.Day != day {
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
		diff := pairNum - a.TimeSlot.PairNum
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
func totalPairsInDay(state *teacherState, day schedule.Day) int {
	count := 0
	for _, a := range state.assignments {
		if a.TimeSlot.Day == day {
			count++
		}
	}
	return count
}

// teacherPairsInDay возвращает номера пар преподавателя в указанный день.
func teacherPairsInDay(state *teacherState, teacherID string, day schedule.Day) []int {
	var pairs []int
	for _, a := range state.assignments {
		if a.TeacherID == teacherID && a.TimeSlot.Day == day {
			pairs = append(pairs, a.TimeSlot.PairNum)
		}
	}
	return pairs
}

// groupPairsInDay возвращает номера пар группы в указанный день.
func groupPairsInDay(state *teacherState, groupID string, day schedule.Day) []int {
	var pairs []int
	for _, a := range state.assignments {
		if a.TimeSlot.Day != day {
			continue
		}
		for _, gid := range a.GroupIDs {
			if gid == groupID {
				pairs = append(pairs, a.TimeSlot.PairNum)
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
func findBestRoom(state *teacherState, subject schedule.SubjectPlan, slot schedule.TimeSlot,
	groupIDs []string, parity schedule.Parity, teacher schedule.Teacher) *schedule.Room {
	totalStudents := 0
	for _, gid := range groupIDs {
		if g, ok := state.groupMap[gid]; ok {
			totalStudents += g.StudentCount
		}
	}

	var best *schedule.Room
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

// orderTeachersByComplexity — сортировка преподавателей
func orderTeachersByComplexity(state *teacherState) []schedule.Teacher {
	teachers := make([]schedule.Teacher, len(state.input.Teachers))
	copy(teachers, state.input.Teachers)

	subjectCount := make(map[string]int)
	for _, sp := range state.input.SubjectPlans {
		subjectCount[sp.TeacherID]++
	}

	sort.Slice(teachers, func(i, j int) bool {
		if teachers[i].MaxWeeklyHours != teachers[j].MaxWeeklyHours {
			return teachers[i].MaxWeeklyHours < teachers[j].MaxWeeklyHours
		}
		return subjectCount[teachers[i].ID] > subjectCount[teachers[j].ID]
	})
	return teachers
}

// sortSubjectsByPriority — сортировка предметов
func sortSubjectsByPriority(subjects []schedule.SubjectPlan) {
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

// assignTeacherSubject — назначение предмета
func assignTeacherSubject(state *teacherState, subject schedule.SubjectPlan, classType schedule.ClassType,
	slot schedule.TimeSlot, room *schedule.Room, parity schedule.Parity) {

	hours := 2

	assignment := schedule.Assignment{
		GroupIDs:   subject.GroupIDs,
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
		state.subjectCount[subject.ID] = make(map[schedule.ClassType]int)
	}
	state.subjectCount[subject.ID][classType] += hours

	for _, gid := range subject.GroupIDs {
		if state.occupiedGroups[slot] == nil {
			state.occupiedGroups[slot] = make(map[string]schedule.Parity)
		}
		state.occupiedGroups[slot][gid] = mergeParity(state.occupiedGroups[slot][gid], parity)
	}

	if state.occupiedTeachers[slot] == nil {
		state.occupiedTeachers[slot] = make(map[string]schedule.Parity)
	}
	state.occupiedTeachers[slot][subject.TeacherID] = mergeParity(state.occupiedTeachers[slot][subject.TeacherID], parity)

	if state.occupiedRooms[slot] == nil {
		state.occupiedRooms[slot] = make(map[string]schedule.Parity)
	}
	state.occupiedRooms[slot][room.ID] = mergeParity(state.occupiedRooms[slot][room.ID], parity)

	state.teacherSlots[subject.TeacherID] = append(state.teacherSlots[subject.TeacherID], slot)
}

// mergeParity — объединение чётностей
func mergeParity(existing, new schedule.Parity) schedule.Parity {
	if existing == "" {
		return new
	}
	if existing == schedule.Always || new == schedule.Always {
		return schedule.Always
	}
	if existing != new {
		return schedule.Always
	}
	return existing
}

// getRemainingHours — оставшиеся часы
func getRemainingHours(state *teacherState, subject schedule.SubjectPlan, classType schedule.ClassType) int {
	current := 0
	if state.subjectCount[subject.ID] != nil {
		current = state.subjectCount[subject.ID][classType]
	}
	var total int
	switch classType {
	case schedule.Lecture:
		total = subject.LectureHours
	case schedule.Practice:
		total = subject.PracticeHours
	case schedule.Lab:
		total = subject.LabHours
	}
	return total - current
}

// subjectRemainingTeacher — проверка остатка.
// Мультигрупповые практики/лабы не обязательны (могут не влезть в лаб. аудитории).
func subjectRemainingTeacher(state *teacherState, subject schedule.SubjectPlan) bool {
	for _, ct := range []schedule.ClassType{schedule.Lecture, schedule.Practice, schedule.Lab} {
		if ct != schedule.Lecture && len(subject.GroupIDs) > 1 {
			continue
		}
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
		for _, ct := range []schedule.ClassType{schedule.Lecture, schedule.Practice, schedule.Lab} {
			if ct != schedule.Lecture && len(plan.GroupIDs) > 1 {
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
func fallbackSolve(state *teacherState) (*schedule.Schedule, error) {
	state.logger.Info("using fallback parallel solver")
	return SolveParallel(state.input, 100000, 4)
}
