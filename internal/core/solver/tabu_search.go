package solver

import (
	"math/rand"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Параметры табу-поиска.
const (
	tabuTenure         = 20  // сколько последних ходов запрещены
	tabuNeighborsLimit = 100 // сколько соседей осматриваем на одной итерации
	tabuMaxNoImprove   = 200 // остановиться, если столько итераций не улучшили best
)

// tabuMove — ход, который запрещён на несколько итераций. Для swap это пара индексов,
// для or-opt — индекс назначения и его новый слот. Чтобы одинаково хранить оба вида,
// нормализуем: kind = 0 (swap) хранит (min(i,j), max(i,j), нулевой слот),
// kind = 1 (or-opt) — (i, -1, новый слот).
type tabuMove struct {
	kind byte
	a, b int
	slot domain.TimeSlot
}

// tabuSearch — реализация классического табу-поиска Гловера.
//
// На каждой итерации ищет ЛУЧШЕГО из ~100 соседей и переходит в него, даже если это
// временно ухудшает score. Только что сделанные ходы попадают в «табу-список»
// на tabuTenure итераций — их нельзя обратить, что заставляет поиск исследовать
// новые районы. Ход из списка разрешён, только если он даёт результат лучше
// best-so-far (aspiration criterion).
func tabuSearch(assignments []domain.Assignment, input domain.InputData,
	deadline time.Time, unavail teacherUnavailable) []domain.Assignment {

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	current := cloneAssignments(assignments)
	currentScore := calculateFitness(current, input)
	best := cloneAssignments(current)
	bestScore := currentScore

	tabu := make([]tabuMove, 0, tabuTenure)
	isTabu := func(m tabuMove) bool {
		for _, t := range tabu {
			if t == m {
				return true
			}
		}
		return false
	}
	pushTabu := func(m tabuMove) {
		if len(tabu) >= tabuTenure {
			tabu = tabu[1:]
		}
		tabu = append(tabu, m)
	}

	noImprove := 0
	for noImprove < tabuMaxNoImprove && !time.Now().After(deadline) {
		bestNeighbor := []domain.Assignment(nil)
		bestNeighborScore := 0
		var bestMove tabuMove

		for k := 0; k < tabuNeighborsLimit; k++ {
			cand, move, ok := randomValidNeighborWithMove(current, rng, unavail)
			if !ok {
				continue
			}
			candScore := calculateFitness(cand, input)

			// Aspiration: табу можно нарушить, если это даёт новый глобальный минимум.
			if isTabu(move) && candScore >= bestScore {
				continue
			}

			if bestNeighbor == nil || candScore < bestNeighborScore {
				bestNeighbor = cand
				bestNeighborScore = candScore
				bestMove = move
			}
		}

		if bestNeighbor == nil {
			break
		}

		current = bestNeighbor
		currentScore = bestNeighborScore
		pushTabu(bestMove)

		if currentScore < bestScore {
			best = cloneAssignments(current)
			bestScore = currentScore
			noImprove = 0
		} else {
			noImprove++
		}
	}

	// Финальный converge — табу-поиск мог оставить недошлифованное решение.
	best = converge(best, input, deadline, unavail)
	return best
}

// randomValidNeighborWithMove делает то же, что randomValidNeighbor, но заодно
// возвращает ход — нужен для табу-списка.
func randomValidNeighborWithMove(current []domain.Assignment, rng *rand.Rand,
	unavail teacherUnavailable) ([]domain.Assignment, tabuMove, bool) {

	if len(current) < 2 {
		return nil, tabuMove{}, false
	}
	for attempt := 0; attempt < 20; attempt++ {
		if rng.Intn(2) == 0 {
			i := rng.Intn(len(current))
			j := rng.Intn(len(current))
			if i == j || current[i].TimeSlot == current[j].TimeSlot {
				continue
			}
			if j < i {
				i, j = j, i
			}
			cand := swapSlots(current, i, j)
			if checkHardConstraints(cand, unavail) {
				return cand, tabuMove{kind: 0, a: i, b: j}, true
			}
		} else {
			i := rng.Intn(len(current))
			day := domain.AllDays[rng.Intn(5)]
			pair := 1 + rng.Intn(6)
			newSlot := domain.MustNewTimeSlot(day, pair)
			if current[i].TimeSlot == newSlot {
				continue
			}
			cand := cloneAssignments(current)
			cand[i].TimeSlot = newSlot
			if checkHardConstraints(cand, unavail) {
				return cand, tabuMove{kind: 1, a: i, slot: newSlot}, true
			}
		}
	}
	return nil, tabuMove{}, false
}
