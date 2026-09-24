//go:build integration

package postgres

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KolManis/uni-scheduler/migrations"
)

// Запуск: go test -tags integration ./internal/adapters/out/postgres/
// Нужен Postgres (make db). Адрес сервера — TEST_DATABASE_DSN, по умолчанию локальный из
// docker-compose. Тест создаёт временные базы и удаляет их после себя.
func TestMigrate(t *testing.T) {
	tests := []struct {
		name string
		// Как база была создана до запуска приложения: файлы миграций, выполненные
		// напрямую, как раньше делал Postgres при создании тома. Пусто — пустая база.
		legacyFiles []string
		runs        int  // сколько раз запустить Migrate
		withData    bool // до миграции в базе есть сохранённое расписание
	}{
		{
			name: "пустая база — создаются все таблицы",
			runs: 1,
		},
		{
			name: "повторный запуск ничего не меняет",
			runs: 2,
		},
		{
			name: "старая база без 0006–0008 — колонки добавляются, расписания сохраняются",
			legacyFiles: []string{
				"0001_init.up.sql",
				"0003_add_semester_half_and_required_building.up.sql",
				"0004_add_unplaced_to_schedules.up.sql",
				"0005_add_schedule_options.up.sql",
			},
			runs:     1,
			withData: true,
		},
		{
			name: "старая база со всеми колонками — принимается как есть",
			legacyFiles: []string{
				"0001_init.up.sql",
				"0003_add_semester_half_and_required_building.up.sql",
				"0004_add_unplaced_to_schedules.up.sql",
				"0005_add_schedule_options.up.sql",
				"0006_add_teacher_external_pairs.up.sql",
				"0007_add_teacher_undesired_slots.up.sql",
				"0008_add_schedule_run.up.sql",
			},
			runs:     1,
			withData: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := tempDatabase(t)

			for _, name := range tt.legacyFiles {
				sql, err := migrations.FS.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, string(sql)); err != nil {
					t.Fatalf("старая схема, %s: %v", name, err)
				}
			}
			if tt.withData {
				if _, err := pool.Exec(ctx, `INSERT INTO schedules (name) VALUES ('сохранённое')`); err != nil {
					t.Fatal(err)
				}
			}

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			for run := 1; run <= tt.runs; run++ {
				if err := Migrate(ctx, pool, logger); err != nil {
					t.Fatalf("запуск %d: %v", run, err)
				}
			}

			wantVersions := []int64{1, 3, 4, 5, 6, 7, 8}
			if got := appliedVersions(t, pool); !slices.Equal(got, wantVersions) {
				t.Errorf("применённые версии %v, ожидалось %v", got, wantVersions)
			}
			for _, col := range []string{
				"subject_plans.semester_half", "schedules.unplaced", "schedules.options",
				"teachers.external_pairs", "teachers.undesired_slots", "schedules.run",
			} {
				if !hasColumn(t, pool, col) {
					t.Errorf("нет колонки %s", col)
				}
			}
			var buildings int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM buildings`).Scan(&buildings); err != nil {
				t.Fatal(err)
			}
			if buildings != 2 {
				t.Errorf("корпусов %d, ожидалось 2 демо-корпуса из 0001 (без дублей)", buildings)
			}
			if tt.withData {
				var name string
				if err := pool.QueryRow(ctx, `SELECT name FROM schedules`).Scan(&name); err != nil {
					t.Fatalf("сохранённое расписание пропало: %v", err)
				}
			}
		})
	}
}

// tempDatabase создаёт пустую базу на сервере из TEST_DATABASE_DSN и удаляет её после теста.
func tempDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	server := os.Getenv("TEST_DATABASE_DSN")
	if server == "" {
		server = "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"
	}
	admin, err := pgxpool.New(ctx, server)
	if err != nil {
		t.Skipf("нет Postgres: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("нет Postgres: %v", err)
	}

	name := fmt.Sprintf("migrate_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(server)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+name); err != nil {
			t.Errorf("удалить временную базу %s: %v", name, err)
		}
		admin.Close()
	})
	return pool
}

func appliedVersions(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT version_id FROM goose_db_version WHERE is_applied AND version_id > 0 ORDER BY version_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// hasColumn — есть ли колонка «таблица.колонка».
func hasColumn(t *testing.T, pool *pgxpool.Pool, tableColumn string) bool {
	t.Helper()
	var table, column string
	for i := range tableColumn {
		if tableColumn[i] == '.' {
			table, column = tableColumn[:i], tableColumn[i+1:]
		}
	}
	var ok bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)`,
		table, column).Scan(&ok)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}
