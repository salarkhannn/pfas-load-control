package responseplan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
)

type Worker struct {
	river.WorkerDefaults[BuildArgs]
	service *Service
	logger  *slog.Logger
}

func NewWorker(logger *slog.Logger) *Worker { return &Worker{logger: logger} }

func (w *Worker) SetService(service *Service) { w.service = service }

func (w *Worker) Work(ctx context.Context, job *river.Job[BuildArgs]) error {
	if w.service == nil {
		return errors.New("response worker service is not configured")
	}
	err := w.service.Process(ctx, job.Args.RunID)
	if err == nil {
		w.logger.Info("PFAS response built", "response_run_id", job.Args.RunID)
		return nil
	}
	if persistErr := w.service.RecordFailure(ctx, job.Args.RunID, "RESPONSE_BUILD_FAILED", "The elevated or prohibited response could not be completed."); persistErr != nil {
		return fmt.Errorf("record response failure: %w", persistErr)
	}
	w.logger.Error("PFAS response failed", "response_run_id", job.Args.RunID, "error_type", fmt.Sprintf("%T", err))
	return nil
}
