package solver

import (
	"math"
	"math/rand"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Параметры имитации отжига.
const (
	saInitialAccept  = 0.3 // с такой вероятностью в начале принимается типичный ухудшающий ход
	saFinalTemp      = 3.0 // к концу бюджета принимаются только ухудшения порядка окна преподавателя
	saCalibrateMoves = 200 // сколько случайных ходов пробуем для подбора начальной температуры
)

// simulatedAnnealing — имитация отжига Кирпатрика.
//
// Ухудшающий ход принимается с вероятностью exp(-Δ/T). Начальная температура
// подбирается по данным: T₀ такая, что средний ухудшающий ход принимается с
// вероятностью saInitialAccept. Раньше T₀ = score/10: при score в сотни тысяч это
// «кипение», в котором принимается почти всё и первая половина бюджета уходит впустую.
//
// Охлаждение привязано ко времени: T падает геометрически от T₀ до saFinalTemp за весь
// бюджет, поэтому график не зависит от скорости машины. Если долго нет улучшения,
// поиск возвращается к лучшему решению (повторный нагрев не нужен: температура к этому
// моменту уже ниже, и поиск продолжает с лучшей точки).
func simulatedAnnealing(assignments []domain.Assignment, input domain.InputData,
	deadline time.Time, unavail teacherUnavailable) []domain.Assignment {

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	e := newEvaluator(assignments, input, unavail)
	if len(e.pairs) < 2 {
		return e.assignments()
	}
	current := e.score()
	best := e.snapshot()
	bestScore := current

	t0 := calibrateTemperature(e, rng)
	start := time.Now()
	total := deadline.Sub(start)
	if total <= 0 {
		return e.assignments()
	}
	restartAfter := 50 * len(e.pairs)

	T := t0
	noImprove := 0
	for iter := 0; ; iter++ {
		if iter&255 == 0 {
			elapsed := time.Since(start)
			if elapsed >= total {
				break
			}
			T = t0 * math.Pow(saFinalTemp/t0, float64(elapsed)/float64(total))
		}

		undo, ok := randomMove(e, rng)
		if !ok {
			continue
		}
		s := e.score()
		delta := s - current
		if delta <= 0 || rng.Float64() < math.Exp(-float64(delta)/T) {
			current = s
		} else {
			e.apply(undo)
		}

		if current < bestScore {
			best = e.snapshot()
			bestScore = current
			noImprove = 0
		} else if noImprove++; noImprove > restartAfter {
			e.restore(best)
			current = bestScore
			noImprove = 0
		}
	}

	e.restore(best)
	convergeEval(e, deadline.Add(5*time.Second))
	return e.assignments()
}

// calibrateTemperature — средний ухудшающий случайный ход, пересчитанный в температуру
// с вероятностью принятия saInitialAccept. Состояние после подбора не меняется.
func calibrateTemperature(e *evaluator, rng *rand.Rand) float64 {
	cur := e.score()
	sum, n := 0.0, 0
	for k := 0; k < saCalibrateMoves; k++ {
		undo, ok := randomMove(e, rng)
		if !ok {
			continue
		}
		if d := e.score() - cur; d > 0 {
			sum += float64(d)
			n++
		}
		e.apply(undo)
	}
	if n == 0 {
		return 100
	}
	t := sum / float64(n) / math.Log(1/saInitialAccept)
	return math.Max(t, saFinalTemp*2)
}

func cloneAssignments(a []domain.Assignment) []domain.Assignment {
	out := make([]domain.Assignment, len(a))
	copy(out, a)
	return out
}
