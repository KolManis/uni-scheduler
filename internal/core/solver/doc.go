// Package solver составляет расписание занятий: из учебных планов, преподавателей, групп и
// аудиторий получается список пар (domain.Assignment) и его оценка — score.
//
// # Как читать
//
// Начните с solve.go: Solve → solveOnce, где весь путь расписания виден по шагам:
//
//	normalizeInput   normalize.go          упорядочить входные данные (результат не зависит от порядка)
//	construct        solve.go              начальное расписание, одним из двух алгоритмов:
//	  constructDSatur    dsatur.go         «самая трудная пара первой» (по умолчанию)
//	  constructByTeacher teacher_solver.go преподаватели от самых ограниченных к гибким
//	insertUnplaced   unplaced_insert.go    поставить не поместившиеся пары, сдвинув соседей
//	converge         local_search.go       простые перестановки, пока становится лучше
//	improve          local_search.go       выбранный метод улучшения:
//	  iteratedLocalSearch local_search.go
//	  simulatedAnnealing  simulated_annealing.go
//	  tabuSearch          tabu_search.go
//	  geneticAlgorithm    genetic.go
//	  largeNeighborhoodSearch large_neighborhood_search.go
//	insertUnplaced   (ещё раз)             улучшение могло освободить место
//
// # Файлы по ролям
//
// Построение (пары ставятся по одной в черновик):
//
//	draft.go       scheduleDraft — что уже поставлено, кто где занят, сколько часов осталось
//	placement.go   постановка одной пары: слот с наименьшим штрафом, затем аудитория
//	constraints.go жёсткие ограничения: свободен ли слот, подходит ли аудитория
//
// Оценка (что такое «хорошее расписание»):
//
//	penalties.go   веса штрафов и правила: окна, дни с одной парой, суббота, переходы…
//	preferences.go необязательные правила (пожелания): лекция раньше практики и т.п.
//	fitness.go     score с нуля: сумма чётной и нечётной недели (CalculateFitness)
//	explain.go     тот же score, разложенный на отдельные нарушения (ExplainScore)
//	quality.go     показатели в штуках для страницы расписания (CalculateQuality)
//
// Улучшение (быстрые пробные ходы):
//
//	evaluator.go   расписание с быстрой переоценкой хода; основа всех методов улучшения
//
// Прочее: multistart.go — несколько запусков параллельно; unplaced.go — отчёт
// «Не размещено»; rooms.go — подходящие аудитории для переноса пары вручную.
//
// Решения и их причины описаны в docs/adr, веса и правила — в docs/spec/03-algorithm.md.
package solver
