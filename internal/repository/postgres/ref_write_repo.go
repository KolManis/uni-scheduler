package postgres

import (
	"context"
	"encoding/json"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RefWriteRepository struct {
	pool *pgxpool.Pool
}

func NewRefWriteRepository(pool *pgxpool.Pool) *RefWriteRepository {
	return &RefWriteRepository{pool: pool}
}

// ── Buildings ────────────────────────────────────────────────────────────────

func (r *RefWriteRepository) InsertBuilding(ctx context.Context, b schedule.Building) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO buildings (id, name, address) VALUES ($1, $2, $3)`,
		b.ID, b.Name, b.Address)
	return err
}

func (r *RefWriteRepository) UpdateBuilding(ctx context.Context, b schedule.Building) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE buildings SET name = $2, address = $3 WHERE id = $1`,
		b.ID, b.Name, b.Address)
	return tag.RowsAffected() > 0, err
}

func (r *RefWriteRepository) DeleteBuilding(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM buildings WHERE id = $1`, id)
	return tag.RowsAffected() > 0, err
}

// ── Departments ───────────────────────────────────────────────────────────────

func (r *RefWriteRepository) InsertDepartment(ctx context.Context, d schedule.Department) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO departments (id, name) VALUES ($1, $2)`,
		d.ID, d.Name)
	return err
}

func (r *RefWriteRepository) UpdateDepartment(ctx context.Context, d schedule.Department) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE departments SET name = $2 WHERE id = $1`,
		d.ID, d.Name)
	return tag.RowsAffected() > 0, err
}

func (r *RefWriteRepository) DeleteDepartment(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM departments WHERE id = $1`, id)
	return tag.RowsAffected() > 0, err
}

// ── Rooms ─────────────────────────────────────────────────────────────────────

func (r *RefWriteRepository) InsertRoom(ctx context.Context, rm schedule.Room) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO rooms (id, number, building_id, capacity, type) VALUES ($1, $2, $3, $4, $5)`,
		rm.ID, rm.Number, rm.BuildingID, rm.Capacity, rm.Type)
	return err
}

func (r *RefWriteRepository) UpdateRoom(ctx context.Context, rm schedule.Room) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rooms SET number = $2, building_id = $3, capacity = $4, type = $5 WHERE id = $1`,
		rm.ID, rm.Number, rm.BuildingID, rm.Capacity, rm.Type)
	return tag.RowsAffected() > 0, err
}

func (r *RefWriteRepository) DeleteRoom(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM rooms WHERE id = $1`, id)
	return tag.RowsAffected() > 0, err
}

// ── Groups ────────────────────────────────────────────────────────────────────

func (r *RefWriteRepository) InsertGroup(ctx context.Context, g schedule.Group) error {
	bids, err := json.Marshal(g.BuildingIDs)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO groups (id, name, student_count, building_ids) VALUES ($1, $2, $3, $4)`,
		g.ID, g.Name, g.StudentCount, bids)
	return err
}

func (r *RefWriteRepository) UpdateGroup(ctx context.Context, g schedule.Group) (bool, error) {
	bids, err := json.Marshal(g.BuildingIDs)
	if err != nil {
		return false, err
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE groups SET name = $2, student_count = $3, building_ids = $4 WHERE id = $1`,
		g.ID, g.Name, g.StudentCount, bids)
	return tag.RowsAffected() > 0, err
}

func (r *RefWriteRepository) DeleteGroup(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, id)
	return tag.RowsAffected() > 0, err
}

// ── Teachers ──────────────────────────────────────────────────────────────────

func (r *RefWriteRepository) InsertTeacher(ctx context.Context, t schedule.Teacher) error {
	uslots, _ := json.Marshal(t.UnavailableSlots)
	pbuilds, _ := json.Marshal(t.PreferredBuildings)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO teachers (id, name, department_id, max_weekly_hours, unavailable_slots, preferred_buildings)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.Name, t.DepartmentID, t.MaxWeeklyHours, uslots, pbuilds)
	return err
}

func (r *RefWriteRepository) UpdateTeacher(ctx context.Context, t schedule.Teacher) (bool, error) {
	uslots, _ := json.Marshal(t.UnavailableSlots)
	pbuilds, _ := json.Marshal(t.PreferredBuildings)
	tag, err := r.pool.Exec(ctx, `
		UPDATE teachers SET name = $2, department_id = $3, max_weekly_hours = $4,
		    unavailable_slots = $5, preferred_buildings = $6 WHERE id = $1`,
		t.ID, t.Name, t.DepartmentID, t.MaxWeeklyHours, uslots, pbuilds)
	return tag.RowsAffected() > 0, err
}

func (r *RefWriteRepository) DeleteTeacher(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM teachers WHERE id = $1`, id)
	return tag.RowsAffected() > 0, err
}

// ── SubjectPlans ──────────────────────────────────────────────────────────────

func (r *RefWriteRepository) InsertSubjectPlan(ctx context.Context, sp schedule.SubjectPlan) error {
	gids, _ := json.Marshal(sp.GroupIDs)
	parity := string(sp.Parity)
	if parity == "" {
		parity = string(schedule.Always)
	}
	half := string(sp.SemesterHalf)
	if half == "" {
		half = string(schedule.HalfFull)
	}
	var reqBldPtr *string
	if sp.RequiredBuildingID != "" {
		reqBldPtr = &sp.RequiredBuildingID
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO subject_plans
			(id, name, department_id, lecture_hours, practice_hours, lab_hours,
			 requires_room_type, required_building_id, teacher_id, group_ids, parity, semester_half)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		sp.ID, sp.Name, sp.DepartmentID,
		sp.LectureHours, sp.PracticeHours, sp.LabHours,
		sp.RequiresRoomType, reqBldPtr,
		sp.TeacherID, gids, parity, half)
	return err
}

func (r *RefWriteRepository) UpdateSubjectPlan(ctx context.Context, sp schedule.SubjectPlan) (bool, error) {
	gids, _ := json.Marshal(sp.GroupIDs)
	parity := string(sp.Parity)
	if parity == "" {
		parity = string(schedule.Always)
	}
	half := string(sp.SemesterHalf)
	if half == "" {
		half = string(schedule.HalfFull)
	}
	var reqBldPtr *string
	if sp.RequiredBuildingID != "" {
		reqBldPtr = &sp.RequiredBuildingID
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE subject_plans SET
			name = $2, department_id = $3, lecture_hours = $4, practice_hours = $5,
			lab_hours = $6, requires_room_type = $7, required_building_id = $8,
			teacher_id = $9, group_ids = $10, parity = $11, semester_half = $12
		WHERE id = $1`,
		sp.ID, sp.Name, sp.DepartmentID,
		sp.LectureHours, sp.PracticeHours, sp.LabHours,
		sp.RequiresRoomType, reqBldPtr,
		sp.TeacherID, gids, parity, half)
	return tag.RowsAffected() > 0, err
}

func (r *RefWriteRepository) DeleteSubjectPlan(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM subject_plans WHERE id = $1`, id)
	return tag.RowsAffected() > 0, err
}
