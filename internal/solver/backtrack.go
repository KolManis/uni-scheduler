package solver

import (
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type solverState struct {
	input            schedule.InputData
	teacherMap       map[string]schedule.Teacher
	groupMap         map[string]schedule.Group
	roomMap          map[string]schedule.Room
	assignments      []schedule.Assignment
	teacherLoad      map[string]int
	subjectCount     map[string]map[schedule.ClassType]int
	occupiedGroups   map[schedule.TimeSlot]map[string]schedule.Parity
	occupiedTeachers map[schedule.TimeSlot]map[string]schedule.Parity
	occupiedRooms    map[schedule.TimeSlot]map[string]schedule.Parity
	dayLoad          map[schedule.Day]int
	iterations       int
	bestScore        int
	bestSolution     []schedule.Assignment
	logger           *slog.Logger
	rng              *rand.Rand
}

func newState(input schedule.InputData, rng *rand.Rand) *solverState {
	teacherMap := make(map[string]schedule.Teacher)
	for _, t := range input.Teachers {
		teacherMap[t.ID] = t
	}

	groupMap := make(map[string]schedule.Group)
	for _, g := range input.Groups {
		groupMap[g.ID] = g
	}

	roomMap := make(map[string]schedule.Room)
	for _, r := range input.Rooms {
		roomMap[r.ID] = r
	}

	return &solverState{
		input:            input,
		teacherMap:       teacherMap,
		groupMap:         groupMap,
		roomMap:          roomMap,
		assignments:      make([]schedule.Assignment, 0),
		teacherLoad:      make(map[string]int),
		subjectCount:     make(map[string]map[schedule.ClassType]int),
		occupiedGroups:   make(map[schedule.TimeSlot]map[string]schedule.Parity),
		occupiedTeachers: make(map[schedule.TimeSlot]map[string]schedule.Parity),
		occupiedRooms:    make(map[schedule.TimeSlot]map[string]schedule.Parity),
		dayLoad:          make(map[schedule.Day]int),
		iterations:       0,
		bestScore:        9999999,
		bestSolution:     nil,
		logger:           slog.Default(),
		rng:              rng,
	}
}

type SolveResult struct {
	Schedule *schedule.Schedule
	Score    int
	Error    error
}

func SolveParallel(input schedule.InputData, maxIterations int, numWorkers int) (*schedule.Schedule, error) {
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
					Schedule: &schedule.Schedule{Assignments: result, Score: score},
					Score:    score,
				}
			} else if len(state.bestSolution) > 0 {
				score := calculateFitness(state.bestSolution, input)
				state.logger.Info("worker partial solution",
					"assignments", len(state.bestSolution),
					"score", score,
				)
				results <- SolveResult{
					Schedule: &schedule.Schedule{Assignments: state.bestSolution, Score: score},
					Score:    score,
				}
			} else {
				state.logger.Error("worker no solution")
				results <- SolveResult{Error: schedule.ErrNoSolution}
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
		return nil, schedule.ErrNoSolution
	}

	slog.Info("parallel solve complete",
		"workers", numWorkers,
		"best_score", bestResult.Score,
	)

	return bestResult.Schedule, nil
}

func Solve(input schedule.InputData, maxIterations int) (*schedule.Schedule, error) {
	return SolveParallel(input, maxIterations, 1)
}

func backtrack(state *solverState, depth int, maxIter int) ([]schedule.Assignment, bool) {
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
			state.bestSolution = make([]schedule.Assignment, len(state.assignments))
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

func assign(state *solverState, a schedule.Assignment) {
	state.assignments = append(state.assignments, a)
	state.teacherLoad[a.TeacherID] += 2
	state.dayLoad[a.TimeSlot.Day]++

	if state.subjectCount[a.SubjectID] == nil {
		state.subjectCount[a.SubjectID] = make(map[schedule.ClassType]int)
	}
	state.subjectCount[a.SubjectID][a.Type] += 2

	for _, gid := range a.GroupIDs {
		if state.occupiedGroups[a.TimeSlot] == nil {
			state.occupiedGroups[a.TimeSlot] = make(map[string]schedule.Parity)
		}
		state.occupiedGroups[a.TimeSlot][gid] = a.Parity
	}

	if state.occupiedTeachers[a.TimeSlot] == nil {
		state.occupiedTeachers[a.TimeSlot] = make(map[string]schedule.Parity)
	}
	state.occupiedTeachers[a.TimeSlot][a.TeacherID] = a.Parity

	if state.occupiedRooms[a.TimeSlot] == nil {
		state.occupiedRooms[a.TimeSlot] = make(map[string]schedule.Parity)
	}
	state.occupiedRooms[a.TimeSlot][a.RoomID] = a.Parity
}

func unassign(state *solverState, a schedule.Assignment) {
	state.assignments = state.assignments[:len(state.assignments)-1]
	state.teacherLoad[a.TeacherID] -= 2
	state.dayLoad[a.TimeSlot.Day]--

	if state.subjectCount[a.SubjectID] != nil {
		state.subjectCount[a.SubjectID][a.Type] -= 2
	}

	for _, gid := range a.GroupIDs {
		delete(state.occupiedGroups[a.TimeSlot], gid)
	}

	delete(state.occupiedTeachers[a.TimeSlot], a.TeacherID)
	delete(state.occupiedRooms[a.TimeSlot], a.RoomID)
}
