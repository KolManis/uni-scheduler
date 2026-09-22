package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutputRepository struct {
	pool *pgxpool.Pool
}

func NewOutputRepository(pool *pgxpool.Pool) *OutputRepository {
	return &OutputRepository{pool: pool}
}

func (r *OutputRepository) SaveSchedule(ctx context.Context, sched *domain.Schedule) (*domain.Schedule, error) {
	assignmentsJSON, err := json.Marshal(sched.Assignments)
	if err != nil {
		return nil, err
	}
	unplacedJSON, err := json.Marshal(sched.Unplaced)
	if err != nil {
		return nil, err
	}

	sched.CreatedAt = time.Now().UTC()

	const query = `
        INSERT INTO schedules (name, assignments, score, unplaced, created_at)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, name, assignments, score, unplaced, created_at
    `

	row := r.pool.QueryRow(ctx, query, sched.Name, assignmentsJSON, sched.Score, unplacedJSON, sched.CreatedAt)

	var result domain.Schedule
	var data, unplacedData []byte
	if err := row.Scan(&result.ID, &result.Name, &data, &result.Score, &unplacedData, &result.CreatedAt); err != nil {
		return nil, err
	}

	if err := decodeScheduleBody(&result, data, unplacedData); err != nil {
		return nil, err
	}
	return &result, nil
}

// decodeScheduleBody разбирает JSONB-колонки assignments и unplaced.
func decodeScheduleBody(s *domain.Schedule, assignments, unplaced []byte) error {
	if err := decodeJSON(assignments, &s.Assignments, fmt.Sprintf("schedule %d assignments", s.ID)); err != nil {
		return err
	}
	return decodeJSON(unplaced, &s.Unplaced, fmt.Sprintf("schedule %d unplaced", s.ID))
}

func (r *OutputRepository) GetSchedule(ctx context.Context, id int64) (*domain.Schedule, error) {
	const query = `
        SELECT id, name, assignments, score, unplaced, created_at
        FROM schedules
        WHERE id = $1
    `

	row := r.pool.QueryRow(ctx, query, id)

	var result domain.Schedule
	var data, unplacedData []byte
	if err := row.Scan(&result.ID, &result.Name, &data, &result.Score, &unplacedData, &result.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	if err := decodeScheduleBody(&result, data, unplacedData); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *OutputRepository) ListSchedules(ctx context.Context) ([]domain.ScheduleSummary, error) {
	const query = `
        SELECT id, name, score, created_at,
               jsonb_array_length(assignments) AS total_pairs,
               jsonb_array_length(COALESCE(unplaced, '[]'::jsonb)) AS unplaced_count
        FROM schedules
        ORDER BY score ASC, created_at DESC
        LIMIT 100
    `

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.ScheduleSummary
	for rows.Next() {
		var s domain.ScheduleSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Score, &s.CreatedAt, &s.TotalPairs, &s.UnplacedCount); err != nil {
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
		return domain.ErrNotFound
	}
	return nil
}

func (r *OutputRepository) UpdateSchedule(ctx context.Context, sched *domain.Schedule) error {
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
		return domain.ErrNotFound
	}
	return nil
}
