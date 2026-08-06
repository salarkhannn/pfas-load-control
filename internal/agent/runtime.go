package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/salarkhannn/pfas-load-control/internal/actioncenter"
	"github.com/salarkhannn/pfas-load-control/internal/decisionpackage"
	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/field"
	"github.com/salarkhannn/pfas-load-control/internal/lab"
	"github.com/salarkhannn/pfas-load-control/internal/mireye"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
	"github.com/salarkhannn/pfas-load-control/internal/policy"
	"github.com/salarkhannn/pfas-load-control/internal/responseplan"
)

type Runtime struct {
	Jobs      *river.Client[pgx.Tx]
	Service   *Service
	Lab       *lab.Service
	Policy    *policy.Service
	Fields    *field.Service
	Evidence  *evidence.Service
	Placement *placement.Service
	Response  *responseplan.Service
	Packages  *decisionpackage.Service
	Actions   *actioncenter.Service
}

func NewRuntime(ctx context.Context, pool *pgxpool.Pool, mireyeClient *mireye.Client, logger *slog.Logger) (*Runtime, error) {
	worker := &readinessWorker{pool: pool, mireye: mireyeClient, logger: logger}
	labWorker := lab.NewWorker(logger)
	evidenceWorker := evidence.NewWorker(logger)
	responseWorker := responseplan.NewWorker(logger)
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	river.AddWorker(workers, labWorker)
	river.AddWorker(workers, evidenceWorker)
	river.AddWorker(workers, responseWorker)

	jobs, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		FetchPollInterval: 500 * time.Millisecond,
		JobTimeout:        5 * time.Minute,
		Queues: map[string]river.QueueConfig{
			readinessQueue:     {MaxWorkers: 2},
			lab.Queue:          {MaxWorkers: 1},
			evidence.Queue:     {MaxWorkers: 1},
			responseplan.Queue: {MaxWorkers: 1},
		},
		Schema:  "river",
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("create River client: %w", err)
	}
	worker.jobs = jobs
	labService := lab.NewService(pool, jobs, lab.NewParser(lab.NewSystemPDFExtractor()))
	labWorker.SetService(labService)
	catalog, err := policy.LoadCatalog()
	if err != nil {
		return nil, fmt.Errorf("load policy catalog: %w", err)
	}
	policyService := policy.NewService(pool, catalog)
	if err := policyService.EnsureRulePacks(ctx); err != nil {
		return nil, fmt.Errorf("initialize policy catalog: %w", err)
	}
	evidenceService := evidence.NewService(pool, jobs, mireyeClient, evidence.NewSupplementalClient(nil), logger)
	evidenceWorker.SetService(evidenceService)
	responseService := responseplan.NewService(pool, jobs, mireyeClient, evidence.NewSupplementalClient(nil), responseplan.NewEGLandfillClient(nil))
	responseWorker.SetService(responseService)
	placementService := placement.NewService(pool)
	packageService := decisionpackage.NewService(pool, labService, policyService, evidenceService, placementService, responseService)
	actionService := actioncenter.NewService(pool, packageService)
	return &Runtime{
		Jobs: jobs, Service: NewService(pool, jobs), Lab: labService,
		Policy: policyService, Fields: field.NewService(pool, mireyeClient), Evidence: evidenceService,
		Placement: placementService, Response: responseService, Packages: packageService, Actions: actionService,
	}, nil
}
