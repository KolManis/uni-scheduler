package solver

import (
	"log/slog"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

type solverState struct {
	input            schedule.InputData
	teacherMap       map[string]schedule.Teacher
	groupMap         map[string]schedule.Group
	roomMap          map[string]schedule.Room
	assignments      []schedule.Assignment
	teacherLoad      map[string]int
	subjectCount     map[string]map[schedule.ClassType]int
	occupiedGroups   map[schedule.TimeSlot]map[string]bool
	occupiedTeachers map[schedule.TimeSlot]map[string]bool
	occupiedRooms    map[schedule.TimeSlot]map[string]bool
	dayLoad          map[schedule.Day]int
	iterations       int
	bestScore        int
	bestSolution     []schedule.Assignment
	logger           *slog.Logger // ДОБАВЛЯЕМ ЛОГГЕР
}

func newState(input schedule.InputData) *solverState {
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
		occupiedGroups:   make(map[schedule.TimeSlot]map[string]bool),
		occupiedTeachers: make(map[schedule.TimeSlot]map[string]bool),
		occupiedRooms:    make(map[schedule.TimeSlot]map[string]bool),
		dayLoad:          make(map[schedule.Day]int),
		iterations:       0,
		bestScore:        9999999,
		bestSolution:     nil,
		logger:           slog.Default(), // ИСПОЛЬЗУЕМ ДЕФОЛТНЫЙ ЛОГГЕР
	}
}

func Solve(input schedule.InputData, maxIterations int) (*schedule.Schedule, error) {
	state := newState(input)
	result, found := backtrack(state, 0, maxIterations)

	if !found {
		if len(state.bestSolution) > 0 {
			score := calculateFitness(state.bestSolution, input)
			state.logger.Info("partial solution found",
				"assignments", len(state.bestSolution),
				"score", score,
				"iterations", state.iterations,
			)
			return &schedule.Schedule{
				Assignments: state.bestSolution,
				Score:       score,
			}, nil
		}
		state.logger.Error("no solution",
			"iterations", state.iterations,
			"assignments_placed", len(state.assignments),
		)
		return nil, schedule.ErrNoSolution
	}

	score := calculateFitness(result, input)
	return &schedule.Schedule{
		Assignments: result,
		Score:       score,
	}, nil
}

func backtrack(state *solverState, depth int, maxIter int) ([]schedule.Assignment, bool) {
	if maxIter > 0 && state.iterations >= maxIter {
		return nil, false
	}
	state.iterations++

	// ЛОГИРОВАНИЕ каждые 100000 итераций
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
		// УБИРАЕМ ранний выход при score==0
		// if score == 0 {
		//     return state.assignments, true
		// }
		return nil, false // продолжаем искать лучшее
	}

	plan, classType := selectMostConstrained(state)
	if plan.ID == "" {
		return nil, false
	}

	candidates := generateCandidates(state, plan, classType)

	// ЛОГ ПРИ ОТСУТСТВИИ КАНДИДАТОВ
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

// assign, unassign — без изменений
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
			state.occupiedGroups[a.TimeSlot] = make(map[string]bool)
		}
		state.occupiedGroups[a.TimeSlot][gid] = true
	}

	if state.occupiedTeachers[a.TimeSlot] == nil {
		state.occupiedTeachers[a.TimeSlot] = make(map[string]bool)
	}
	state.occupiedTeachers[a.TimeSlot][a.TeacherID] = true

	if state.occupiedRooms[a.TimeSlot] == nil {
		state.occupiedRooms[a.TimeSlot] = make(map[string]bool)
	}
	state.occupiedRooms[a.TimeSlot][a.RoomID] = true
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
