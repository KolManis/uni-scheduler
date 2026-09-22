package solver

import (
	"math/rand"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Параметры LNS.
const (
	lnsDestroyFraction = 0.20 // сколько назначений «сносим» на одном шаге
	lnsMaxNoImprove    = 30   // остановиться, если столько итераций подряд без улучшения
)

// largeNeighborhoodSearch — «разрушение-восстановление» (Very Large Neighborhood Search).
//
// Классический локальный поиск (2-opt, or-opt) двигает одно назначение за шаг: если
// улучшить расписание можно только согласованным переносом нескольких пар, он этого
// увидеть не в состоянии. LNS решает это перебором «больших» окрестностей:
//
//  1. Destroy: 20% случайных пар откладываются в сторону — их слоты «стираются».
//  2. Repair: для каждой отложенной пары находится ЛУЧШИЙ слот с учётом ВСЕХ
//     остальных (не только тех, что были на построении). Оценка по полному fitness,
//     а не по частичному slotPenalty — здесь мы видим окна и перегрузки целиком.
//  3. Converge: обычный 2-opt/or-opt поверх восстановленного.
//
// Если получилось лучше — принимаем; иначе возвращаемся к best и пробуем снова.
func largeNeighborhoodSearch(assignments []domain.Assignment, input domain.InputData,
	deadline time.Time, unavail teacherUnavailable) []domain.Assignment {

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	best := cloneAssignments(assignments)
	bestScore := calculateFitness(best, input)
	noImprove := 0

	for noImprove < lnsMaxNoImprove && !time.Now().After(deadline) {
		candidate := cloneAssignments(best)

		// Destroy + repair: N случайных назначений двигаем в лучший из 36 слотов.
		n := len(candidate)
		destroyCount := int(float64(n) * lnsDestroyFraction)
		if destroyCount < 1 {
			destroyCount = 1
		}
		for k := 0; k < destroyCount; k++ {
			i := rng.Intn(n)
			candidate = repairOne(candidate, i, input, unavail)
			if time.Now().After(deadline) {
				break
			}
		}

		// Converge: доводим до локального оптимума.
		candidate = converge(candidate, input, deadline, unavail)
		score := calculateFitness(candidate, input)

		if score < bestScore {
			best = candidate
			bestScore = score
			noImprove = 0
		} else {
			noImprove++
		}
	}
	return best
}

// repairOne находит лучший слот для назначения i (перебирая все 36) с учётом текущего
// расписания. Возвращает копию с назначением в новом слоте либо исходную, если
// ни один слот не даёт улучшения.
func repairOne(assignments []domain.Assignment, i int, input domain.InputData,
	unavail teacherUnavailable) []domain.Assignment {

	best := assignments
	bestScore := calculateFitness(best, input)

	for _, day := range domain.AllDays {
		for pair := 1; pair <= 6; pair++ {
			slot := domain.MustNewTimeSlot(day, pair)
			if assignments[i].TimeSlot == slot {
				continue
			}
			cand := cloneAssignments(assignments)
			cand[i].TimeSlot = slot
			if !checkHardConstraints(cand, unavail) {
				continue
			}
			score := calculateFitness(cand, input)
			if score < bestScore {
				best = cand
				bestScore = score
			}
		}
	}
	return best
}
