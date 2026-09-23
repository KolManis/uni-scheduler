package solver

import (
	"cmp"
	"slices"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// normalizeInput возвращает копию входных данных, отсортированную по ID.
//
// Построение зависит от порядка входных данных: при равных штрафах побеждает тот
// преподаватель, план, аудитория или слот, что встретился раньше. Порядок строк из БД
// без ORDER BY не гарантирован, поэтому одни и те же данные давали разные расписания:
// замер на реальных данных — score построения от 198 до 364 тыс. при перемешивании
// входа, и изредка случайно плохой итог генерации. Сортировка делает результат
// зависимым только от самих данных, а не от того, как их прочитали.
func normalizeInput(input domain.InputData) domain.InputData {
	input.Buildings = sortedByID(input.Buildings, func(b domain.Building) string { return b.ID })
	input.Departments = sortedByID(input.Departments, func(d domain.Department) string { return d.ID })
	input.Groups = sortedByID(input.Groups, func(g domain.Group) string { return g.ID })
	input.Teachers = sortedByID(input.Teachers, func(t domain.Teacher) string { return t.ID })
	input.Rooms = sortedByID(input.Rooms, func(r domain.Room) string { return r.ID })
	input.SubjectPlans = sortedByID(input.SubjectPlans, func(sp domain.SubjectPlan) string { return sp.ID })
	return input
}

func sortedByID[T any](items []T, id func(T) string) []T {
	out := slices.Clone(items)
	slices.SortStableFunc(out, func(a, b T) int { return cmp.Compare(id(a), id(b)) })
	return out
}
