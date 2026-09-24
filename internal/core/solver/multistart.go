package solver

import (
	"log/slog"
	"sync"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// SolveTeacherMultiStart запускает SolveTeacherWithBudget `starts` раз параллельно
// с разными зёрнами случайности и возвращает лучший результат.
//
// budget == 0 — используется дефолтный localSearchTotalBudget. При параллельных запусках
// вызывающая сторона обязана передавать увеличенный бюджет, чтобы все N горутин успевали
// сделать метаэвристику под конкуренцией за CPU.
//
// starts == 0 или 1 — один запуск, как обычный SolveTeacher.
func SolveTeacherMultiStart(input domain.InputData, maxIter int, improve ImproveAlgorithm, starts int, budget time.Duration) (*domain.Schedule, error) {
	return SolveMultiStart(input, ConstructTeacher, maxIter, improve, starts, budget)
}

// SolveMultiStart — многостартовый поиск с выбранным алгоритмом построения.
func SolveMultiStart(input domain.InputData, construct Construction, maxIter int, improve ImproveAlgorithm, starts int, budget time.Duration) (*domain.Schedule, error) {
	if starts <= 1 {
		return SolveWithBudget(input, construct, maxIter, improve, 0, budget)
	}

	type result struct {
		sched *domain.Schedule
		err   error
		seed  int64
	}

	// seeds[0] = 0 → детерминированный запуск как безопасный baseline.
	// seeds[1..] → фиксированные, но разные значения; берём time.Now() один раз,
	// иначе несколько горутин с очень близкими UnixNano-зёрнами дадут одинаковые
	// перемешивания и весь смысл многостартовости пропадёт.
	seeds := make([]int64, starts)
	base := time.Now().UnixNano()
	seeds[0] = 0
	for i := 1; i < starts; i++ {
		seeds[i] = base + int64(i)*1_000_003 // множитель — простое число, чтобы зёрна расходились
	}

	results := make([]result, starts)
	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Add(1)
		go func(idx int, s int64) {
			defer wg.Done()
			sched, err := SolveWithBudget(input, construct, maxIter, improve, s, budget)
			results[idx] = result{sched: sched, err: err, seed: s}
		}(i, seed)
	}
	wg.Wait()

	// Выбираем лучший (минимальный score). Ошибки пропускаем, но если ВСЕ упали —
	// возвращаем первую попавшуюся, чтобы вызывающая сторона увидела причину.
	var best *domain.Schedule
	var firstErr error
	for _, r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		if best == nil || r.sched.Score < best.Score {
			best = r.sched
		}
	}
	if best == nil {
		return nil, firstErr
	}

	slog.Default().Info("multistart complete",
		"starts", starts,
		"best_score", best.Score,
		"scores", func() []int {
			out := make([]int, 0, len(results))
			for _, r := range results {
				if r.sched != nil {
					out = append(out, r.sched.Score)
				}
			}
			return out
		}(),
	)
	return best, nil
}
