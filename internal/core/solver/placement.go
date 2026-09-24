package solver

// Постановка одной пары при построении: выбрать слот с наименьшим штрафом (slotPenalty),
// выбрать аудиторию (findBestRoom), записать пару в черновик (addAssignment).

import (
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

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
			saturdayPairs := 0
			for _, a := range draft.assignments {
				if a.TimeSlot.Day() != domain.Saturday {
					continue
				}
				for _, agid := range a.GroupIDs {
					if agid == gid {
						saturdayPairs++
						break
					}
				}
			}
			if saturdayPairs == 0 {
				satPenalty += 40000
			}
		}
	}

	// Окна, загрузка дня, переходы и нагрузка преподавателя — по каждой учебной неделе,
	// в которую идёт пара, и в среднем по двум неделям (для выбора слота важно только сравнение). Пара «только
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
			gapPenalty += gapsBetween(all)
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
			buildingPenalty += buildingTransitionPenalty(draft, gid, day, slot.PairNum(), newBuilding, week)
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

// buildingTransitionPenalty начисляет штраф если новое занятие (pairNum, building)
// стоит вплотную или через одно окно к уже поставленным парам группы в другом корпусе.
func buildingTransitionPenalty(draft *scheduleDraft, gid string, day domain.Day, pairNum int, newBuilding string, week domain.Parity) int {
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

// gapsBetween — сколько пустых пар между первой и последней парой набора (порядок не важен).
func gapsBetween(pairs []int) int {
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

// placeTask ставит одну пару задачи, если по плану она ещё нужна, и возвращает её слот.
// Не получилось — пишет в журнал, пара попадёт в отчёт «Не размещено».
func placeTask(draft *scheduleDraft, task placementTask) (domain.TimeSlot, bool) {
	if remainingHours(draft, task.subject, task.classType) <= 0 {
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
