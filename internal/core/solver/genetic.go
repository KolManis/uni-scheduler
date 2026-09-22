package solver

import (
	"math/rand"
	"sort"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Параметры генетического алгоритма.
const (
	gaPopulationSize   = 16
	gaTournamentSize   = 3
	gaMutationsPerKid  = 3 // сколько случайных возмущений применить к ребёнку
	gaMaxGenerations   = 500
	gaGensWithoutBest  = 60 // остановиться, если столько поколений подряд best не улучшился
	gaInitPerturbKicks = 6  // сколько случайных ходов для получения разнообразной популяции
)

type individual struct {
	assignments []domain.Assignment
	score       int
}

// geneticAlgorithm — популяционная эвристика по типу «пропорционального отбора Голланда».
//
// Отличие от SA/Tabu: работает не с одним решением, а с популяцией из gaPopulationSize
// расписаний. На каждом поколении отбирает двух родителей турнирным методом,
// делает кроссовер (точка разреза + починка через converge), мутирует ребёнка
// несколькими случайными ходами. Если ребёнок валиден и лучше худшего в популяции —
// вытесняет его. Элитарность — лучший не заменяется никогда.
func geneticAlgorithm(seed []domain.Assignment, input domain.InputData,
	deadline time.Time, unavail teacherUnavailable) []domain.Assignment {

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	pop := initialPopulation(seed, input, rng, unavail)
	sortPopulation(pop)

	bestScore := pop[0].score
	noImprove := 0

	for gen := 0; gen < gaMaxGenerations && noImprove < gaGensWithoutBest; gen++ {
		if time.Now().After(deadline) {
			break
		}

		parentA := tournamentSelect(pop, rng)
		parentB := tournamentSelect(pop, rng)
		child, ok := crossover(parentA, parentB, rng, unavail)
		if !ok {
			continue
		}
		mutate(child, rng, unavail)
		if !checkHardConstraints(child, unavail) {
			continue
		}

		childScore := calculateFitness(child, input)
		// Если ребёнок лучше худшего — заменяем худшего; элита (лучший) сохраняется.
		worst := &pop[len(pop)-1]
		if childScore < worst.score {
			worst.assignments = child
			worst.score = childScore
			sortPopulation(pop)
		}

		if pop[0].score < bestScore {
			bestScore = pop[0].score
			noImprove = 0
		} else {
			noImprove++
		}
	}

	// Финальный converge лучшей особи — часто GA оставляет незашлифованный результат.
	best := converge(pop[0].assignments, input, deadline, unavail)
	return best
}

// initialPopulation создаёт разнообразный стартовый набор: одна особь — seed целиком
// (гарантирует, что качество не упадёт ниже стартового), остальные — seed, потрёпанный
// случайными валидными ходами. Разнообразие нужно, иначе кроссовер даёт клонов.
func initialPopulation(seed []domain.Assignment, input domain.InputData,
	rng *rand.Rand, unavail teacherUnavailable) []individual {

	pop := make([]individual, gaPopulationSize)
	pop[0] = individual{assignments: cloneAssignments(seed), score: calculateFitness(seed, input)}

	for i := 1; i < gaPopulationSize; i++ {
		variant := cloneAssignments(seed)
		for k := 0; k < gaInitPerturbKicks; k++ {
			nb, ok := randomValidNeighbor(variant, rng, unavail)
			if ok {
				variant = nb
			}
		}
		pop[i] = individual{assignments: variant, score: calculateFitness(variant, input)}
	}
	return pop
}

func sortPopulation(pop []individual) {
	sort.Slice(pop, func(i, j int) bool { return pop[i].score < pop[j].score })
}

// tournamentSelect выбирает gaTournamentSize случайных особей и возвращает лучшую из них.
// Даёт большее давление отбора при большем размере турнира.
func tournamentSelect(pop []individual, rng *rand.Rand) individual {
	best := pop[rng.Intn(len(pop))]
	for k := 1; k < gaTournamentSize; k++ {
		cand := pop[rng.Intn(len(pop))]
		if cand.score < best.score {
			best = cand
		}
	}
	return best
}

// crossover — одноточечный кроссовер по временным слотам:
// ребёнок берёт слоты из родителя A до точки cut, из родителя B после.
// Роомы и группы сохраняются от seed-расписания (они у обоих родителей одинаковые,
// потому что стартовая популяция получена перестановкой одного и того же построения).
// Если результат нарушает HC — возвращает (nil, false), вызывающая сторона попробует другую пару.
func crossover(a, b individual, rng *rand.Rand, unavail teacherUnavailable) ([]domain.Assignment, bool) {
	if len(a.assignments) != len(b.assignments) {
		return nil, false
	}
	n := len(a.assignments)
	if n < 2 {
		return cloneAssignments(a.assignments), true
	}

	cut := 1 + rng.Intn(n-1)
	child := make([]domain.Assignment, n)
	for i := 0; i < cut; i++ {
		child[i] = a.assignments[i]
	}
	for i := cut; i < n; i++ {
		child[i] = a.assignments[i]
		child[i].TimeSlot = b.assignments[i].TimeSlot
	}

	if !checkHardConstraints(child, unavail) {
		return nil, false
	}
	return child, true
}

// mutate — несколько случайных валидных ходов на ребёнке. Аналог биологической мутации.
// Без неё популяция быстро выродится в клонов лучшей особи и поиск встанет.
func mutate(child []domain.Assignment, rng *rand.Rand, unavail teacherUnavailable) {
	for k := 0; k < gaMutationsPerKid; k++ {
		nb, ok := randomValidNeighbor(child, rng, unavail)
		if !ok {
			return
		}
		copy(child, nb)
	}
}
