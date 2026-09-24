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
// по преподавателям (placePair), поэтому качество дня оценивается одинаково.
//
// Ненулевой rng перемешивает пары с полностью равным приоритетом (многостартовый поиск).
func constructDSatur(draft *scheduleDraft, rng *rand.Rand) {
	queue := newDSaturQueue(draft, rng)
	for !queue.empty() {
		task := queue.popHardest()
		if slot, ok := placeTask(draft, task); ok {
			queue.slotTaken(draft, slot)
		}
	}
}

// dsaturQueue — непоставленные пары и то, по чему выбирается самая трудная.
type dsaturQueue struct {
	tasks     []placementTask
	remaining []int            // номера ещё не поставленных пар в tasks
	feasible  [][numSlots]bool // feasible[k][s] — слот s ещё допустим для пары k
	free      []int            // сколько допустимых слотов осталось у пары
	degree    []int            // сколько других пар делят с ней преподавателя или группу
	tie       []int            // порядок при полном равенстве (перемешивается зерном)
}

func newDSaturQueue(draft *scheduleDraft, rng *rand.Rand) *dsaturQueue {
	q := &dsaturQueue{}
	for _, t := range draft.input.Teachers {
		q.tasks = append(q.tasks, collectRemaining(draft, draft.input, t)...)
	}
	n := len(q.tasks)
	q.remaining = make([]int, n)
	q.feasible = make([][numSlots]bool, n)
	q.free = make([]int, n)
	q.degree = taskDegrees(q.tasks)
	q.tie = make([]int, n)
	for k := range q.tasks {
		q.remaining[k] = k
		q.tie[k] = k
		for s := 0; s < numSlots; s++ {
			if slotFeasible(draft, q.tasks[k], slotFromIndex(s)) {
				q.feasible[k][s] = true
				q.free[k]++
			}
		}
	}
	if rng != nil {
		rng.Shuffle(n, func(a, b int) { q.tie[a], q.tie[b] = q.tie[b], q.tie[a] })
	}
	return q
}

// taskDegrees — степень вершины: сколько других пар делят преподавателя или хотя бы одну группу.
func taskDegrees(tasks []placementTask) []int {
	byTeacher := map[string]int{}
	byGroup := map[string]int{}
	for _, t := range tasks {
		byTeacher[t.teacher.ID]++
		for _, g := range t.subject.GroupIDs {
			byGroup[g]++
		}
	}
	degree := make([]int, len(tasks))
	for k, t := range tasks {
		degree[k] = byTeacher[t.teacher.ID] - 1
		for _, g := range t.subject.GroupIDs {
			degree[k] += byGroup[g] - 1
		}
	}
	return degree
}

func (q *dsaturQueue) empty() bool { return len(q.remaining) == 0 }

// popHardest достаёт из очереди самую трудную пару.
func (q *dsaturQueue) popHardest() placementTask {
	pick := 0
	for x := 1; x < len(q.remaining); x++ {
		if q.harder(q.remaining[x], q.remaining[pick]) {
			pick = x
		}
	}
	k := q.remaining[pick]
	q.remaining = append(q.remaining[:pick], q.remaining[pick+1:]...)
	return q.tasks[k]
}

// classRank — при прочем равенстве лекции раньше практик, практики раньше лабораторных.
var classRank = map[domain.ClassType]int{domain.Lecture: 0, domain.Practice: 1, domain.Lab: 2}

// harder — пара a трудней пары b: меньше свободных слотов, затем больше групп, затем
// больше степень, затем лекция раньше практики, затем случайный порядок tie.
func (q *dsaturQueue) harder(a, b int) bool {
	if q.free[a] != q.free[b] {
		return q.free[a] < q.free[b]
	}
	ga, gb := len(q.tasks[a].subject.GroupIDs), len(q.tasks[b].subject.GroupIDs)
	if ga != gb {
		return ga > gb
	}
	if q.degree[a] != q.degree[b] {
		return q.degree[a] > q.degree[b]
	}
	if ra, rb := classRank[q.tasks[a].classType], classRank[q.tasks[b].classType]; ra != rb {
		return ra < rb
	}
	return q.tie[a] < q.tie[b]
}

// slotTaken — в slot поставлена пара: у остальных пар этот слот мог стать недопустимым.
// Постановка меняет занятость только в своём слоте, поэтому перепроверяется только он.
func (q *dsaturQueue) slotTaken(draft *scheduleDraft, slot domain.TimeSlot) {
	s := slotIndex(slot)
	for _, k := range q.remaining {
		if q.feasible[k][s] && !slotFeasible(draft, q.tasks[k], slot) {
			q.feasible[k][s] = false
			q.free[k]--
		}
	}
}

// slotFeasible — можно ли сейчас поставить пару task в slot: преподаватель доступен и
// свободен, свободны все группы и есть свободная подходящая аудитория.
func slotFeasible(draft *scheduleDraft, task placementTask, slot domain.TimeSlot) bool {
	if !isTeacherAvailable(slot, task.teacher) ||
		!isSlotFree(slot, task.teacher.ID, task.parity, draft.occupiedTeachers) ||
		!isSlotFreeForAllGroups(slot, task.subject.GroupIDs, task.parity, draft.occupiedGroups) {
		return false
	}
	for _, room := range draft.input.Rooms {
		if !isRoomSuitable(room, task.subject.RequiresRoomType) ||
			!isSlotFree(slot, room.ID, task.parity, draft.occupiedRooms) {
			continue
		}
		if ok, _ := isRoomBigEnoughWithOverflow(room, task.subject.GroupIDs, draft.groupMap); !ok {
			continue
		}
		if isRoomValidForSubject(room, task.subject, task.subject.GroupIDs, draft.groupMap, task.teacher) {
			return true
		}
	}
	return false
}
