package solver

import (
	"log/slog"
	"sync"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// solveMultiStart запускает solveOnce opt.Starts раз параллельно с разными зёрнами и
// возвращает расписание с наименьшим score. Первый запуск — без зерна (детерминированный),
// чтобы результат был не хуже обычного. Бюджет каждому запуску — полный: они идут
// одновременно, поэтому вызывающий должен закладывать запас на конкуренцию за процессор.
func solveMultiStart(input domain.InputData, opt Options) (*domain.Schedule, error) {
	// Зёрна считаются от одного time.Now(): близкие UnixNano у горутин дали бы одинаковые
	// перемешивания. Множитель — простое число, чтобы зёрна расходились.
	base := time.Now().UnixNano()
	schedules := make([]*domain.Schedule, opt.Starts)
	errs := make([]error, opt.Starts)

	var wg sync.WaitGroup
	for i := range opt.Starts {
		run := opt
		run.Seed = 0
		if i > 0 {
			run.Seed = base + int64(i)*1_000_003
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			schedules[i], errs[i] = solveOnce(input, run)
		}()
	}
	wg.Wait()

	var best *domain.Schedule
	var scores []int
	for i, s := range schedules {
		if errs[i] != nil {
			continue
		}
		scores = append(scores, s.Score)
		if best == nil || s.Score < best.Score {
			best = s
		}
	}
	if best == nil {
		return nil, errs[0] // все запуски упали — показываем причину первого
	}
	slog.Default().Info("multistart complete", "starts", opt.Starts, "best_score", best.Score, "scores", scores)
	return best, nil
}
