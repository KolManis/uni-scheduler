package solver

import (
	"math/rand"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

const (
	// localSearchTotalBudget — потолок на всю работу LocalSearch (сходимость и метаэвристику).
	// С инкрементальной оценкой (evaluator) сходимость на 362 парах занимает доли секунды,
	// остальное время достаётся метаэвристике. Отжиг использует бюджет целиком, остальные
	// методы останавливаются раньше по застою.
	localSearchTotalBudget = 60 * time.Second
	// localSearchMaxStagnation — если столько раундов iteratedLocalSearch подряд не дали
	// улучшения, прекращаем раньше срока (на маленьких расписаниях сходится почти мгновенно).
	localSearchMaxStagnation = 60
)

// teacherUnavailable — предвычисленные недоступные слоты преподавателей (HC7).
// Строится один раз на запуск LocalSearch: checkHardConstraints вызывается в горячем
// цикле O(n²) раз, и пересобирать карту на каждый вызов было бы непозволительно дорого.
// nil/пустая карта означает "ограничений нет" — проверка тогда ничего не стоит.
type teacherUnavailable map[string]map[domain.TimeSlot]bool

func buildTeacherUnavailable(input domain.InputData) teacherUnavailable {
	var m teacherUnavailable
	for _, t := range input.Teachers {
		if len(t.UnavailableSlots) == 0 {
			continue
		}
		if m == nil {
			m = make(teacherUnavailable)
		}
		slots := make(map[domain.TimeSlot]bool, len(t.UnavailableSlots))
		for _, s := range t.UnavailableSlots {
			slots[s] = true
		}
		m[t.ID] = slots
	}
	return m
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

// LocalSearch улучшает расписание в два этапа, уложившись в бюджете суммарно:
//  1. converge — детерминированные 2-opt/or-opt по очереди до сходимости.
//  2. Одна из мета-эвристик (algo) поверх сошедшегося результата: помогает
//     выбраться из локального оптимума, куда converge не пускает.
//
// budget == 0 — используется дефолт localSearchTotalBudget (60 сек). Для параллельных
// запусков (несколько горутин конкурируют за CPU) вызывающая сторона должна передавать
// увеличенный бюджет, иначе deadline срабатывает на converge и метаэвристика не успевает
// ни одной итерации сделать.
//
// Пустой algo эквивалентен ImproveHillClimb (значение по умолчанию, поведение как раньше).
func LocalSearch(assignments []domain.Assignment, input domain.InputData, algo ImproveAlgorithm, budget time.Duration) []domain.Assignment {
	if budget <= 0 {
		budget = localSearchTotalBudget
	}
	deadline := time.Now().Add(budget)
	unavail := buildTeacherUnavailable(input)
	current := converge(assignments, input, deadline, unavail)

	switch algo {
	case ImproveSimulatedAnnealing:
		current = simulatedAnnealing(current, input, deadline, unavail)
	case ImproveTabuSearch:
		current = tabuSearch(current, input, deadline, unavail)
	case ImproveGeneticAlgorithm:
		current = geneticAlgorithm(current, input, deadline, unavail)
	case ImproveLNS:
		current = largeNeighborhoodSearch(current, input, deadline, unavail)
	default: // ImproveHillClimb или пусто
		current = iteratedLocalSearch(current, input, deadline, unavail)
	}
	return current
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
		for i := range e.asg {
			if time.Now().After(deadline) {
				return
			}
			for j := i + 1; j < len(e.asg); j++ {
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
		for i := range e.asg {
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
	for i := range e.asg {
		if time.Now().After(deadline) {
			return
		}
		if e.slot[i] == unplacedSlot {
			continue
		}
		for _, r := range e.info[i].cands {
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
	n := len(e.asg)
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
			cands := e.info[i].cands
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
	s := make([]move, len(e.asg))
	for i := range e.asg {
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
func iteratedLocalSearch(assignments []domain.Assignment, input domain.InputData, deadline time.Time,
	unavail teacherUnavailable) []domain.Assignment {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	e := newEvaluator(assignments, input, unavail)
	best := e.snapshot()
	bestScore := e.score()

	stagnant := 0
	for stagnant < localSearchMaxStagnation && !time.Now().After(deadline) {
		kicks := 2 + rng.Intn(3) + stagnant/8
		for k := 0; k < kicks; k++ {
			randomMove(e, rng)
		}
		convergeEval(e, deadline)
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
	type key struct {
		slot   domain.TimeSlot
		id     string
		entity string // "teacher" | "group" | "room"
	}

	type paritySet struct {
		even, odd, always bool
	}

	occupied := make(map[key]*paritySet)

	conflicts := func(ps *paritySet, p domain.Parity) bool {
		switch p {
		case domain.Always:
			return ps.even || ps.odd || ps.always
		case domain.Even:
			return ps.always || ps.even
		case domain.Odd:
			return ps.always || ps.odd
		}
		return false
	}

	add := func(ps *paritySet, p domain.Parity) {
		switch p {
		case domain.Always:
			ps.always = true
		case domain.Even:
			ps.even = true
		case domain.Odd:
			ps.odd = true
		}
	}

	for _, a := range assignments {
		parity := a.Parity
		if parity == "" {
			parity = domain.Always
		}

		// HC7: слот не должен быть отмечен преподавателем как недоступный
		if len(unavail) > 0 {
			if slots, ok := unavail[a.TeacherID]; ok && slots[a.TimeSlot] {
				return false
			}
		}

		// HC1: преподаватель
		tk := key{slot: a.TimeSlot, id: a.TeacherID, entity: "teacher"}
		if ps, ok := occupied[tk]; ok && conflicts(ps, parity) {
			return false
		}
		if occupied[tk] == nil {
			occupied[tk] = &paritySet{}
		}
		add(occupied[tk], parity)

		// HC2: группы
		for _, gid := range a.GroupIDs {
			gk := key{slot: a.TimeSlot, id: gid, entity: "group"}
			if ps, ok := occupied[gk]; ok && conflicts(ps, parity) {
				return false
			}
			if occupied[gk] == nil {
				occupied[gk] = &paritySet{}
			}
			add(occupied[gk], parity)
		}

		// HC3: аудитория
		rk := key{slot: a.TimeSlot, id: a.RoomID, entity: "room"}
		if ps, ok := occupied[rk]; ok && conflicts(ps, parity) {
			return false
		}
		if occupied[rk] == nil {
			occupied[rk] = &paritySet{}
		}
		add(occupied[rk], parity)
	}
	return true
}
