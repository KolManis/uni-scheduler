package solver

import (
	"math/rand"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// constructDSatur — построение «самая трудная пара первой» (DSatur из раскраски графов).
//
// Расписание — раскраска графа: вершины — пары, цвета — слоты, рёбра — общий
// преподаватель или группа. DSatur на каждом шаге красит вершину с наименьшим числом
// оставшихся допустимых цветов. Здесь «цвет» — слот, в котором свободны преподаватель,
// все группы и хотя бы одна подходящая аудитория (с учётом чётности и недоступности).
//
// Отличие от построения по преподавателям: там пары идут блоками по преподавателю, и
// пары последних преподавателей получают остатки сетки, даже если они труднее пар тех,
// кто шёл раньше. Здесь порядок пересчитывается после каждой постановки по всем парам.
//
// При равном числе вариантов первыми идут пары с большим числом групп, затем с большей
// «степенью» (сколько других пар делят с ней преподавателя или группу), затем лекции.
// Слот для выбранной пары — тот же выбор с наименьшим штрафом, что и при построении
// по преподавателям (findBestSlot), поэтому качество дня оценивается одинаково.
//
// Ненулевой rng перемешивает пары с полностью равным приоритетом (многостартовый поиск).
func constructDSatur(state *teacherState, rng *rand.Rand) {
	var tasks []placementTask
	for _, t := range state.input.Teachers {
		tasks = append(tasks, collectRemaining(state, state.input, t)...)
	}
	if len(tasks) == 0 {
		return
	}

	// Степень вершины: сколько других пар делят преподавателя или хотя бы одну группу.
	byTeacher := map[string]int{}
	byGroup := map[string]int{}
	for _, t := range tasks {
		byTeacher[t.teacher.ID]++
		for _, g := range t.subject.GroupIDs {
			byGroup[g]++
		}
	}
	degree := make([]int, len(tasks))
	tie := make([]int, len(tasks))
	for k, t := range tasks {
		d := byTeacher[t.teacher.ID] - 1
		for _, g := range t.subject.GroupIDs {
			d += byGroup[g] - 1
		}
		degree[k] = d
		tie[k] = k
	}
	if rng != nil {
		rng.Shuffle(len(tie), func(a, b int) { tie[a], tie[b] = tie[b], tie[a] })
	}

	// feasible[k][s] — слот s ещё допустим для пары k. Постановка пары меняет занятость
	// только в своём слоте, поэтому после неё перепроверяется один слот у всех пар.
	feasible := make([][numSlots]bool, len(tasks))
	count := make([]int, len(tasks))
	for k := range tasks {
		for s := 0; s < numSlots; s++ {
			if slotFeasible(state, tasks[k], slotFromIndex(s)) {
				feasible[k][s] = true
				count[k]++
			}
		}
	}

	classRank := map[domain.ClassType]int{domain.Lecture: 0, domain.Practice: 1, domain.Lab: 2}
	harder := func(a, b int) bool {
		if count[a] != count[b] {
			return count[a] < count[b]
		}
		ga, gb := len(tasks[a].subject.GroupIDs), len(tasks[b].subject.GroupIDs)
		if ga != gb {
			return ga > gb
		}
		if degree[a] != degree[b] {
			return degree[a] > degree[b]
		}
		if ra, rb := classRank[tasks[a].classType], classRank[tasks[b].classType]; ra != rb {
			return ra < rb
		}
		return tie[a] < tie[b]
	}

	remaining := make([]int, len(tasks))
	for k := range remaining {
		remaining[k] = k
	}
	for len(remaining) > 0 {
		pick := 0
		for x := 1; x < len(remaining); x++ {
			if harder(remaining[x], remaining[pick]) {
				pick = x
			}
		}
		k := remaining[pick]
		remaining = append(remaining[:pick], remaining[pick+1:]...)

		task := tasks[k]
		before := len(state.assignments)
		placeTask(state, task)
		if len(state.assignments) == before {
			continue
		}
		slot := state.assignments[len(state.assignments)-1].TimeSlot
		s := slotIndex(slot)
		for _, other := range remaining {
			if feasible[other][s] && !slotFeasible(state, tasks[other], slot) {
				feasible[other][s] = false
				count[other]--
			}
		}
	}
}

// slotFeasible — можно ли сейчас поставить пару task в slot: преподаватель доступен и
// свободен, свободны все группы и есть свободная подходящая аудитория.
func slotFeasible(state *teacherState, task placementTask, slot domain.TimeSlot) bool {
	if !isTeacherAvailable(slot, task.teacher) ||
		!isSlotFree(slot, task.teacher.ID, task.parity, state.occupiedTeachers) ||
		!isSlotFreeForAllGroups(slot, task.subject.GroupIDs, task.parity, state.occupiedGroups) {
		return false
	}
	for _, room := range state.input.Rooms {
		if !isRoomSuitable(room, task.subject.RequiresRoomType) ||
			!isSlotFree(slot, room.ID, task.parity, state.occupiedRooms) {
			continue
		}
		if ok, _ := isRoomBigEnoughWithOverflow(room, task.subject.GroupIDs, state.groupMap); !ok {
			continue
		}
		if isRoomValidForSubject(room, task.subject, task.subject.GroupIDs, state.groupMap, task.teacher) {
			return true
		}
	}
	return false
}
