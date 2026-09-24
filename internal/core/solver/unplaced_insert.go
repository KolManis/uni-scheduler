package solver

import (
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// insertMaxBlockers — сколько поставленных пар можно временно снять, чтобы освободить
// слот для непоставленной. Больше — шире поиск, но дороже и выше риск не вернуть снятые.
const insertMaxBlockers = 3

// insertUnplaced пытается поставить пары, которые не поместились при построении.
//
// Построение ставит пары по одной и место, выбранное раньше, не пересматривает: пара,
// которой не хватило общего окна у преподавателя, групп и аудитории, уходила в
// «Не размещено» навсегда. Улучшение двигало только поставленные пары, а score за
// непоставленные не штрафует, так что освобождать им место было некому.
//
// Для каждой непоставленной пары (трудные первыми):
//  1. лучший по score свободный слот пн–пт, затем суббота;
//  2. если свободного нет — вытеснение: в слоте снимаются мешающие пары (тот же
//     преподаватель или группы, не больше insertMaxBlockers), пара ставится туда,
//     снятые переставляются в лучшие свободные слоты. Ход принимается, только если
//     удалось вернуть всех снятых; из удачных выбирается лучший по score.
//
// Поставить пару важнее, чем сохранить score: вставка может его ухудшить.
func insertUnplaced(assignments []domain.Assignment, input domain.InputData,
	unavail teacherUnavailable) []domain.Assignment {

	pending := pendingAssignments(assignments, input)
	if len(pending) == 0 {
		return assignments
	}
	e := newEvaluatorWithPending(assignments, pending, input, unavail)

	var todo []int
	for i := len(assignments); i < len(e.asg); i++ {
		if len(e.info[i].cands) > 0 {
			todo = append(todo, i)
		}
	}
	options := make(map[int]int, len(todo))
	for _, i := range todo {
		for s := 0; s < numSlots; s++ {
			if e.fits(i, s, e.info[i].room) {
				options[i]++
			}
		}
	}
	sort.SliceStable(todo, func(a, b int) bool { return options[todo[a]] < options[todo[b]] })

	for _, i := range todo {
		if placeBest(e, i, 0, saturdayIdx*6) || placeBest(e, i, saturdayIdx*6, numSlots) {
			continue
		}
		insertWithEjection(e, i)
	}
	return e.assignments()
}

// pendingAssignments — непоставленные пары как заготовки назначений без слота.
func pendingAssignments(assignments []domain.Assignment, input domain.InputData) []domain.Assignment {
	plans := make(map[string]domain.SubjectPlan, len(input.SubjectPlans))
	for _, sp := range input.SubjectPlans {
		plans[sp.ID] = sp
	}
	var out []domain.Assignment
	for _, item := range ComputeUnplaced(assignments, input) {
		sp := plans[item.SubjectID]
		parity := sp.Parity
		if parity == "" {
			parity = domain.Always
		}
		for h := 0; h < item.MissingHours; h += 2 {
			out = append(out, domain.Assignment{
				GroupIDs:  sp.GroupIDs,
				TeacherID: sp.TeacherID,
				SubjectID: sp.ID,
				Type:      item.Type,
				Parity:    parity,
			})
		}
	}
	return out
}

// insertWithEjection ставит снятую пару i в слот, вытесняя мешающие пары.
func insertWithEjection(e *evaluator, i int) bool {
	start := e.snapshot()
	var best []move
	bestScore := 0

	for s := 0; s < numSlots; s++ {
		if busy := e.unavail[e.info[i].teacher][s]; (busy[0] && e.info[i].weeks[0]) || (busy[1] && e.info[i].weeks[1]) {
			continue
		}
		blockers := blockersAt(e, i, s)
		if len(blockers) == 0 || len(blockers) > insertMaxBlockers {
			continue
		}
		unplace := make([]move, len(blockers))
		for k, j := range blockers {
			unplace[k] = move{j, unplacedSlot, e.info[j].room}
		}
		e.apply(unplace)

		ok := false
		if _, placed := e.relocate(i, s); placed {
			ok = true
			for _, j := range blockers {
				if !placeBest(e, j, 0, saturdayIdx*6) && !placeBest(e, j, saturdayIdx*6, numSlots) {
					ok = false
					break
				}
			}
		}
		if ok {
			if sc := e.score(); best == nil || sc < bestScore {
				best, bestScore = e.snapshot(), sc
			}
		}
		e.restore(start)
	}

	if best == nil {
		return false
	}
	e.restore(best)
	return true
}

// blockersAt — поставленные пары в слоте s, которые делят с парой i преподавателя
// или группу в одну и ту же неделю.
func blockersAt(e *evaluator, i, s int) []int {
	inf := &e.info[i]
	shares := func(j int) bool {
		o := &e.info[j]
		if !(inf.weeks[0] && o.weeks[0]) && !(inf.weeks[1] && o.weeks[1]) {
			return false
		}
		if o.teacher == inf.teacher {
			return true
		}
		for _, g := range inf.groups {
			for _, h := range o.groups {
				if g == h {
					return true
				}
			}
		}
		return false
	}

	seen := map[int]bool{}
	var out []int
	check := func(members []int) {
		for _, j := range members {
			if j != i && e.slot[j] == s && !seen[j] && shares(j) {
				seen[j] = true
				out = append(out, j)
			}
		}
	}
	check(e.teachers[inf.teacher].members)
	for _, g := range inf.groups {
		check(e.groups[g].members)
	}
	return out
}
