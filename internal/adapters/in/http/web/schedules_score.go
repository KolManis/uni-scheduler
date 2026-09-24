package web

import (
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// scheduleScoreData — страница «из-за чего такой score»: итог по правилам и каждое нарушение.
type scheduleScoreData struct {
	Schedule   *domain.Schedule
	Score      int
	Rules      []ruleTotal
	Violations []domain.Violation
	Teachers   []domain.Teacher
	Groups     []domain.Group
}

// ruleTotal — одно правило: сколько раз нарушено и сколько это стоит в сумме.
type ruleTotal struct {
	Rule    string
	Count   int
	Penalty int
}

func (h *Handler) schedulesScore(w http.ResponseWriter, r *http.Request) {
	id, err := int64FromPath(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	eval, data, err := h.evaluate(r, id)
	if err != nil {
		writePageError(w, r, err)
		return
	}
	render(w, r, h.pages["schedules_score.html"], scheduleScoreData{
		Schedule: eval.Schedule, Score: eval.Score,
		Rules:      totalsByRule(eval.Violations),
		Violations: eval.Violations,
		Teachers:   data.Teachers, Groups: data.Groups,
	})
}

// totalsByRule — итог по правилам в порядке первого появления (нарушения уже отсортированы
// от дорогих к дешёвым, так что сверху — самые дорогие правила).
func totalsByRule(violations []domain.Violation) []ruleTotal {
	var out []ruleTotal
	index := map[string]int{}
	for _, v := range violations {
		i, ok := index[v.Rule]
		if !ok {
			i = len(out)
			index[v.Rule] = i
			out = append(out, ruleTotal{Rule: v.Rule})
		}
		out[i].Count++
		out[i].Penalty += v.Penalty
	}
	return out
}
