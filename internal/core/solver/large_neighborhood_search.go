package solver

import (
	"math/rand"
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Параметры LNS.
const (
	lnsDestroyMin   = 0.05 // доля снимаемых пар: минимум…
	lnsDestroyMax   = 0.20 // …и максимум, выбирается случайно на каждой итерации
	lnsMaxNoImprove = 300  // остановиться, если столько итераций подряд без улучшения
)

// largeNeighborhoodSearch — разрушение и восстановление (ruin & recreate).
//
// Локальный поиск двигает одну-две пары за шаг и не видит улучшений, требующих
// согласованного переноса нескольких пар. LNS:
//
//  1. Destroy: снимает часть пар целиком, освобождая их слоты. Набор выбирается
//     либо случайно, либо «связанно» — все пары группы, соседних с ней по потокам,
//     или все пары преподавателя: так освобождается место, где пары мешают друг другу.
//     Прицельно: связанный набор от группы, у которой сейчас есть день с одной парой, —
//     такой день не убрать одним ходом, нужно переставить её пары вместе с потоками.
//  2. Repair: снятые пары ставятся заново по одной, первыми — у которых меньше всего
//     допустимых слотов; каждой — лучший по полному score слот пн–пт (суббота — только
//     если в будни места нет) и аудитория.
//  3. Converge поверх восстановленного.
//
// Результат принимается, если он не хуже лучшего; иначе возврат к лучшему.
// Раньше «разрушение» переносило пары по одной, не освобождая их слотов заранее,
// — по сути это был тот же or-opt.
func largeNeighborhoodSearch(assignments []domain.Assignment, input domain.InputData,
	run *runBudget, rng *rand.Rand, unavail teacherUnavailable) []domain.Assignment {

	e := newEvaluator(assignments, input, unavail)
	if len(e.pairs) < 2 {
		return e.assignments()
	}
	best := e.snapshot()
	bestScore := e.score()

	noImprove := 0
	for noImprove < lnsMaxNoImprove && run.nextRound() {
		ruined := chooseRuin(e, rng)
		if len(ruined) == 0 {
			break
		}
		unplace := make([]move, len(ruined))
		for k, i := range ruined {
			unplace[k] = move{i, unplacedSlot, e.info[i].room}
		}
		e.apply(unplace)

		recreated := recreate(e, ruined)
		if recreated {
			convergeEval(e, run.innerDeadline())
		}
		if !run.roundDone() {
			break // время вышло посреди раунда — он отбрасывается (ADR-0023)
		}
		if !recreated {
			e.restore(best)
			noImprove++
			continue
		}

		switch s := e.score(); {
		case s < bestScore:
			best, bestScore, noImprove = e.snapshot(), s, 0
		case s == bestScore:
			best = e.snapshot()
			noImprove++
		default:
			e.restore(best)
			noImprove++
		}
	}
	e.restore(best)
	return e.assignments()
}

// chooseRuin — какие пары снять на этой итерации.
func chooseRuin(e *evaluator, rng *rand.Rand) []int {
	movable := 0
	for i := range e.pairs {
		if !e.pairs[i].Pinned {
			movable++
		}
	}
	if movable == 0 {
		return nil
	}
	frac := lnsDestroyMin + rng.Float64()*(lnsDestroyMax-lnsDestroyMin)
	ruin := newRuinSet(min(movable, max(1, int(float64(len(e.pairs))*frac))))

	switch rng.Intn(4) {
	case 0:
		if len(e.groups) > 0 {
			addLinkedGroups(e, rng.Intn(len(e.groups)), ruin)
		}
	case 1:
		addWholeTeachers(e, rng, ruin)
	case 2:
		if g, ok := groupWithSingleDay(e, rng); ok {
			addLinkedGroups(e, g, ruin)
		}
	}
	for !ruin.full() {
		ruin.add(e, rng.Intn(len(e.pairs)))
	}
	return ruin.pairs
}

// ruinAndRecreate — одно прицельное разрушение-восстановление для толчка в ILS: снять
// пары группы g и связанных с ней потоками (5–20% расписания) и поставить заново.
// false — какую-то пару поставить некуда; расписание тогда надо вернуть к снимку.
func ruinAndRecreate(e *evaluator, rng *rand.Rand, g int) bool {
	frac := lnsDestroyMin + rng.Float64()*(lnsDestroyMax-lnsDestroyMin)
	ruin := newRuinSet(max(1, int(float64(len(e.pairs))*frac)))
	addLinkedGroups(e, g, ruin)
	if len(ruin.pairs) == 0 {
		return true
	}
	unplace := make([]move, len(ruin.pairs))
	for k, i := range ruin.pairs {
		unplace[k] = move{i, unplacedSlot, e.info[i].room}
	}
	e.apply(unplace)
	return recreate(e, ruin.pairs)
}

// ruinSet — пары, которые снимаются на этой итерации LNS (не больше limit, без закреплённых).
type ruinSet struct {
	pairs  []int
	picked map[int]bool
	limit  int
}

func newRuinSet(limit int) *ruinSet {
	return &ruinSet{picked: make(map[int]bool, limit), limit: limit}
}

func (r *ruinSet) full() bool { return len(r.pairs) >= r.limit }

// add добавляет пару i, если она ещё не выбрана, не закреплена и место есть.
func (r *ruinSet) add(e *evaluator, i int) {
	if !r.picked[i] && !e.pairs[i].Pinned && !r.full() {
		r.picked[i] = true
		r.pairs = append(r.pairs, i)
	}
}

// groupWithSingleDay — случайная группа, у которой сейчас есть день с одной парой, который
// можно убрать. Если у группы за неделю всего одна пара, день с одной парой неизбежен
// при любом расписании (та же проверка, что в ExplainScore), — такая неделя не в счёт.
func groupWithSingleDay(e *evaluator, rng *rand.Rand) (int, bool) {
	var found []int
	for g := range e.groups {
		for w := 0; w < 2; w++ {
			if e.groups[g].penalty[w].SingleClassDay > 0 && e.pairsInWeek(g, w) > 1 {
				found = append(found, g)
				break
			}
		}
	}
	if len(found) == 0 {
		return 0, false
	}
	return found[rng.Intn(len(found))], true
}

// pairsInWeek — сколько поставленных пар у группы g в неделю w.
func (e *evaluator) pairsInWeek(g, w int) int {
	n := 0
	for _, i := range e.groups[g].members {
		if e.slot[i] != unplacedSlot && e.info[i].weeks[w] {
			n++
		}
	}
	return n
}

// addLinkedGroups — пары группы start и групп, связанных с ней общими потоками
// (обход в ширину), пока набор не заполнится.
func addLinkedGroups(e *evaluator, start int, ruin *ruinSet) {
	queue := []int{start}
	seen := map[int]bool{queue[0]: true}
	for len(queue) > 0 && !ruin.full() {
		g := queue[0]
		queue = queue[1:]
		for _, i := range e.groups[g].members {
			ruin.add(e, i)
			for _, other := range e.info[i].groups {
				if !seen[other] {
					seen[other] = true
					queue = append(queue, other)
				}
			}
		}
	}
}

// addWholeTeachers — все пары случайных преподавателей (до 10 попыток), пока набор не заполнится.
func addWholeTeachers(e *evaluator, rng *rand.Rand, ruin *ruinSet) {
	for tries := 0; !ruin.full() && tries < 10 && len(e.teachers) > 0; tries++ {
		for _, i := range e.teachers[rng.Intn(len(e.teachers))].members {
			ruin.add(e, i)
		}
	}
}

// recreate ставит снятые пары обратно жадно, трудные первыми. false — если какую-то
// пару поставить некуда (тогда вызывающий откатывается к лучшему решению).
func recreate(e *evaluator, ruined []int) bool {
	options := make(map[int]int, len(ruined))
	for _, i := range ruined {
		for s := 0; s < numSlots; s++ {
			if e.fits(i, s, e.info[i].room) {
				options[i]++
			}
		}
	}
	sort.SliceStable(ruined, func(a, b int) bool { return options[ruined[a]] < options[ruined[b]] })

	for _, i := range ruined {
		if !placeBest(e, i, 0, saturdayIdx*6) && !placeBest(e, i, saturdayIdx*6, numSlots) {
			return false
		}
	}
	return true
}

// placeBest ставит снятую пару i в лучший слот из [from, to). false — если ни один не подошёл.
func placeBest(e *evaluator, i, from, to int) bool {
	var bestMove move
	bestScore, found := 0, false
	for s := from; s < to; s++ {
		undo, ok := e.relocate(i, s)
		if !ok {
			continue
		}
		if sc := e.score(); !found || sc < bestScore {
			bestMove, bestScore, found = move{i, s, e.info[i].room}, sc, true
		}
		e.apply(undo)
	}
	if !found {
		return false
	}
	_, ok := e.apply([]move{bestMove})
	return ok
}
