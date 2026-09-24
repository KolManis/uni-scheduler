package solver

import (
	"testing"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// TestSolveWithFixed_PinnedStay — перегенерация вокруг закреплённых пар: каждый метод
// улучшения оставляет их на месте и в той же аудитории, остальные пары ставит, дублей нет.
func TestSolveWithFixed_PinnedStay(t *testing.T) {
	input := loadSnapshotInput(t)
	base, err := Solve(input, Options{Construction: ConstructDSatur, Improve: ImproveHillClimb, Budget: time.Nanosecond})
	if err != nil {
		t.Fatal(err)
	}
	// Закрепляем каждую седьмую пару.
	var fixed []domain.Assignment
	for i, a := range base.Assignments {
		if i%7 == 0 {
			fixed = append(fixed, a)
		}
	}

	for _, algo := range []ImproveAlgorithm{ImproveHillClimb, ImproveSimulatedAnnealing, ImproveTabuSearch, ImproveGeneticAlgorithm, ImproveLNS} {
		t.Run(string(algo), func(t *testing.T) {
			sched, err := Solve(input, Options{Construction: ConstructTeacher, Improve: algo, Budget: 2 * time.Second, Fixed: fixed})
			if err != nil {
				t.Fatal(err)
			}
			type key struct {
				subject, teacher, room string
				slot                   domain.TimeSlot
				parity                 domain.Parity
			}
			pinned := map[key]int{}
			for _, a := range sched.Assignments {
				if a.Pinned {
					pinned[key{a.SubjectID, a.TeacherID, a.RoomID, a.TimeSlot, a.Parity}]++
				}
			}
			for _, f := range fixed {
				k := key{f.SubjectID, f.TeacherID, f.RoomID, f.TimeSlot, f.Parity}
				if pinned[k] == 0 {
					t.Fatalf("закреплённая пара сдвинута или потеряна: %+v", f)
				}
				pinned[k]--
			}
			if len(sched.Assignments) != len(base.Assignments) {
				t.Errorf("пар было %d, стало %d", len(base.Assignments), len(sched.Assignments))
			}
			if n := len(ComputeUnplaced(sched.Assignments, input)); n != 0 {
				t.Errorf("не поставлено: %d", n)
			}
			if v := checkHardConstraintsAll(sched.Assignments, normalizeInput(input)); len(v) > 0 {
				t.Errorf("нарушения: %v", v[:min(len(v), 3)])
			}
		})
	}
}
