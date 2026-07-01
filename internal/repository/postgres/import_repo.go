package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/KolManis/uni-scheduler/internal/importer"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ImportRepository выполняет UPSERT сущностей, извлечённых из Excel.
type ImportRepository struct {
	pool *pgxpool.Pool
}

func NewImportRepository(pool *pgxpool.Pool) *ImportRepository {
	return &ImportRepository{pool: pool}
}

// UpsertAll сохраняет всё из ImportedData, возвращает ImportResult.
func (r *ImportRepository) UpsertAll(ctx context.Context, data *importer.ImportedData) (*importer.ImportResult, error) {
	result := &importer.ImportResult{}

	// Собираем уникальные сущности из занятий
	buildings := map[string]bool{}  // buildingID → exists
	teachers := map[string]string{} // teacherName → id (slug)
	groups := map[string]bool{}     // groupName → exists
	rooms := map[string]string{}    // roomNumber+buildingID → id

	for _, l := range data.Lessons {
		buildings[l.BuildingID] = true
		teachers[l.TeacherName] = slugify(l.TeacherName)
		for _, g := range l.GroupNames {
			groups[g] = true
		}
		rk := l.RoomNumber + "@" + l.BuildingID
		rooms[rk] = slugify(l.RoomNumber + "_" + l.BuildingID)
	}

	// 1. Корпуса
	for bid := range buildings {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO buildings (id, name, address) VALUES ($1, $2, '')
             ON CONFLICT (id) DO NOTHING`,
			bid, "Корпус "+bid,
		)
		if err != nil {
			return nil, fmt.Errorf("upsert building %q: %w", bid, err)
		}
	}

	// 2. Кафедра по умолчанию (ИТАС)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO departments (id, name) VALUES ('itas', 'ИТАС')
         ON CONFLICT (id) DO NOTHING`,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert department: %w", err)
	}

	// 3. Преподаватели
	for name, id := range teachers {
		tag, err := r.pool.Exec(ctx,
			`INSERT INTO teachers (id, name, department_id, max_weekly_hours)
             VALUES ($1, $2, 'itas', 18)
             ON CONFLICT (id) DO NOTHING`,
			id, name,
		)
		if err != nil {
			return nil, fmt.Errorf("upsert teacher %q: %w", name, err)
		}
		if tag.RowsAffected() > 0 {
			result.TeachersCreated++
		}
	}

	// 4. Группы
	for name := range groups {
		id := slugify(name)
		tag, err := r.pool.Exec(ctx,
			`INSERT INTO groups (id, name, student_count, building_ids)
             VALUES ($1, $2, 25, '["A"]')
             ON CONFLICT (id) DO NOTHING`,
			id, name,
		)
		if err != nil {
			return nil, fmt.Errorf("upsert group %q: %w", name, err)
		}
		if tag.RowsAffected() > 0 {
			result.GroupsCreated++
		}
	}

	// 5. Аудитории
	for rk, id := range rooms {
		parts := strings.SplitN(rk, "@", 2)
		roomNum := parts[0]
		buildingID := ""
		if len(parts) == 2 {
			buildingID = parts[1]
		}
		tag, err := r.pool.Exec(ctx,
			`INSERT INTO rooms (id, number, building_id, capacity, type)
             VALUES ($1, $2, $3, 30, 'lecture')
             ON CONFLICT (id) DO NOTHING`,
			id, roomNum, buildingID,
		)
		if err != nil {
			return nil, fmt.Errorf("upsert room %q: %w", roomNum, err)
		}
		if tag.RowsAffected() > 0 {
			result.RoomsCreated++
		}
	}

	// 6. Учебные планы — группируем по (teacherName, subjectName, parity)
	type planKey struct {
		teacher string
		subject string
		parity  string
	}
	type planData struct {
		groupNames map[string]bool
		roomType   schedule.ClassType
	}
	plans := map[planKey]*planData{}

	for _, l := range data.Lessons {
		k := planKey{
			teacher: teachers[l.TeacherName],
			subject: l.SubjectName,
			parity:  string(l.Parity),
		}
		if plans[k] == nil {
			plans[k] = &planData{groupNames: map[string]bool{}}
		}
		for _, g := range l.GroupNames {
			plans[k].groupNames[g] = true
		}
	}

	for k, pd := range plans {
		groupIDs := make([]string, 0, len(pd.groupNames))
		for g := range pd.groupNames {
			groupIDs = append(groupIDs, slugify(g))
		}

		planID := slugify(k.teacher + "_" + k.subject + "_" + k.parity)

		groupIDsJSON := toJSONArray(groupIDs)

		tag, err := r.pool.Exec(ctx,
			`INSERT INTO subject_plans
                 (id, name, department_id, teacher_id, group_ids,
                  lecture_hours, practice_hours, lab_hours,
                  requires_room_type, parity)
             VALUES ($1, $2, 'itas', $3, $4, 2, 2, 2, 'lecture', $5)
             ON CONFLICT (id) DO NOTHING`,
			planID, k.subject, k.teacher, groupIDsJSON, k.parity,
		)
		if err != nil {
			return nil, fmt.Errorf("upsert plan %q: %w", k.subject, err)
		}
		if tag.RowsAffected() > 0 {
			result.SubjectsCreated++
		}
	}

	return result, nil
}

// slugify преобразует строку в безопасный ID: нижний регистр, пробелы → '_'.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "-", "_")
	// убираем повторные подчёркивания
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "_")
	return s
}

// toJSONArray формирует JSONB-массив строк.
func toJSONArray(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = `"` + id + `"`
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
