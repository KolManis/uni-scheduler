package solver

// Построение по преподавателям: преподаватели от самых ограниченных к самым гибким, пары
// каждого ставятся подряд функцией placeTask (placement.go).

import (
	"math/rand"
	"sort"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// constructByTeacher — построение по преподавателям: от самых ограниченных к самым
// гибким, пары преподавателя ставятся подряд.
func constructByTeacher(draft *scheduleDraft, rng *rand.Rand) {
	teachers := orderTeachersByFlexibility(draft)
	if rng != nil {
		shuffleWithinBuckets(teachers, rng, func(t domain.Teacher) float64 {
			return teacherFlexibility(draft, t)
		})
	}

	for _, t := range teachers {
		draft.logger.Info("processing teacher", "id", t.ID, "name", t.Name, "max_hours", t.MaxWeeklyHours)
		tasks := collectRemaining(draft, draft.input, t)
		if rng != nil {
			shuffleTasksWithinPriority(tasks, rng)
		}
		for _, task := range tasks {
			placeTask(draft, task)
		}
	}
}

// orderTeachersByFlexibility — от самых «жёстких» к самым «гибким». Гибкость = сколько
// пар в неделю преподаватель реально может провести, делённое на сколько ему надо провести.
// Значение < 1 — преподаватель ограничен физически (у него меньше свободных пар, чем нагрузка).
// При равной гибкости — вперёд идут те, у кого больше предметов (сложнее собрать расписание).
func orderTeachersByFlexibility(draft *scheduleDraft) []domain.Teacher {
	teachers := make([]domain.Teacher, len(draft.input.Teachers))
	copy(teachers, draft.input.Teachers)

	subjectCount := make(map[string]int)
	for _, sp := range draft.input.SubjectPlans {
		subjectCount[sp.TeacherID]++
	}

	sort.Slice(teachers, func(i, j int) bool {
		fi, fj := teacherFlexibility(draft, teachers[i]), teacherFlexibility(draft, teachers[j])
		if fi != fj {
			return fi < fj
		}
		return subjectCount[teachers[i].ID] > subjectCount[teachers[j].ID]
	})
	return teachers
}

// teacherFlexibility — доступных пар в неделю на один нужный час. Меньше = жёстче.
// Используется и в основной сортировке, и в bucketing для случайных перемешиваний.
func teacherFlexibility(draft *scheduleDraft, t domain.Teacher) float64 {
	const totalSlots = 36 // 6 дней × 6 пар

	need := 0
	for _, sp := range draft.input.SubjectPlans {
		if sp.TeacherID == t.ID {
			need += sp.LectureHours + sp.PracticeHours + sp.LabHours
		}
	}
	if need <= 0 {
		return 1e9 // без нагрузки — максимально гибкий, в конец
	}
	// Пара на другом факультете «всегда» занимает слот целиком, по чётности — наполовину.
	external := 0.0
	for _, ep := range t.ExternalPairs {
		if ep.Parity == domain.Even || ep.Parity == domain.Odd {
			external += 0.5
		} else {
			external++
		}
	}
	available := float64(totalSlots-len(t.UnavailableSlots)) - external
	if available <= 0 {
		return 0
	}
	return available / float64(need)
}

// shuffleWithinBuckets перемешивает элементы уже отсортированного слайса, но только
// внутри непрерывных участков, где ключ одинаковый. Это ломает произвольный tie-break,
// не нарушая доминирующий порядок.
func shuffleWithinBuckets[T any](items []T, rng *rand.Rand, key func(T) float64) {
	i := 0
	for i < len(items) {
		j := i + 1
		for j < len(items) && key(items[j]) == key(items[i]) {
			j++
		}
		if j-i > 1 {
			bucket := items[i:j]
			rng.Shuffle(len(bucket), func(a, b int) { bucket[a], bucket[b] = bucket[b], bucket[a] })
		}
		i = j
	}
}

// shuffleTasksWithinPriority перемешивает задачи одного класса и близких приоритетов.
// Порядок «лекции первыми, потом практики, потом лабы» сохраняется — рандомизация только
// в пределах одного classType между разными предметами.
func shuffleTasksWithinPriority(tasks []placementTask, rng *rand.Rand) {
	i := 0
	for i < len(tasks) {
		j := i + 1
		for j < len(tasks) && tasks[j].classType == tasks[i].classType {
			j++
		}
		if j-i > 1 {
			bucket := tasks[i:j]
			rng.Shuffle(len(bucket), func(a, b int) { bucket[a], bucket[b] = bucket[b], bucket[a] })
		}
		i = j
	}
}
