package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	migrations "github.com/salarkhannn/pfas-load-control/db/migrations"
)

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errorsWithoutConnectionDetails("parse DATABASE_URL", err)
	}
	poolConfig.MaxConns = 8
	poolConfig.MinConns = 0
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, errorsWithoutConnectionDetails("create database pool", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errorsWithoutConnectionDetails("ping database", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.Files, goose.WithSlog(logger))
	if err != nil {
		return fmt.Errorf("create application migrator: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply application migrations: %w", err)
	}

	riverMigrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Schema: "river", Logger: logger})
	if err != nil {
		return fmt.Errorf("create River migrator: %w", err)
	}
	if _, err := riverMigrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("apply River migrations: %w", err)
	}
	return nil
}

func errorsWithoutConnectionDetails(operation string, err error) error {
	return fmt.Errorf("%s: database connection failed (%T)", operation, err)
}
