package agent

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/salarkhannn/pfas-load-control/internal/mireye"
)

type Runtime struct {
	Jobs    *river.Client[pgx.Tx]
	Service *Service
}

func NewRuntime(pool *pgxpool.Pool, mireyeClient *mireye.Client, logger *slog.Logger) (*Runtime, error) {
	worker := &readinessWorker{pool: pool, mireye: mireyeClient, logger: logger}
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)

	jobs, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		FetchPollInterval: 500 * time.Millisecond,
		JobTimeout:        30 * time.Second,
		Queues: map[string]river.QueueConfig{
			readinessQueue: {MaxWorkers: 2},
		},
		Schema:  "river",
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("create River client: %w", err)
	}
	worker.jobs = jobs
	return &Runtime{Jobs: jobs, Service: NewService(pool, jobs)}, nil
}
