// Бенчмарк солвера на данных из БД: несколько прогонов каждого метода улучшения,
// медиана и разброс score плюс показатели качества. Результат — CSV в stdout.
//
//	go run ./cmd/bench -runs 5 -budget 60s -methods hillclimb,sa,lns > bench.csv
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/KolManis/uni-scheduler/internal/adapters/out/postgres"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

func main() {
	var (
		dsn     = flag.String("dsn", "postgres://postgres:postgres@127.0.0.1:5432/scheduler?sslmode=disable", "строка подключения")
		runs    = flag.Int("runs", 5, "прогонов на метод")
		budget  = flag.Duration("budget", 60*time.Second, "бюджет локального поиска на прогон")
		methods = flag.String("methods", "hillclimb,sa,tabu,ga,lns", "методы через запятую")
	)
	flag.Parse()

	ctx := context.Background()
	pool, err := postgres.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "подключение к БД:", err)
		os.Exit(1)
	}
	defer pool.Close()

	data, err := postgres.NewInputRepository(pool).LoadInput(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "загрузка входных данных:", err)
		os.Exit(1)
	}

	fmt.Println("method,run,score,group_gaps,single_days,saturday_pairs,max_group_day,unplaced,seconds")
	for _, method := range strings.Split(*methods, ",") {
		algo := solver.ImproveAlgorithm(strings.TrimSpace(method))
		scores := make([]int, 0, *runs)
		for run := 1; run <= *runs; run++ {
			start := time.Now()
			sched, err := solver.SolveTeacherWithBudget(*data, 50000, algo, 0, *budget)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s прогон %d: %v\n", algo, run, err)
				continue
			}
			q := solver.CalculateQuality(sched.Assignments)
			unplaced := len(solver.ComputeUnplaced(sched.Assignments, *data))
			fmt.Printf("%s,%d,%d,%d,%d,%d,%d,%d,%.0f\n", algo, run, sched.Score,
				q.GroupGaps, q.SingleClassDays, q.SaturdayPairs, q.MaxGroupPairsPerDay,
				unplaced, time.Since(start).Seconds())
			scores = append(scores, sched.Score)
		}
		if len(scores) > 0 {
			sort.Ints(scores)
			fmt.Fprintf(os.Stderr, "%s: медиана %d, мин %d, макс %d (%d прогонов)\n",
				algo, scores[len(scores)/2], scores[0], scores[len(scores)-1], len(scores))
		}
	}
}
