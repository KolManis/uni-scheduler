package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpTransport "github.com/KolManis/uni-scheduler/internal/adapters/in/http"
	"github.com/KolManis/uni-scheduler/internal/adapters/in/http/rest"
	"github.com/KolManis/uni-scheduler/internal/adapters/in/http/web"
	"github.com/KolManis/uni-scheduler/internal/adapters/out/postgres"
	"github.com/KolManis/uni-scheduler/internal/core/application"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	databaseDSN := os.Getenv("DATABASE_DSN")
	if databaseDSN == "" {
		databaseDSN = "postgres://postgres:postgres@127.0.0.1:5432/scheduler?sslmode=disable"
	}
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Open(ctx, databaseDSN)
	if err != nil {
		logger.Error("open db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	inputRepo := postgres.NewInputRepository(pool)
	outputRepo := postgres.NewOutputRepository(pool)
	importRepo := postgres.NewImportRepository(pool)
	refWriteRepo := postgres.NewRefWriteRepository(pool)

	// Сценарии (команды и запросы) поверх хранилищ; адаптеры берут из них нужные.
	uc := application.NewUseCases(inputRepo, outputRepo, importRepo)

	scheduleHandler := rest.NewScheduleHandler(uc)
	excelHandler := rest.NewExcelHandler(uc.GetSchedule, inputRepo)
	importHandler := rest.NewImportHandler(uc.ImportExcel)
	referenceHandler := rest.NewReferenceHandler(inputRepo)
	refWriteHandler := rest.NewRefWriteHandler(refWriteRepo)
	webHandler := web.NewHandler(inputRepo, refWriteRepo, uc)

	router := httpTransport.NewRouter(scheduleHandler, excelHandler, importHandler, referenceHandler, refWriteHandler, webHandler)

	server := &http.Server{
		Addr:              httpAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown", "error", err)
		}
	}()

	logger.Info("starting server", "addr", httpAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
