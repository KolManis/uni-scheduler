package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

func (r *OutputRepository) ListSchedules(ctx context.Context) ([]schedule.Schedule, error) {
	const query = `
        SELECT id, name, assignments, score, created_at
        FROM schedules
        ORDER BY score ASC, created_at DESC
        LIMIT 100
    `

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []schedule.Schedule
	for rows.Next() {
		var s schedule.Schedule
		var data []byte
		if err := rows.Scan(&s.ID, &s.Name, &data, &s.Score, &s.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(data, &s.Assignments)
		schedules = append(schedules, s)
	}

	return schedules, rows.Err()
}
