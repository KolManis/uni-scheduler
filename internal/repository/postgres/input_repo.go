package postgres

import (
	"context"
	"encoding/json"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InputRepository struct {
	pool *pgxpool.Pool
}

func NewInputRepository(pool *pgxpool.Pool) *InputRepository {
	return &InputRepository{pool: pool}
}

func (r *InputRepository) LoadInput(ctx context.Context) (*schedule.InputData, error) {
	var data schedule.InputData

	// Здания
	rows, err := r.pool.Query(ctx, `SELECT id, name, address FROM buildings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b schedule.Building
		if err := rows.Scan(&b.ID, &b.Name, &b.Address); err != nil {
			return nil, err
		}
		data.Buildings = append(data.Buildings, b)
	}

	// Кафедры
	rows2, err := r.pool.Query(ctx, `SELECT id, name FROM departments`)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var d schedule.Department
		if err := rows2.Scan(&d.ID, &d.Name); err != nil {
			return nil, err
		}
		data.Departments = append(data.Departments, d)
	}

	// Группы
	rows3, err := r.pool.Query(ctx, `SELECT id, name, student_count, building_ids FROM groups`)
	if err != nil {
		return nil, err
	}
	defer rows3.Close()
	for rows3.Next() {
		var g schedule.Group
		var bidsJSON []byte
		if err := rows3.Scan(&g.ID, &g.Name, &g.StudentCount, &bidsJSON); err != nil {
			return nil, err
		}
		if bidsJSON != nil {
			json.Unmarshal(bidsJSON, &g.BuildingIDs)
		}
		data.Groups = append(data.Groups, g)
	}

	// Преподаватели
	rows4, err := r.pool.Query(ctx, `
		SELECT id, name, department_id, max_weekly_hours, unavailable_slots, preferred_buildings
		FROM teachers
	`)
	if err != nil {
		return nil, err
	}
	defer rows4.Close()
	for rows4.Next() {
		var t schedule.Teacher
		var uslotsJSON, pbJSON []byte
		if err := rows4.Scan(&t.ID, &t.Name, &t.DepartmentID, &t.MaxWeeklyHours,
			&uslotsJSON, &pbJSON); err != nil {
			return nil, err
		}
		if uslotsJSON != nil {
			json.Unmarshal(uslotsJSON, &t.UnavailableSlots)
		}
		if pbJSON != nil {
			json.Unmarshal(pbJSON, &t.PreferredBuildings)
		}
		data.Teachers = append(data.Teachers, t)
	}

	// Аудитории
	rows5, err := r.pool.Query(ctx, `
		SELECT id, number, building_id, capacity, type FROM rooms
	`)
	if err != nil {
		return nil, err
	}
	defer rows5.Close()
	for rows5.Next() {
		var rm schedule.Room
		if err := rows5.Scan(&rm.ID, &rm.Number, &rm.BuildingID, &rm.Capacity, &rm.Type); err != nil {
			return nil, err
		}
		data.Rooms = append(data.Rooms, rm)
	}

	// Предметы
	rows6, err := r.pool.Query(ctx, `
		SELECT id, name, department_id, lecture_hours, practice_hours,
			   lab_hours, requires_room_type, teacher_id, group_ids, parity
		FROM subject_plans
	`)
	if err != nil {
		return nil, err
	}
	defer rows6.Close()
	for rows6.Next() {
		var sp schedule.SubjectPlan
		var gidsJSON []byte
		var parityStr string
		if err := rows6.Scan(&sp.ID, &sp.Name, &sp.DepartmentID,
			&sp.LectureHours, &sp.PracticeHours, &sp.LabHours,
			&sp.RequiresRoomType, &sp.TeacherID, &gidsJSON, &parityStr); err != nil {
			return nil, err
		}
		if gidsJSON != nil {
			json.Unmarshal(gidsJSON, &sp.GroupIDs)
		}
		sp.Parity = schedule.Parity(parityStr)
		if sp.Parity == "" {
			sp.Parity = schedule.Always
		}
		data.SubjectPlans = append(data.SubjectPlans, sp)
	}

	return &data, nil
}
