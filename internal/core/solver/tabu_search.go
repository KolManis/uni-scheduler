package solver

import (
	"math/rand"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Параметры табу-поиска.
const (
	tabuTenureMin      = 10    // сколько итераций пара не может вернуться в покинутый слот (минимум)
	tabuTenureSpread   = 10    // случайная добавка к сроку запрета: снижает зацикливание
	tabuNeighborsLimit = 100   // сколько соседей осматриваем на одной итерации
	tabuMaxNoImprove   = 30000 // остановиться, если столько итераций не улучшили best
)

// tabuSearch — табу-поиск Гловера.
//
// На каждой итерации осматривает tabuNeighborsLimit случайных соседей и переходит
// в лучшего, даже если это ухудшение. Запрет — по атрибуту «пара i в слоте s»: пара,
// покинувшая слот, не может вернуться в него несколько итераций. Раньше запрещались
// конкретные ходы (обмен i↔j), и возврат тем же путём через другой ход был разрешён.
// Запрет снимается, если ход даёт новый лучший результат (критерий стремления).
func tabuSearch(assignments []domain.Assignment, input domain.InputData,
	deadline time.Time, unavail teacherUnavailable) []domain.Assignment {

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	e := newEvaluator(assignments, input, unavail)
	best := e.snapshot()
	bestScore := e.score()

	tabuUntil := make([]int, len(e.asg)*numSlots)
	isTabu := func(undo []move, iter int) bool {
		for _, m := range undo {
			// После хода пара стоит в e.slot[m.i]; запрещено возвращаться туда, где она была раньше.
			if s := e.slot[m.i]; s != unplacedSlot && tabuUntil[m.i*numSlots+s] > iter {
				return true
			}
		}
		return false
	}

	noImprove := 0
	for iter := 1; noImprove < tabuMaxNoImprove && !time.Now().After(deadline); iter++ {
		var bestMove []move
		bestNeighbor := 0

		for k := 0; k < tabuNeighborsLimit; k++ {
			undo, ok := randomMove(e, rng)
			if !ok {
				continue
			}
			s := e.score()
			allowed := !isTabu(undo, iter) || s < bestScore
			if allowed && (bestMove == nil || s < bestNeighbor) {
				bestMove = forwardOf(e, undo)
				bestNeighbor = s
			}
			e.apply(undo)
		}
		if bestMove == nil {
			break
		}

		undo, ok := e.apply(bestMove)
		if !ok {
			continue
		}
		tenure := tabuTenureMin + rng.Intn(tabuTenureSpread+1)
		for _, m := range undo {
			if m.slot != unplacedSlot {
				tabuUntil[m.i*numSlots+m.slot] = iter + tenure
			}
		}

		if s := e.score(); s < bestScore {
			best = e.snapshot()
			bestScore = s
			noImprove = 0
		} else {
			noImprove++
		}
	}

	e.restore(best)
	convergeEval(e, deadline.Add(5*time.Second))
	return e.assignments()
}

// forwardOf — ход, который привёл к текущему состоянию, по его откату.
func forwardOf(e *evaluator, undo []move) []move {
	fwd := make([]move, len(undo))
	for k, m := range undo {
		fwd[k] = move{m.i, e.slot[m.i], e.info[m.i].room}
	}
	return fwd
}
