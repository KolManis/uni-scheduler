// Бенчмарк солвера на данных из БД: несколько прогонов каждого метода улучшения,
// медиана и разброс score плюс показатели качества. Результат — CSV в stdout.
//
//	go run ./cmd/bench -runs 5 -budget 60s -methods hillclimb,sa,lns > bench.csv
//	go run ./cmd/bench -snapshots data/snapshots -runs 3 -budget 30s   # без БД
//	go run ./cmd/bench -snapshots data/snapshots -construct teacher,dsatur -budget 1ns   # только построение
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/KolManis/uni-scheduler/internal/adapters/out/postgres"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/solver"
)

func main() {
	var (
		dsn     = flag.String("dsn", "postgres://postgres:postgres@127.0.0.1:5432/scheduler?sslmode=disable", "строка подключения")
		runs    = flag.Int("runs", 5, "прогонов на метод")
		budget  = flag.Duration("budget", 60*time.Second, "бюджет локального поиска на прогон")
		methods = flag.String("methods", "hillclimb,sa,tabu,ga,lns", "методы через запятую")
		builds  = flag.String("construct", "teacher", "алгоритмы построения через запятую: teacher, dsatur")
		snaps   = flag.String("snapshots", "", "каталог JSON-снимков справочников вместо БД (например, data/snapshots)")
		starts  = flag.Int("starts", 1, "параллельных запусков в одном прогоне, берётся лучший")
		noRuin  = flag.Bool("no-targeted", false, "ILS без прицельного разрушения (для сравнения, ADR-0021)")
	)
	flag.Parse()

	var data *domain.InputData
	var err error
	if *snaps != "" {
		data, err = loadSnapshots(*snaps)
	} else {
		data, err = loadFromDB(*dsn)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "загрузка входных данных:", err)
		os.Exit(1)
	}

	fmt.Println("construct,method,run,score,gaps_even,gaps_odd,long_gaps,single_even,single_odd,saturday_pairs,max_group_day,unplaced,seconds,rounds")
	for _, b := range strings.Split(*builds, ",") {
		construct, ok := solver.ParseConstruction(strings.TrimSpace(b))
		if !ok {
			fmt.Fprintln(os.Stderr, "неизвестный алгоритм построения:", b)
			os.Exit(1)
		}
		for _, method := range strings.Split(*methods, ",") {
			algo := solver.ImproveAlgorithm(strings.TrimSpace(method))
			scores := make([]int, 0, *runs)
			for run := 1; run <= *runs; run++ {
				start := time.Now()
				sched, err := solver.Solve(*data, solver.Options{Construction: construct, Improve: algo, Budget: *budget,
					Starts: *starts, NoTargetedRuin: *noRuin})
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s/%s прогон %d: %v\n", construct, algo, run, err)
					continue
				}
				q := solver.CalculateQuality(sched.Assignments)
				unplaced := len(solver.ComputeUnplaced(sched.Assignments, *data))
				fmt.Printf("%s,%s,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%.0f,%d\n", construct, algo, run, sched.Score,
					q.Even.GroupGaps, q.Odd.GroupGaps, q.Even.GroupLongGaps+q.Odd.GroupLongGaps, q.Even.SingleClassDays, q.Odd.SingleClassDays,
					q.Even.SaturdayPairs+q.Odd.SaturdayPairs,
					max(q.Even.MaxGroupPairsPerDay, q.Odd.MaxGroupPairsPerDay),
					unplaced, time.Since(start).Seconds(), sched.Run.Rounds)
				scores = append(scores, sched.Score)
			}
			if len(scores) > 0 {
				sort.Ints(scores)
				fmt.Fprintf(os.Stderr, "%s/%s: медиана %d, мин %d, макс %d (%d прогонов)\n",
					construct, algo, scores[len(scores)/2], scores[0], scores[len(scores)-1], len(scores))
			}
		}
	}
}

func loadFromDB(dsn string) (*domain.InputData, error) {
	ctx := context.Background()
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("подключение к БД: %w", err)
	}
	defer pool.Close()
	return postgres.NewInputRepository(pool).LoadInput(ctx)
}

// loadSnapshots читает справочники из JSON-файлов, которые заливает make seed.
func loadSnapshots(dir string) (*domain.InputData, error) {
	var in domain.InputData
	files := []struct {
		name string
		dst  any
	}{
		{"buildings.json", &in.Buildings},
		{"departments.json", &in.Departments},
		{"groups.json", &in.Groups},
		{"teachers.json", &in.Teachers},
		{"rooms.json", &in.Rooms},
		{"subject_plans.json", &in.SubjectPlans},
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dir, f.name))
		if err != nil {
			return nil, err
		}
		raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
		if err := json.Unmarshal(raw, f.dst); err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
	}
	return &in, nil
}
