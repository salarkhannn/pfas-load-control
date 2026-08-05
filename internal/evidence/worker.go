package evidence

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
)

type Worker struct {
	river.WorkerDefaults[EvaluateArgs]
	service *Service
	logger  *slog.Logger
}

func NewWorker(logger *slog.Logger) *Worker { return &Worker{logger: logger} }

func (w *Worker) SetService(service *Service) { w.service = service }

func (w *Worker) Work(ctx context.Context, job *river.Job[EvaluateArgs]) error {
	if w.service == nil {
		return errors.New("physical evidence worker service is not configured")
	}
	err := w.service.Process(ctx, job.Args.EvaluationID)
	if err == nil {
		w.logger.Info("field physical evidence evaluated", "evaluation_id", job.Args.EvaluationID)
		return nil
	}
	if persistErr := w.service.RecordFailure(ctx, job.Args.EvaluationID, "EVALUATION_FAILED", "The field evidence could not be completed."); persistErr != nil {
		return fmt.Errorf("record field evidence failure: %w", persistErr)
	}
	w.logger.Error("field physical evidence failed", "evaluation_id", job.Args.EvaluationID, "error_type", fmt.Sprintf("%T", err))
	return nil
}
