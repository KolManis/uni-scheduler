package solver

import (
	"math/rand"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

const (
	// localSearchTotalBudget — бюджет по умолчанию на сходимость и улучшение (шаги 3–4 solveOnce).
	// С инкрементальной оценкой (evaluator) сходимость на 362 парах занимает доли секунды,
	// остальное время достаётся метаэвристике. Отжиг использует бюджет целиком, остальные
	// методы останавливаются раньше по застою.
	localSearchTotalBudget = 60 * time.Second
	// localSearchMaxStagnation — если столько раундов iteratedLocalSearch подряд не дали
	// улучшения, прекращаем раньше срока (на маленьких расписаниях сходится почти мгновенно).
	localSearchMaxStagnation = 60
)

// teacherUnavailable — предвычисленная занятость преподавателей извне (HC7): слот → в
// какие недели преподаватель занят. Недоступные слоты заняты всегда; пары на других
// факультетах — в свою чётность. Строится один раз на запуск (solveOnce):
// checkHardConstraints вызывается в горячем цикле, пересобирать карту на каждый вызов
// было бы дорого. nil/пустая карта означает "ограничений нет".
type teacherUnavailable map[string]map[domain.TimeSlot]domain.Parity

func buildTeacherUnavailable(input domain.InputData) teacherUnavailable {
	m := teacherUnavailable{}
	for _, t := range input.Teachers {
		for _, s := range t.UnavailableSlots {
			m.add(t.ID, s, domain.Always)
		}
		for _, ep := range t.ExternalPairs {
			m.add(t.ID, ep.TimeSlot, ep.Parity)
		}
	}
	if len(m) == 0 {
		return nil // ограничений нет — проверка ничего не стоит
	}
	return m
}

// add отмечает, что преподаватель занят в slot в недели parity (пусто — каждую неделю).
func (m teacherUnavailable) add(teacherID string, slot domain.TimeSlot, parity domain.Parity) {
	if m[teacherID] == nil {
		m[teacherID] = make(map[domain.TimeSlot]domain.Parity)
	}
	if parity == "" {
		parity = domain.Always
	}
	m[teacherID][slot] = mergeParity(m[teacherID][slot], parity)
}

// paritiesOverlap — идут ли пары с чётностями a и b хотя бы в одну общую неделю.
func paritiesOverlap(a, b domain.Parity) bool {
	return (inWeek(a, domain.Even) && inWeek(b, domain.Even)) || (inWeek(a, domain.Odd) && inWeek(b, domain.Odd))
}

// ImproveAlgorithm — какой мета-эвристикой улучшать расписание после детерминированной
// сходимости. Все варианты работают на одинаковом окружении (тот же converge, тот же
// checkHardConstraints, тот же fitness), различаются только стратегией выбора соседей.
type ImproveAlgorithm string

const (
	ImproveHillClimb          ImproveAlgorithm = "hillclimb" // iteratedLocalSearch со случайным perturb (по умолчанию)
	ImproveSimulatedAnnealing ImproveAlgorithm = "sa"        // имитация отжига: принимает ухудшающие ходы с падающей вероятностью
	ImproveTabuSearch         ImproveAlgorithm = "tabu"      // табу-поиск: избегает недавних ходов через память
	ImproveGeneticAlgorithm   ImproveAlgorithm = "ga"        // генетический: популяция расписаний с кроссовером
	ImproveLNS                ImproveAlgorithm = "lns"       // large neighborhood search: разрушение-восстановление
)

// improve — улучшение выбранным методом (ADR-0012) поверх сошедшегося расписания:
// каждый метод по-своему выбирается из локального оптимума, куда converge не пускает.
// Пусто или неизвестное значение — iterated local search.
// run — когда остановиться (по времени или по числу раундов, ADR-0023), rng — случайные
// ходы с записанным сидом, targeted — прицельное разрушение в ILS (ADR-0021).
func improve(algo ImproveAlgorithm, assignments []domain.Assignment, input domain.InputData,
	run *runBudget, rng *rand.Rand, unavail teacherUnavailable, targeted bool) []domain.Assignment {
	switch algo {
	case ImproveSimulatedAnnealing:
		return simulatedAnnealing(assignments, input, run.deadline, rng, unavail)
	case ImproveTabuSearch:
		return tabuSearch(assignments, input, run, rng, unavail)
	case ImproveGeneticAlgorithm:
		return geneticAlgorithm(assignments, input, run, rng, unavail)
	case ImproveLNS:
		return largeNeighborhoodSearch(assignments, input, run, rng, unavail)
	default:
		return iteratedLocalSearch(assignments, input, run, rng, unavail, targeted)
	}
}

// converge доводит расписание до локального оптимума (см. convergeEval).
func converge(assignments []domain.Assignment, input domain.InputData, deadline time.Time,
	unavail teacherUnavailable) []domain.Assignment {
	e := newEvaluator(assignments, input, unavail)
	convergeEval(e, deadline)
	return e.assignments()
}

// convergeEval крутит окрестности по очереди, пока очередной раунд даёт строгое
// улучшение score, но не дольше deadline:
//   - 2-opt — обмен слотами двух пар;
//   - or-opt — перенос пары в другой слот пн–пт (с подбором другой аудитории, если своя занята);
//   - смена аудитории — влияет на переходы между корпусами;
//   - обмен днями группы — все пары группы из одного дня переезжают в другой и наоборот.
//     Одиночные ходы такого не находят: каждый промежуточный шаг создаёт окно.
func convergeEval(e *evaluator, deadline time.Time) {
	for !time.Now().After(deadline) {
		before := e.score()
		twoOptPass(e, deadline)
		orOptPass(e, deadline)
		roomPass(e, deadline)
		groupDayPass(e, deadline)
		if e.score() >= before {
			return
		}
	}
}

// keepIfBetter оставляет применённый ход, если score стал меньше cur, иначе откатывает.
func keepIfBetter(e *evaluator, undo []move, cur int) (int, bool) {
	if s := e.score(); s < cur {
		return s, true
	}
	e.apply(undo)
	return cur, false
}

// twoOptPass — попарный обмен слотами до исчерпания улучшений.
func twoOptPass(e *evaluator, deadline time.Time) {
	cur := e.score()
	for improved := true; improved; {
		improved = false
		for i := range e.pairs {
			if time.Now().After(deadline) {
				return
			}
			for j := i + 1; j < len(e.pairs); j++ {
				if e.slot[i] == e.slot[j] || e.slot[i] == unplacedSlot || e.slot[j] == unplacedSlot {
					continue
				}
				undo, ok := e.swap(i, j)
				if !ok {
					continue
				}
				var better bool
				if cur, better = keepIfBetter(e, undo, cur); better {
					improved = true
				}
			}
		}
	}
}

// orOptPass — перенос одной пары в другой слот пн–пт до исчерпания улучшений.
func orOptPass(e *evaluator, deadline time.Time) {
	cur := e.score()
	for improved := true; improved; {
		improved = false
		for i := range e.pairs {
			if time.Now().After(deadline) {
				return
			}
			for s := 0; s < saturdayIdx*6; s++ {
				if s == e.slot[i] || e.slot[i] == unplacedSlot {
					continue
				}
				undo, ok := e.relocate(i, s)
				if !ok {
					continue
				}
				var better bool
				if cur, better = keepIfBetter(e, undo, cur); better {
					improved = true
				}
			}
		}
	}
}

// roomPass — пара остаётся в своём слоте, но переходит в другую допустимую аудиторию.
func roomPass(e *evaluator, deadline time.Time) {
	cur := e.score()
	for i := range e.pairs {
		if time.Now().After(deadline) {
			return
		}
		if e.slot[i] == unplacedSlot {
			continue
		}
		for _, r := range e.info[i].roomOptions {
			if r == e.info[i].room {
				continue
			}
			undo, ok := e.apply([]move{{i, e.slot[i], r}})
			if !ok {
				continue
			}
			cur, _ = keepIfBetter(e, undo, cur)
		}
	}
}

// groupDayMoves — ход «обмен днями»: пары группы g из дня d1 переезжают в d2 на те же
// номера пар и наоборот. nil — если в обоих днях у группы пусто.
func groupDayMoves(e *evaluator, g, d1, d2 int) []move {
	var moves []move
	for _, i := range e.groups[g].members {
		s := e.slot[i]
		if s == unplacedSlot {
			continue
		}
		switch s / 6 {
		case d1:
			moves = append(moves, move{i, d2*6 + s%6, e.info[i].room})
		case d2:
			moves = append(moves, move{i, d1*6 + s%6, e.info[i].room})
		}
	}
	return moves
}

func groupDayPass(e *evaluator, deadline time.Time) {
	cur := e.score()
	for g := range e.groups {
		if time.Now().After(deadline) {
			return
		}
		for d1 := 0; d1 < saturdayIdx; d1++ {
			for d2 := d1 + 1; d2 < saturdayIdx; d2++ {
				moves := groupDayMoves(e, g, d1, d2)
				if len(moves) == 0 {
					continue
				}
				undo, ok := e.apply(moves)
				if !ok {
					continue
				}
				cur, _ = keepIfBetter(e, undo, cur)
			}
		}
	}
}

// randomMove — случайный допустимый ход: обмен, перенос, смена аудитории или обмен
// днями группы. Возвращает откат; ok == false, если за 20 попыток ничего не нашлось.
func randomMove(e *evaluator, rng *rand.Rand) ([]move, bool) {
	n := len(e.pairs)
	if n < 2 {
		return nil, false
	}
	for attempt := 0; attempt < 20; attempt++ {
		i := rng.Intn(n)
		if e.slot[i] == unplacedSlot {
			continue
		}
		var undo []move
		var ok bool
		switch r := rng.Intn(20); {
		case r < 8:
			j := rng.Intn(n)
			if i == j || e.slot[i] == e.slot[j] || e.slot[j] == unplacedSlot {
				continue
			}
			undo, ok = e.swap(i, j)
		case r < 16:
			s := rng.Intn(saturdayIdx * 6)
			if s == e.slot[i] {
				continue
			}
			undo, ok = e.relocate(i, s)
		case r < 18:
			cands := e.info[i].roomOptions
			if len(cands) < 2 {
				continue
			}
			room := cands[rng.Intn(len(cands))]
			if room == e.info[i].room {
				continue
			}
			undo, ok = e.apply([]move{{i, e.slot[i], room}})
		default:
			if len(e.groups) == 0 {
				continue
			}
			g := rng.Intn(len(e.groups))
			d1, d2 := rng.Intn(saturdayIdx), rng.Intn(saturdayIdx)
			if d1 == d2 {
				continue
			}
			moves := groupDayMoves(e, g, d1, d2)
			if len(moves) == 0 {
				continue
			}
			undo, ok = e.apply(moves)
		}
		if ok {
			return undo, true
		}
	}
	return nil, false
}

// snapshot — слоты и аудитории всех пар; restore возвращает расписание к снимку.
func (e *evaluator) snapshot() []move {
	s := make([]move, len(e.pairs))
	for i := range e.pairs {
		s[i] = move{i, e.slot[i], e.info[i].room}
	}
	return s
}

func (e *evaluator) restore(snap []move) {
	if _, ok := e.apply(snap); !ok {
		panic("solver: снимок расписания нарушает жёсткие ограничения")
	}
}

// iteratedLocalSearch — iterated local search: толчок из нескольких случайных ходов,
// сходимость, и принимаем результат, если он не хуже лучшего. Равные принимаются, чтобы
// поиск мог двигаться по «плато» одинаковых score. Сила толчка растёт с застоем.
func iteratedLocalSearch(assignments []domain.Assignment, input domain.InputData, run *runBudget,
	rng *rand.Rand, unavail teacherUnavailable, targeted bool) []domain.Assignment {
	e := newEvaluator(assignments, input, unavail)
	best := e.snapshot()
	bestScore := e.score()

	stagnant := 0
	for stagnant < localSearchMaxStagnation && run.nextRound() {
		// Толчок: обычно несколько случайных ходов. Примерно каждый четвёртый раунд, если
		// у какой-то группы есть день с одной парой, — прицельное разрушение из LNS:
		// снять её пары вместе со связанными потоками и расставить заново. Случайными
		// ходами такой день почти не убирается (нужно сдвинуть сразу несколько групп).
		recreated := true
		if g, ok := groupWithSingleDay(e, rng); targeted && ok && rng.Intn(4) == 0 {
			recreated = ruinAndRecreate(e, rng, g)
		} else {
			kicks := 2 + rng.Intn(3) + stagnant/8
			for k := 0; k < kicks; k++ {
				randomMove(e, rng)
			}
		}
		if recreated {
			convergeEval(e, run.innerDeadline())
		}
		if !run.roundDone() {
			break // время вышло посреди раунда — он отбрасывается (ADR-0023)
		}
		if !recreated {
			e.restore(best)
			stagnant++
			continue
		}
		score := e.score()

		switch {
		case score < bestScore:
			stagnant = 0
		case score == bestScore:
			stagnant++
		default:
			e.restore(best)
			stagnant++
			continue
		}
		best = e.snapshot()
		bestScore = score
	}
	e.restore(best)
	return e.assignments()
}

// checkHardConstraints проверяет HC1–HC3 (двойная занятость преподавателя, группы, аудитории)
// и HC7 (недоступные слоты преподавателя) для набора назначений.
//
// HC7 проверять обязательно: перестановка меняет именно временной слот, поэтому занятие
// может уехать в слот, который преподаватель отметил как недоступный. Остальные ограничения
// (тип и вместимость аудитории, корпуса) завязаны на аудиторию, а она при обмене слотами
// не меняется — они были соблюдены при генерации и остаются соблюдёнными.
//
// unavail может быть nil — тогда ограничений по доступности нет и проверка бесплатна.
func checkHardConstraints(assignments []domain.Assignment, unavail teacherUnavailable) bool {
	// busy — в какие недели ресурс уже занят в слоте. Ресурс — "teacher:…", "group:…", "room:…".
	type resourceSlot struct {
		resource string
		slot     domain.TimeSlot
	}
	busy := make(map[resourceSlot]domain.Parity)

	for _, a := range assignments {
		parity := a.Parity
		if parity == "" {
			parity = domain.Always
		}

		// HC7: преподаватель не отметил слот недоступным и не ведёт в это время пару на другом факультете.
		if p, ok := unavail[a.TeacherID][a.TimeSlot]; ok && paritiesOverlap(p, parity) {
			return false
		}

		// HC1–HC3: преподаватель, каждая группа и аудитория не заняты в этот слот в ту же неделю.
		resources := []string{"teacher:" + a.TeacherID, "room:" + a.RoomID}
		for _, gid := range a.GroupIDs {
			resources = append(resources, "group:"+gid)
		}
		for _, r := range resources {
			k := resourceSlot{r, a.TimeSlot}
			if p, ok := busy[k]; ok && paritiesOverlap(p, parity) {
				return false
			}
			busy[k] = mergeParity(busy[k], parity)
		}
	}
	return true
}
