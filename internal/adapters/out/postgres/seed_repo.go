package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// ErrSchedulesExist — в базе есть расписания: замена справочников сломала бы их ссылки на
// преподавателей, группы и аудитории.
var ErrSchedulesExist = errors.New("в базе есть расписания")

// ReplaceReferences заменяет все справочники (корпуса, кафедры, аудитории, группы,
// преподаватели, учебные планы) на in, сохраняя их ID. Нужна для первого запуска: новая
// база получает демо-справочники из миграции 0001, их заменяют данными кафедры.
//
// Если в базе есть расписания, без force ничего не делает и возвращает ErrSchedulesExist:
// пары расписаний ссылаются на ID справочников, и после замены перестали бы находить
// своих преподавателей и аудитории.
//
// Замена не в одной транзакции: если прервётся на середине, повторный запуск начнёт заново
// с очистки, поэтому справочники всё равно придут в целостное состояние.
func (r *RefWriteRepository) ReplaceReferences(ctx context.Context, in domain.InputData, force bool) error {
	var schedules int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM schedules`).Scan(&schedules); err != nil {
		return fmt.Errorf("count schedules: %w", err)
	}
	if schedules > 0 && !force {
		return fmt.Errorf("%w (%d)", ErrSchedulesExist, schedules)
	}

	if _, err := r.pool.Exec(ctx,
		`TRUNCATE subject_plans, teachers, rooms, groups, departments, buildings CASCADE`); err != nil {
		return fmt.Errorf("truncate references: %w", err)
	}
	// Порядок — от тех, на кого ссылаются, к тем, кто ссылается.
	for _, b := range in.Buildings {
		if err := r.InsertBuilding(ctx, b); err != nil {
			return fmt.Errorf("insert building %s: %w", b.ID, err)
		}
	}
	for _, d := range in.Departments {
		if err := r.InsertDepartment(ctx, d); err != nil {
			return fmt.Errorf("insert department %s: %w", d.ID, err)
		}
	}
	for _, rm := range in.Rooms {
		if err := r.InsertRoom(ctx, rm); err != nil {
			return fmt.Errorf("insert room %s: %w", rm.ID, err)
		}
	}
	for _, g := range in.Groups {
		if err := r.InsertGroup(ctx, g); err != nil {
			return fmt.Errorf("insert group %s: %w", g.ID, err)
		}
	}
	for _, t := range in.Teachers {
		if err := r.InsertTeacher(ctx, t); err != nil {
			return fmt.Errorf("insert teacher %s: %w", t.ID, err)
		}
	}
	for _, sp := range in.SubjectPlans {
		if err := r.InsertSubjectPlan(ctx, sp); err != nil {
			return fmt.Errorf("insert subject plan %s: %w", sp.ID, err)
		}
	}
	return nil
}
