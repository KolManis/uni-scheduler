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
	gaMaxGenerations   = 2000
	gaGensWithoutBest  = 150 // остановиться, если столько поколений подряд best не улучшился
	gaInitPerturbKicks = 6   // сколько случайных ходов для получения разнообразной популяции
)

type individual struct {
	assignments []domain.Assignment
	score       int
}

// geneticAlgorithm — меметический алгоритм: популяция расписаний, турнирный отбор,
// одноточечный кроссовер по слотам, мутация случайными ходами и короткий локальный
// поиск (or-opt) каждого ребёнка. Ребёнок вытесняет худшую особь, если лучше неё;
// лучшая особь не заменяется никогда.
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
			noImprove++
			continue
		}
		e := newEvaluator(child, input, unavail)
		for k := 0; k < gaMutationsPerKid; k++ {
			randomMove(e, rng)
		}
		orOptPass(e, deadline)
		childScore := e.score()

		worst := &pop[len(pop)-1]
		if childScore < worst.score {
			worst.assignments = e.assignments()
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

	return converge(pop[0].assignments, input, deadline.Add(5*time.Second), unavail)
}

// initialPopulation: одна особь — seed целиком (качество не упадёт ниже стартового),
// остальные — seed после случайных допустимых ходов, иначе кроссовер даёт клонов.
func initialPopulation(seed []domain.Assignment, input domain.InputData,
	rng *rand.Rand, unavail teacherUnavailable) []individual {

	pop := make([]individual, gaPopulationSize)
	for i := range pop {
		e := newEvaluator(seed, input, unavail)
		if i > 0 {
			for k := 0; k < gaInitPerturbKicks; k++ {
				randomMove(e, rng)
			}
		}
		pop[i] = individual{assignments: e.assignments(), score: e.score()}
	}
	return pop
}

func sortPopulation(pop []individual) {
	sort.Slice(pop, func(i, j int) bool { return pop[i].score < pop[j].score })
}

// tournamentSelect выбирает gaTournamentSize случайных особей и возвращает лучшую из них.
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

// crossover — одноточечный кроссовер: до точки cut пары берутся из родителя A, после —
// слот и аудитория из родителя B. Если результат нарушает HC — (nil, false).
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
	copy(child[:cut], a.assignments[:cut])
	copy(child[cut:], b.assignments[cut:])

	if !checkHardConstraints(child, unavail) {
		return nil, false
	}
	return child, true
}
