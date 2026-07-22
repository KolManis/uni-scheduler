package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := "postgres://postgres:postgres@127.0.0.1:5432/scheduler?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		fmt.Println("connect error:", err)
		os.Exit(1)
	}
	defer pool.Close()

	_, err = pool.Exec(context.Background(),
		"TRUNCATE subject_plans, teachers, rooms, groups, departments, buildings CASCADE")
	if err != nil {
		fmt.Println("truncate error:", err)
		os.Exit(1)
	}
	fmt.Println("OK: all reference tables truncated")
}
