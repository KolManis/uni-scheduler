package solver

import (
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type solverState struct {
	input            domain.InputData
	teacherMap       map[string]domain.Teacher
	groupMap         map[string]domain.Group
	roomMap          map[string]domain.Room
	assignments      []domain.Assignment
	teacherLoad      map[string]int
	subjectCount     map[string]map[domain.ClassType]int
	occupiedGroups   map[domain.TimeSlot]map[string]domain.Parity
	occupiedTeachers map[domain.TimeSlot]map[string]domain.Parity
	occupiedRooms    map[domain.TimeSlot]map[string]domain.Parity
	dayLoad          map[domain.Day]int
	iterations       int
	bestScore        int
	bestSolution     []domain.Assignment
	logger           *slog.Logger
	rng              *rand.Rand
}

func newState(input domain.InputData, rng *rand.Rand) *solverState {
	teacherMap := make(map[string]domain.Teacher)
	for _, t := range input.Teachers {
		teacherMap[t.ID] = t
	}

	groupMap := make(map[string]domain.Group)
	for _, g := range input.Groups {
		groupMap[g.ID] = g
	}

	roomMap := make(map[string]domain.Room)
	for _, r := range input.Rooms {
		roomMap[r.ID] = r
	}

	return &solverState{
		input:            input,
		teacherMap:       teacherMap,
		groupMap:         groupMap,
		roomMap:          roomMap,
		assignments:      make([]domain.Assignment, 0),
		teacherLoad:      make(map[string]int),
		subjectCount:     make(map[string]map[domain.ClassType]int),
		occupiedGroups:   make(map[domain.TimeSlot]map[string]domain.Parity),
		occupiedTeachers: make(map[domain.TimeSlot]map[string]domain.Parity),
		occupiedRooms:    make(map[domain.TimeSlot]map[string]domain.Parity),
		dayLoad:          make(map[domain.Day]int),
		iterations:       0,
		bestScore:        9999999,
		bestSolution:     nil,
		logger:           slog.Default(),
		rng:              rng,
	}
}

type SolveResult struct {
	Schedule *domain.Schedule
	Score    int
	Error    error
}

func SolveParallel(input domain.InputData, maxIterations int, numWorkers int) (*domain.Schedule, error) {
	if numWorkers <= 0 {
		numWorkers = 4
	}

	results := make(chan SolveResult, numWorkers)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			state := newState(input, rng)
			state.logger = slog.Default().With("worker", workerID)

			state.logger.Info("worker started", "max_iter", maxIterations)

			result, found := backtrack(state, 0, maxIterations)

			if found {
				score := calculateFitness(result, input)
				results <- SolveResult{
					Schedule: &domain.Schedule{Assignments: result, Score: score},
					Score:    score,
				}
			} else if len(state.bestSolution) > 0 {
				score := calculateFitness(state.bestSolution, input)
				state.logger.Info("worker partial solution",
					"assignments", len(state.bestSolution),
					"score", score,
				)
				results <- SolveResult{
					Schedule: &domain.Schedule{Assignments: state.bestSolution, Score: score},
					Score:    score,
				}
			} else {
				state.logger.Error("worker no solution")
				results <- SolveResult{Error: domain.ErrNoSolution}
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var bestResult *SolveResult
	for result := range results {
		if result.Error != nil {
			continue
		}
		if bestResult == nil || result.Score < bestResult.Score {
			bestResult = &result
		}
	}

	if bestResult == nil {
		return nil, domain.ErrNoSolution
	}

	slog.Info("parallel solve complete",
		"workers", numWorkers,
		"best_score", bestResult.Score,
	)

	return bestResult.Schedule, nil
}

func Solve(input domain.InputData, maxIterations int) (*domain.Schedule, error) {
	return SolveParallel(input, maxIterations, 1)
}

func backtrack(state *solverState, depth int, maxIter int) ([]domain.Assignment, bool) {
	if maxIter > 0 && state.iterations >= maxIter {
		return nil, false
	}
	state.iterations++

	if state.iterations%100000 == 0 {
		state.logger.Info("solving progress",
			"iter", state.iterations,
			"depth", depth,
			"placed", len(state.assignments),
			"best_score", state.bestScore,
		)
	}

	if allSubjectsPlaced(state) {
		score := calculateFitness(state.assignments, state.input)
		if score < state.bestScore {
			state.bestScore = score
			state.bestSolution = make([]domain.Assignment, len(state.assignments))
			copy(state.bestSolution, state.assignments)
			state.logger.Info("new best solution", "score", score, "iter", state.iterations)
		}
		return nil, false
	}

	plan, classType := selectMostConstrained(state)
	if plan.ID == "" {
		return nil, false
	}

	candidates := generateCandidates(state, plan, classType)

	if len(candidates) == 0 {
		state.logger.Warn("no candidates",
			"subject", plan.ID,
			"type", classType,
			"teacher", plan.TeacherID,
			"groups", plan.GroupIDs,
			"depth", depth,
			"iter", state.iterations,
		)
	}

	for _, cand := range candidates {
		assign(state, cand.assignment)
		if result, found := backtrack(state, depth+1, maxIter); found {
			return result, true
		}
		unassign(state, cand.assignment)
	}

	if depth == 0 && len(state.bestSolution) > 0 {
		return state.bestSolution, true
	}

	return nil, false
}

func assign(state *solverState, a domain.Assignment) {
	state.assignments = append(state.assignments, a)
	state.teacherLoad[a.TeacherID] += 2
	state.dayLoad[a.TimeSlot.Day()]++

	if state.subjectCount[a.SubjectID] == nil {
		state.subjectCount[a.SubjectID] = make(map[domain.ClassType]int)
	}
	state.subjectCount[a.SubjectID][a.Type] += 2

	for _, gid := range a.GroupIDs {
		if state.occupiedGroups[a.TimeSlot] == nil {
			state.occupiedGroups[a.TimeSlot] = make(map[string]domain.Parity)
		}
		state.occupiedGroups[a.TimeSlot][gid] = a.Parity
	}

	if state.occupiedTeachers[a.TimeSlot] == nil {
		state.occupiedTeachers[a.TimeSlot] = make(map[string]domain.Parity)
	}
	state.occupiedTeachers[a.TimeSlot][a.TeacherID] = a.Parity

	if state.occupiedRooms[a.TimeSlot] == nil {
		state.occupiedRooms[a.TimeSlot] = make(map[string]domain.Parity)
	}
	state.occupiedRooms[a.TimeSlot][a.RoomID] = a.Parity
}

func unassign(state *solverState, a domain.Assignment) {
	state.assignments = state.assignments[:len(state.assignments)-1]
	state.teacherLoad[a.TeacherID] -= 2
	state.dayLoad[a.TimeSlot.Day()]--

	if state.subjectCount[a.SubjectID] != nil {
		state.subjectCount[a.SubjectID][a.Type] -= 2
	}

	for _, gid := range a.GroupIDs {
		delete(state.occupiedGroups[a.TimeSlot], gid)
	}

	delete(state.occupiedTeachers[a.TimeSlot], a.TeacherID)
	delete(state.occupiedRooms[a.TimeSlot], a.RoomID)
}
