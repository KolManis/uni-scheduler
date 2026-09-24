package solver

// Вход в солвер: Solve и настройки Options. Весь путь от входных данных до расписания —
// в solveOnce, по шагам; каждый шаг — функция в своём файле (карта пакета — doc.go).

import (
	"log/slog"
	"math/rand"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// Construction — алгоритм построения начального расписания.
type Construction string

const (
	ConstructTeacher Construction = "teacher" // по преподавателям, от самых ограниченных
	ConstructDSatur  Construction = "dsatur"  // самая трудная пара первой (DSatur, раскраска графа; по умолчанию)
)

// ParseConstruction — алгоритм построения по значению solver_type. Пустое — DSatur:
// с ним улучшение даёт лучший и самый стабильный результат (ADR-0019).
func ParseConstruction(s string) (Construction, bool) {
	switch Construction(s) {
	case "", ConstructDSatur:
		return ConstructDSatur, true
	case ConstructTeacher:
		return ConstructTeacher, true
	}
	return "", false
}

// Options — как составлять расписание. Нулевые значения — разумные умолчания.
type Options struct {
	Construction Construction     // алгоритм построения; пусто — DSatur
	Improve      ImproveAlgorithm // метод улучшения; пусто — iterated local search
	Budget       time.Duration    // время на улучшение; 0 — localSearchTotalBudget (60 с)
	// Seed — 0: построение детерминированное; иначе пары с равным приоритетом
	// перемешиваются этим зерном (для нескольких запусков).
	Seed int64
	// Starts — сколько запусков с разными зёрнами сделать параллельно и взять лучший;
	// 0 и 1 — один запуск.
	Starts int
	// Fixed — закреплённые пары: стоят заранее и не двигаются, построение ставит остальные
	// вокруг них, их часы засчитываются в план.
	Fixed []domain.Assignment
}

// Solve составляет расписание: построение, вставка непоставленных, улучшение.
// Непоставленные пары не ошибка: они видны через ComputeUnplaced.
func Solve(input domain.InputData, opt Options) (*domain.Schedule, error) {
	if opt.Construction == "" {
		opt.Construction = ConstructDSatur
	}
	if opt.Starts > 1 {
		return solveMultiStart(input, opt)
	}
	return solveOnce(input, opt)
}

// solveOnce — один запуск. Весь путь от входных данных до расписания — здесь, по шагам.
func solveOnce(input domain.InputData, opt Options) (*domain.Schedule, error) {
	input = normalizeInput(input) // порядок справочников не должен влиять на результат (ADR-0008)
	unavail := buildTeacherUnavailable(input)
	budget := opt.Budget
	if budget <= 0 {
		budget = localSearchTotalBudget
	}
	deadline := time.Now().Add(budget)

	// 1. Построение: пары ставятся по одной, каждая — в лучший на этот момент слот.
	pairs := construct(input, opt)
	// 2. Пары, не поместившиеся при построении, — ещё попытка, с вытеснением соседей.
	pairs = insertUnplaced(pairs, input, unavail)
	// 3. Сходимость: простые перестановки, пока расписание улучшается.
	pairs = converge(pairs, input, deadline, unavail)
	// 4. Улучшение выбранным методом — выход из локального оптимума.
	pairs = improve(opt.Improve, pairs, input, deadline, unavail)
	// 5. Улучшение могло освободить место — последняя попытка поставить оставшиеся пары.
	pairs = insertUnplaced(pairs, input, unavail)

	score := calculateFitness(pairs, input)
	slog.Default().Info("solve complete",
		"construction", opt.Construction, "improve", opt.Improve,
		"assignments", len(pairs), "score", score)
	return &domain.Schedule{Assignments: pairs, Score: score}, nil
}

// construct — начальное расписание выбранным алгоритмом, вокруг закреплённых пар.
// Непоставленные пары не ошибка: они попадут в отчёт «Не размещено».
func construct(input domain.InputData, opt Options) []domain.Assignment {
	draft := newDraft(input)
	for _, a := range opt.Fixed {
		placeFixed(draft, a)
	}

	var rng *rand.Rand // nil — без перемешивания, построение детерминированное
	if opt.Seed != 0 {
		rng = rand.New(rand.NewSource(opt.Seed))
	}
	if opt.Construction == ConstructDSatur {
		constructDSatur(draft, rng)
	} else {
		constructByTeacher(draft, rng)
	}

	if !allSubjectsPlaced(draft) {
		draft.logger.Warn("not all subjects placed, continuing; see unplaced report")
	}
	return draft.assignments
}
