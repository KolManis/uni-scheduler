//go:build integration

package postgres

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/KolManis/uni-scheduler/data/snapshots"
)

// Загрузка справочников кафедры в новую базу (scheduler seed): демо-справочники из 0001
// заменяются, а при сохранённых расписаниях без force ничего не трогается.
func TestReplaceReferences(t *testing.T) {
	tests := []struct {
		name          string
		withSchedule  bool
		force         bool
		wantErr       error
		wantBuildings int // корпусов после вызова
	}{
		{
			name:          "новая база — демо-справочники заменяются данными кафедры",
			wantBuildings: 2,
		},
		{
			name:          "есть расписание — без force отказ, справочники не тронуты",
			withSchedule:  true,
			wantErr:       ErrSchedulesExist,
			wantBuildings: 2,
		},
		{
			name:          "есть расписание, force — справочники заменяются",
			withSchedule:  true,
			force:         true,
			wantBuildings: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := tempDatabase(t)
			if err := Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
				t.Fatal(err)
			}
			if tt.withSchedule {
				if _, err := pool.Exec(ctx, `INSERT INTO schedules (name) VALUES ('сохранённое')`); err != nil {
					t.Fatal(err)
				}
			}
			data, err := snapshots.Load(snapshots.FS)
			if err != nil {
				t.Fatal(err)
			}

			err = NewRefWriteRepository(pool).ReplaceReferences(ctx, data, tt.force)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ошибка %v, ожидалась %v", err, tt.wantErr)
			}

			var demo, rooms int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM buildings WHERE id = 'B1'`).Scan(&demo); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM rooms`).Scan(&rooms); err != nil {
				t.Fatal(err)
			}
			replaced := tt.wantErr == nil
			if replaced && (demo != 0 || rooms != len(data.Rooms)) {
				t.Errorf("после замены: демо-корпус B1 %d шт., аудиторий %d из %d", demo, rooms, len(data.Rooms))
			}
			if !replaced && demo != 1 {
				t.Errorf("при отказе справочники изменились: демо-корпус B1 %d шт.", demo)
			}
		})
	}
}
