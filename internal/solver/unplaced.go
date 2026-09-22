package solver

import "github.com/KolManis/uni-scheduler/internal/domain/schedule"

// ComputeUnplaced сравнивает итоговые assignments с планом (input.SubjectPlans) и возвращает
// те часы, которые ни один из солверов не смог поставить в расписание. Считается по факту
// из готового результата, поэтому работает одинаково для teacher-driven и backtracking-солвера.
func ComputeUnplaced(assignments []schedule.Assignment, input schedule.InputData) []schedule.UnplacedItem {
	placed := make(map[string]map[schedule.ClassType]int)
	for _, a := range assignments {
		if placed[a.SubjectID] == nil {
			placed[a.SubjectID] = make(map[schedule.ClassType]int)
		}
		placed[a.SubjectID][a.Type] += 2
	}

	// make(..., 0), а не nil-слайс: nil маршалится в JSON как null, а не [],
	// из-за чего jsonb_array_length(unplaced) в SQL падает на "пустых" расписаниях.
	unplaced := make([]schedule.UnplacedItem, 0)
	for _, sp := range input.SubjectPlans {
		for _, ct := range []schedule.ClassType{schedule.Lecture, schedule.Practice, schedule.Lab} {
			var total int
			switch ct {
			case schedule.Lecture:
				total = sp.LectureHours
			case schedule.Practice:
				total = sp.PracticeHours
			case schedule.Lab:
				total = sp.LabHours
			}
			if total <= 0 {
				continue
			}
			have := 0
			if placed[sp.ID] != nil {
				have = placed[sp.ID][ct]
			}
			if have < total {
				unplaced = append(unplaced, schedule.UnplacedItem{
					SubjectID:    sp.ID,
					Type:         ct,
					MissingHours: total - have,
				})
			}
		}
	}
	return unplaced
}
