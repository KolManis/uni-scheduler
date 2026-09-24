package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// decodeJSON разбирает JSONB-колонку в dst.
//
// NULL в базе (raw == nil) не считается ошибкой — поле просто остаётся нулевым: так ведут себя,
// например, расписания, сохранённые до появления колонки unplaced. А вот повреждённый JSON —
// ошибка: раньше такие ошибки молча отбрасывались, и запись загружалась с пустым полем, что
// незаметно ломало ограничения (скажем, у преподавателя «пропадали» недоступные слоты).
func decodeJSON(raw []byte, dst any, what string) error {
	if raw == nil {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode %s: %w", what, err)
	}
	return nil
}

type InputRepository struct {
	pool *pgxpool.Pool
}

func NewInputRepository(pool *pgxpool.Pool) *InputRepository {
	return &InputRepository{pool: pool}
}

func (r *InputRepository) LoadInput(ctx context.Context) (*domain.InputData, error) {
	var data domain.InputData

	// Здания
	rows, err := r.pool.Query(ctx, `SELECT id, name, address FROM buildings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b domain.Building
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
		var d domain.Department
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
		var g domain.Group
		var bidsJSON []byte
		if err := rows3.Scan(&g.ID, &g.Name, &g.StudentCount, &bidsJSON); err != nil {
			return nil, err
		}
		if err := decodeJSON(bidsJSON, &g.BuildingIDs, "group "+g.ID+" building_ids"); err != nil {
			return nil, err
		}
		data.Groups = append(data.Groups, g)
	}

	// Преподаватели
	rows4, err := r.pool.Query(ctx, `
		SELECT id, name, department_id, max_weekly_hours, unavailable_slots, preferred_buildings, external_pairs, undesired_slots
		FROM teachers
	`)
	if err != nil {
		return nil, err
	}
	defer rows4.Close()
	for rows4.Next() {
		var t domain.Teacher
		var uslotsJSON, pbJSON, extJSON, undJSON []byte
		if err := rows4.Scan(&t.ID, &t.Name, &t.DepartmentID, &t.MaxWeeklyHours,
			&uslotsJSON, &pbJSON, &extJSON, &undJSON); err != nil {
			return nil, err
		}
		if err := decodeJSON(uslotsJSON, &t.UnavailableSlots, "teacher "+t.ID+" unavailable_slots"); err != nil {
			return nil, err
		}
		if err := decodeJSON(pbJSON, &t.PreferredBuildings, "teacher "+t.ID+" preferred_buildings"); err != nil {
			return nil, err
		}
		if err := decodeJSON(undJSON, &t.UndesiredSlots, "teacher "+t.ID+" undesired_slots"); err != nil {
			return nil, err
		}
		if err := decodeJSON(extJSON, &t.ExternalPairs, "teacher "+t.ID+" external_pairs"); err != nil {
			return nil, err
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
		var rm domain.Room
		if err := rows5.Scan(&rm.ID, &rm.Number, &rm.BuildingID, &rm.Capacity, &rm.Type); err != nil {
			return nil, err
		}
		data.Rooms = append(data.Rooms, rm)
	}

	// Предметы
	rows6, err := r.pool.Query(ctx, `
		SELECT id, name, department_id, lecture_hours, practice_hours,
			   lab_hours, requires_room_type,
			   COALESCE(required_building_id, ''),
			   teacher_id, group_ids, parity,
			   COALESCE(semester_half, 'full')
		FROM subject_plans
	`)
	if err != nil {
		return nil, err
	}
	defer rows6.Close()
	for rows6.Next() {
		var sp domain.SubjectPlan
		var gidsJSON []byte
		var parityStr, halfStr string
		if err := rows6.Scan(&sp.ID, &sp.Name, &sp.DepartmentID,
			&sp.LectureHours, &sp.PracticeHours, &sp.LabHours,
			&sp.RequiresRoomType, &sp.RequiredBuildingID,
			&sp.TeacherID, &gidsJSON, &parityStr, &halfStr); err != nil {
			return nil, err
		}
		if err := decodeJSON(gidsJSON, &sp.GroupIDs, "subject plan "+sp.ID+" group_ids"); err != nil {
			return nil, err
		}
		sp.Parity = domain.Parity(parityStr)
		if sp.Parity == "" {
			sp.Parity = domain.Always
		}
		sp.SemesterHalf = domain.SemesterHalf(halfStr)
		if sp.SemesterHalf == "" {
			sp.SemesterHalf = domain.HalfFull
		}
		data.SubjectPlans = append(data.SubjectPlans, sp)
	}

	return &data, nil
}
