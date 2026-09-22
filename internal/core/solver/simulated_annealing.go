package solver

import (
	"math"
	"math/rand"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Параметры имитации отжига. Подобраны по классической рекомендации Кирпатрика:
// начальная температура T0 ≈ score / 10 (10% от текущего значения принимается как «шум»),
// охлаждение — геометрическое с коэффициентом 0.995 за итерацию,
// нижняя граница T_min — когда exp(-Δ/T) для типичного Δ становится пренебрежимо малой.
const (
	saCoolingRate    = 0.995
	saMinTemperature = 0.5
	saMaxNoImprove   = 3000 // остановиться, если столько итераций подряд не улучшили лучшее
)

// simulatedAnnealing — реализация классического отжига Кирпатрика.
//
// В отличие от iteratedLocalSearch, SA принимает ухудшающие ходы с вероятностью
// exp(-Δ/T), где T — «температура», падающая от итерации к итерации. Это даёт
// строго вероятностный (а не «случайный толчок каждые N раундов») способ
// вылезти из локального оптимума. При T → 0 SA становится обычным hill climbing.
func simulatedAnnealing(assignments []domain.Assignment, input domain.InputData,
	deadline time.Time, unavail teacherUnavailable) []domain.Assignment {

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	current := cloneAssignments(assignments)
	currentScore := calculateFitness(current, input)
	best := cloneAssignments(current)
	bestScore := currentScore

	// Начальная температура: 10% от текущего значения score гарантирует,
	// что ухудшающие ходы соизмеримые с типичным штрафом (~пары тысяч)
	// будут приниматься на первых итерациях с вероятностью ~0.9.
	T := float64(currentScore) / 10.0
	if T < 100 {
		T = 100
	}

	noImprove := 0
	for T > saMinTemperature && noImprove < saMaxNoImprove && !time.Now().After(deadline) {
		neighbor, ok := randomValidNeighbor(current, rng, unavail)
		if !ok {
			T *= saCoolingRate
			continue
		}
		neighborScore := calculateFitness(neighbor, input)
		delta := neighborScore - currentScore

		accept := false
		if delta < 0 {
			accept = true
		} else if rng.Float64() < math.Exp(-float64(delta)/T) {
			accept = true
		}

		if accept {
			current = neighbor
			currentScore = neighborScore
			if currentScore < bestScore {
				best = cloneAssignments(current)
				bestScore = currentScore
				noImprove = 0
			} else {
				noImprove++
			}
		} else {
			noImprove++
		}

		T *= saCoolingRate
	}

	// Финальный детерминированный проход — SA мог остановиться на не-локальном оптимуме.
	best = converge(best, input, deadline, unavail)
	return best
}

// randomValidNeighbor пытается вернуть одно валидное расписание-соседа:
// случайный swap или or-opt перенос, проверенный на HC1-3 и HC7.
// Возвращает (nil, false), если за 20 попыток ничего не нашлось.
func randomValidNeighbor(current []domain.Assignment, rng *rand.Rand,
	unavail teacherUnavailable) ([]domain.Assignment, bool) {

	if len(current) < 2 {
		return nil, false
	}
	for attempt := 0; attempt < 20; attempt++ {
		var cand []domain.Assignment
		if rng.Intn(2) == 0 {
			// swap: обмен слотами двух назначений
			i := rng.Intn(len(current))
			j := rng.Intn(len(current))
			if i == j || current[i].TimeSlot == current[j].TimeSlot {
				continue
			}
			cand = swapSlots(current, i, j)
		} else {
			// or-opt: перенос одного назначения в случайный слот Пн-Пт
			i := rng.Intn(len(current))
			day := domain.AllDays[rng.Intn(5)] // 0..4 = пн-пт
			pair := 1 + rng.Intn(6)
			newSlot := domain.MustNewTimeSlot(day, pair)
			if current[i].TimeSlot == newSlot {
				continue
			}
			cand = cloneAssignments(current)
			cand[i].TimeSlot = newSlot
		}
		if cand != nil && checkHardConstraints(cand, unavail) {
			return cand, true
		}
	}
	return nil, false
}

func cloneAssignments(a []domain.Assignment) []domain.Assignment {
	out := make([]domain.Assignment, len(a))
	copy(out, a)
	return out
}
