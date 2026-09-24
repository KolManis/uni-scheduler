package solver

import (
	"reflect"
	"testing"
	"time"
)

// Повтор по записанным сидам и числу раундов даёт то же расписание, хотя первый запуск
// останавливался по времени, а повтор — по раундам (ADR-0023). Бюджет повтора нарочно
// меньше: время на результат повтора влиять не должно.
func TestReplayGivesSameSchedule(t *testing.T) {
	if testing.Short() {
		t.Skip("долгий тест на реальных данных")
	}
	input := loadSnapshotInput(t)
	for _, algo := range []ImproveAlgorithm{ImproveHillClimb, ImproveLNS, ImproveTabuSearch, ImproveGeneticAlgorithm} {
		t.Run(string(algo), func(t *testing.T) {
			first, err := Solve(input, Options{Improve: algo, Budget: 2 * time.Second, Seed: 7})
			if err != nil {
				t.Fatal(err)
			}
			run := first.Run
			if !run.Exact || run.Rounds == 0 || run.ImproveSeed == 0 {
				t.Fatalf("запуск не записан: %+v", run)
			}
			again, err := Solve(input, Options{Improve: algo, Budget: time.Millisecond,
				Seed: run.Seed, ImproveSeed: run.ImproveSeed, Rounds: run.Rounds})
			if err != nil {
				t.Fatal(err)
			}
			if again.Score != first.Score || !reflect.DeepEqual(again.Assignments, first.Assignments) {
				t.Errorf("повтор дал другое расписание: score %d вместо %d (раундов %d)", again.Score, first.Score, run.Rounds)
			}
		})
	}
}
