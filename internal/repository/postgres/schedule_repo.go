package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// убеждаемся что time используется (для ScheduleSummary)
var _ = time.Time{}

type OutputRepository struct {
	pool *pgxpool.Pool
}

func NewOutputRepository(pool *pgxpool.Pool) *OutputRepository {
	return &OutputRepository{pool: pool}
}

func (r *OutputRepository) SaveSchedule(ctx context.Context, sched *schedule.Schedule) (*schedule.Schedule, error) {
	assignmentsJSON, err := json.Marshal(sched.Assignments)
	if err != nil {
		return nil, err
	}

	sched.CreatedAt = time.Now().UTC()

	const query = `
        INSERT INTO schedules (name, assignments, score, created_at)
        VALUES ($1, $2, $3, $4)
        RETURNING id, name, assignments, score, created_at
    `

	row := r.pool.QueryRow(ctx, query, sched.Name, assignmentsJSON, sched.Score, sched.CreatedAt)

	var result schedule.Schedule
	var data []byte
	if err := row.Scan(&result.ID, &result.Name, &data, &result.Score, &result.CreatedAt); err != nil {
		return nil, err
	}

	json.Unmarshal(data, &result.Assignments)
	return &result, nil
}

func (r *OutputRepository) GetSchedule(ctx context.Context, id int64) (*schedule.Schedule, error) {
	const query = `
        SELECT id, name, assignments, score, created_at
        FROM schedules
        WHERE id = $1
    `

	row := r.pool.QueryRow(ctx, query, id)

	var result schedule.Schedule
	var data []byte
	if err := row.Scan(&result.ID, &result.Name, &data, &result.Score, &result.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, schedule.ErrNotFound
		}
		return nil, err
	}

	json.Unmarshal(data, &result.Assignments)
	return &result, nil
}

// ScheduleSummary — расписание без тела assignments (для списка).
type ScheduleSummary struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Score      int       `json:"score"`
	TotalPairs int       `json:"total_pairs"`
	CreatedAt  time.Time `json:"created_at"`
}

func (r *OutputRepository) ListSchedules(ctx context.Context) ([]ScheduleSummary, error) {
	const query = `
        SELECT id, name, score, created_at,
               jsonb_array_length(assignments) AS total_pairs
        FROM schedules
        ORDER BY score ASC, created_at DESC
        LIMIT 100
    `

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ScheduleSummary
	for rows.Next() {
		var s ScheduleSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Score, &s.CreatedAt, &s.TotalPairs); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *OutputRepository) DeleteSchedule(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM schedules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return schedule.ErrNotFound
	}
	return nil
}

func (r *OutputRepository) UpdateSchedule(ctx context.Context, sched *schedule.Schedule) error {
	assignmentsJSON, err := json.Marshal(sched.Assignments)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE schedules SET assignments = $1, score = $2 WHERE id = $3`,
		assignmentsJSON, sched.Score, sched.ID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return schedule.ErrNotFound
	}
	return nil
}
