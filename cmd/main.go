package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	repo "github.com/KolManis/uni-scheduler/internal/repository/postgres"
	httpTransport "github.com/KolManis/uni-scheduler/internal/transport/http"

	"github.com/KolManis/uni-scheduler/internal/infrastructure/postgres"
	"github.com/KolManis/uni-scheduler/internal/transport/http/handlers"
	"github.com/KolManis/uni-scheduler/internal/usecase/generator"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	databaseDSN := os.Getenv("DATABASE_DSN")
	if databaseDSN == "" {
		databaseDSN = "postgres://postgres:postgres@localhost:5432/scheduler?sslmode=disable"
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

	inputRepo := repo.NewInputRepository(pool)
	outputRepo := repo.NewOutputRepository(pool)

	genService := generator.NewService(inputRepo, outputRepo)
	genHandler := handlers.NewScheduleHandler(genService)
	router := httpTransport.NewRouter(genHandler)

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
