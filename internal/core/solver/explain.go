package solver

import (
	"fmt"
	"sort"
	"strings"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// ExplainScore — из чего складывается score: каждое нарушение отдельной строкой — правило,
// группа или преподаватель, неделя, день, что именно и сколько стоит. Считается теми же
// правилами (penalties.go), что и оценка, поэтому сумма Penalty равна CalculateFitness.
// Порядок: сначала дорогие, при равной цене — по правилу, неделе и дню.
func ExplainScore(assignments []domain.Assignment, input domain.InputData) []domain.Violation {
	buildingNames := make(map[string]string)
	for _, b := range input.Buildings {
		buildingNames[b.ID] = b.Name
	}
	var out []domain.Violation
	for _, weekParity := range []domain.Parity{domain.Even, domain.Odd} {
		pairs := assignmentsInWeek(assignments, weekParity)
		groups, teachers := buildWeeks(pairs, input)
		for id, w := range groups {
			out = append(out, groupViolations(id, weekParity, w, buildingNames)...)
		}
		for id, w := range teachers {
			out = append(out, teacherViolations(id, weekParity, w)...)
		}
		out = append(out, saturdayPairViolations(pairs, weekParity)...)
	}
	out = append(out, preferenceViolations(assignments, input)...)

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Penalty != b.Penalty {
			return a.Penalty > b.Penalty
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Week != b.Week {
			return a.Week < b.Week
		}
		if dayIndex(a.Day) != dayIndex(b.Day) {
			return dayIndex(a.Day) < dayIndex(b.Day)
		}
		return who(a) < who(b)
	})
	return out
}

// groupViolations — нарушения одной группы за неделю: правила те же, что в groupPenalties.
// buildingNames — названия корпусов для подписей переходов («Корпус А → Корпус Б»).
func groupViolations(groupID string, weekParity domain.Parity, w *week, buildingNames map[string]string) []domain.Violation {
	out := violationList{base: domain.Violation{GroupIDs: []string{groupID}, Week: weekParity}}

	if w.saturday == 1 {
		out.add("Saturday", "Одна пара в субботу", domain.Saturday, "ради одной пары группа едет в субботу", loneSaturdayPenalty)
	}
	days, total := 0, 0
	for d, day := range domain.AllDays {
		all, n := w.dayPairs(d)
		pairs := all[:n]
		if n == 0 {
			continue
		}
		days++
		total += n

		out.add("GroupDayOverload", "Перегрузка дня", day, fmt.Sprintf("%d пар", n), dayOverloadPenalty(n))
		out.add("GroupLongDay", "Длинный день", day, fmt.Sprintf("%d пар: %s", n, pairList(pairs)), longDayPenalty(pairs))
		if n == 1 {
			out.add("SingleClassDay", "День с одной парой", day, fmt.Sprintf("только %d-я пара", pairs[0]+1), singleClassDayPenalty)
		}
		for i := 1; i < n; i++ {
			prev, next := pairs[i-1], pairs[i]
			empty := next - prev - 1
			switch {
			case empty >= 2:
				out.add("GroupGaps", "Окно у группы", day, fmt.Sprintf("пустые %s между %d-й и %d-й", pairRange(prev+1, next-1), prev+1, next+1), empty*groupGapPenalty)
				out.add("GroupLongGaps", "Окно в 2+ пары подряд (HC8)", day, fmt.Sprintf("между %d-й и %d-й парой", prev+1, next+1), longGapPenalty)
			case empty == 1:
				out.add("GroupGaps", "Окно у группы", day, fmt.Sprintf("пустая %d-я пара", prev+2), groupGapPenalty)
			}
			from, to := w.building[d*6+prev], w.building[d*6+next]
			out.add("BuildingTransitions", "Переход в другой корпус", day,
				fmt.Sprintf("%s → %s: %d-я → %d-я пара", buildingLabel(buildingNames, from), buildingLabel(buildingNames, to), prev+1, next+1), transitionPenalty(from, to, next-prev))
		}
	}
	out.add("GroupTooFewDays", "Мало учебных дней", "", fmt.Sprintf("%d пар за %d дн.", total, days), tooFewDaysPenalty(total, days))
	return out.items
}

// teacherViolations — нарушения одного преподавателя за неделю: как в teacherPenalties.
func teacherViolations(teacherID string, weekParity domain.Parity, w *week) []domain.Violation {
	out := violationList{base: domain.Violation{TeacherID: teacherID, Week: weekParity}}

	days, total := 0, 0
	for d, day := range domain.AllDays {
		all, n := w.dayPairs(d)
		pairs := all[:n]
		if n == 0 {
			continue
		}
		days++
		total += n
		if extra := n - teacherMaxPairsPerDay; extra > 0 {
			out.add("TeacherDayOverload", "Перегрузка преподавателя", day,
				fmt.Sprintf("%d пар (норма %d)", n, teacherMaxPairsPerDay), extra*teacherOverloadPenalty)
		}
		if g := gapsIn(pairs); g > 0 {
			out.add("TeacherGaps", "Окно у преподавателя", day, fmt.Sprintf("пустых пар: %d (%s)", g, pairList(pairs)), g*teacherGapPenalty)
		}
	}
	if total >= 4 && days < 3 {
		out.add("TeacherConcentration", "Нагрузка преподавателя в мало дней", "",
			fmt.Sprintf("%d пар за %d дн.", total, days), (3-days)*400)
	}
	return out.items
}

// violationList собирает нарушения одного владельца: общие поля берутся из base,
// нарушения с нулевой ценой пропускаются.
type violationList struct {
	base  domain.Violation
	items []domain.Violation
}

func (l *violationList) add(category, rule string, day domain.Day, detail string, penalty int) {
	if penalty <= 0 {
		return
	}
	v := l.base
	v.Category, v.Rule, v.Day, v.Detail, v.Penalty = category, rule, day, detail, penalty
	l.items = append(l.items, v)
}

// saturdayPairViolations — каждая пара в субботу (saturdayPairPenalty).
func saturdayPairViolations(pairs []domain.Assignment, weekParity domain.Parity) []domain.Violation {
	var out []domain.Violation
	for _, a := range pairs {
		if a.TimeSlot.Day() == domain.Saturday {
			out = append(out, domain.Violation{Category: "Saturday", Rule: "Пара в субботу", GroupIDs: a.GroupIDs,
				TeacherID: a.TeacherID, Week: weekParity, Day: domain.Saturday,
				Detail: fmt.Sprintf("%d-я пара", a.TimeSlot.PairNum()), Penalty: saturdayPairPenalty})
		}
	}
	return out
}

// buildWeeks — недели всех групп и преподавателей из пар одной учебной недели (как в
// weekBreakdown). Спортзал и стадион не участвуют в переходах между корпусами.
func buildWeeks(pairs []domain.Assignment, input domain.InputData) (groups, teachers map[string]*week) {
	sportRooms := make(map[string]bool)
	for _, r := range input.Rooms {
		if isSportRoomType(r.Type) {
			sportRooms[r.ID] = true
		}
	}
	groups, teachers = map[string]*week{}, map[string]*week{}
	for _, a := range pairs {
		slot := slotIndex(a.TimeSlot)
		building := a.BuildingID
		if sportRooms[a.RoomID] {
			building = ""
		}
		for _, gid := range a.GroupIDs {
			weekFor(groups, gid).add(slot, building)
		}
		weekFor(teachers, a.TeacherID).add(slot, "")
	}
	return groups, teachers
}

// buildingLabel — название корпуса, а если его нет в справочнике — «корпус <id>».
func buildingLabel(names map[string]string, id string) string {
	if name := names[id]; name != "" {
		return name
	}
	return "корпус " + id
}

// pairList — «1, 2, 4» (номера с единицы).
func pairList(pairs []int) string {
	s := make([]string, len(pairs))
	for i, p := range pairs {
		s[i] = fmt.Sprint(p + 1)
	}
	return strings.Join(s, ", ")
}

// pairRange — «2-я и 3-я пары» или «2–4-я пары» (номера с нуля на входе).
func pairRange(from, to int) string {
	if to == from+1 {
		return fmt.Sprintf("%d-я и %d-я пары", from+1, to+1)
	}
	return fmt.Sprintf("%d–%d-я пары", from+1, to+1)
}

func dayIndex(d domain.Day) int {
	for i, x := range domain.AllDays {
		if x == d {
			return i
		}
	}
	return len(domain.AllDays)
}

func who(v domain.Violation) string {
	return v.TeacherID + strings.Join(v.GroupIDs, ",")
}
