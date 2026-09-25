package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KolManis/uni-scheduler/data/snapshots"
	"github.com/KolManis/uni-scheduler/internal/adapters/out/postgres"
)

// runSeed — команда `scheduler seed [-force]`: заменить справочники в базе данными кафедры,
// встроенными в бинарник (data/snapshots). Нужна на новой базе: после первого запуска в ней
// демо-справочники из миграции 0001. Если в базе уже есть расписания, без -force отказывается.
func runSeed(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, args []string) error {
	flags := flag.NewFlagSet("seed", flag.ContinueOnError)
	force := flags.Bool("force", false, "заменить справочники, даже если в базе есть расписания (они перестанут открываться)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	data, err := snapshots.Load(snapshots.FS)
	if err != nil {
		return fmt.Errorf("load embedded snapshots: %w", err)
	}
	err = postgres.NewRefWriteRepository(pool).ReplaceReferences(ctx, data, *force)
	if errors.Is(err, postgres.ErrSchedulesExist) {
		return fmt.Errorf("%w: справочники не заменены — ссылки расписаний сломались бы; "+
			"если это точно нужно, запустите с -force", err)
	}
	if err != nil {
		return err
	}
	logger.Info("references loaded",
		"buildings", len(data.Buildings), "departments", len(data.Departments), "rooms", len(data.Rooms),
		"groups", len(data.Groups), "teachers", len(data.Teachers), "subject_plans", len(data.SubjectPlans))
	return nil
}
