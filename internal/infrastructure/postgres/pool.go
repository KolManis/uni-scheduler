package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database dsn is empty")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	// Повторяем попытки подключения с задержкой
	var pool *pgxpool.Pool
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		err = pool.Ping(ctx)
		if err == nil {
			return pool, nil
		}

		pool.Close()
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("failed to connect after %d retries: %w", maxRetries, err)
}
