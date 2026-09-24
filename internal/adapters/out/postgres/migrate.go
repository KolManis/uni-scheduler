package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/KolManis/uni-scheduler/migrations"
)

// Migrate приводит схему базы к текущей версии: применяет миграции, которых в базе ещё нет.
// Вызывается при старте приложения, поэтому отдельный шаг «накатить миграции» не нужен.
// Какие версии применены, goose хранит в таблице goose_db_version.
//
// Базы, созданные до перехода на goose (миграции выполнял Postgres при создании тома),
// сначала отмечаются как базы с версией 0001 — см. adoptLegacySchema.
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	// Закрытие *sql.DB не закрывает пул приложения (см. stdlib.OpenDBFromPool).
	db := stdlib.OpenDBFromPool(pool)
	defer func() {
		if err := db.Close(); err != nil {
			logger.Warn("close migrations db", "error", err)
		}
	}()

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}

	adopted, err := adoptLegacySchema(ctx, pool, db)
	if err != nil {
		return err
	}
	if adopted {
		logger.Info("migrations: existing schema adopted as version 1")
	}

	before, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	after, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	logger.Info("migrations applied", "from", before, "to", after)
	return nil
}

// adoptLegacySchema — переход для баз, созданных до goose. В них таблицы уже есть, а записи
// о версиях нет, и goose начал бы с 0001: она создаёт таблицы (IF NOT EXISTS) и вставляет
// демо-справочники — вставка упала бы на повторяющихся ключах. Поэтому 0001 отмечается как
// применённая без выполнения. Остальные миграции (0003+) идемпотентны: goose применит их
// поверх, даже если часть из них в базе уже была, — так доедут и забытые 0007–0008.
//
// Признак старой базы — таблица schedules есть, а версия 1 не записана.
func adoptLegacySchema(ctx context.Context, pool *pgxpool.Pool, db *sql.DB) (bool, error) {
	var hasSchedules, hasVersions bool
	err := pool.QueryRow(ctx,
		`SELECT to_regclass('public.schedules') IS NOT NULL, to_regclass('public.goose_db_version') IS NOT NULL`,
	).Scan(&hasSchedules, &hasVersions)
	if err != nil {
		return false, fmt.Errorf("inspect schema: %w", err)
	}
	if !hasSchedules {
		return false, nil // пустая база: goose создаст всё с 0001
	}

	if hasVersions {
		var hasInit bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 1 AND is_applied)`,
		).Scan(&hasInit)
		if err != nil {
			return false, fmt.Errorf("read goose_db_version: %w", err)
		}
		if hasInit {
			return false, nil // база уже под goose
		}
	}

	if _, err := goose.EnsureDBVersionContext(ctx, db); err != nil {
		return false, fmt.Errorf("create goose_db_version: %w", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (1, true)`); err != nil {
		return false, fmt.Errorf("mark version 1 as applied: %w", err)
	}
	return true, nil
}
