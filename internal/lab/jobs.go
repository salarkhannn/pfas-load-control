package lab

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
)

const Queue = "lab-evidence"

type IngestArgs struct {
	ReportID string `json:"reportId"`
}

func (IngestArgs) Kind() string { return "ingest_lab_report" }

func (args IngestArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 2,
		Queue:       Queue,
	}
}

type Worker struct {
	river.WorkerDefaults[IngestArgs]
	service *Service
	logger  *slog.Logger
}

func NewWorker(logger *slog.Logger) *Worker {
	return &Worker{logger: logger}
}

func (w *Worker) SetService(service *Service) {
	w.service = service
}

func (w *Worker) Work(ctx context.Context, job *river.Job[IngestArgs]) error {
	if w.service == nil {
		return errors.New("lab worker service is not configured")
	}
	err := w.service.Process(ctx, job.Args.ReportID)
	if err == nil {
		w.logger.Info("laboratory report processed", "report_id", job.Args.ReportID)
		return nil
	}
	var extractionErr *ExtractionError
	if errors.As(err, &extractionErr) {
		if persistErr := w.service.Fail(ctx, job.Args.ReportID, extractionErr.Code); persistErr != nil {
			return fmt.Errorf("record laboratory extraction failure: %w", persistErr)
		}
		w.logger.Warn("laboratory report needs a different source file", "report_id", job.Args.ReportID, "code", extractionErr.Code)
		return nil
	}
	if job.Attempt >= job.MaxAttempts {
		if persistErr := w.service.Fail(ctx, job.Args.ReportID, "PROCESSING_FAILED"); persistErr != nil {
			return fmt.Errorf("record exhausted laboratory processing failure: %w", persistErr)
		}
		w.logger.Error("laboratory report processing exhausted", "report_id", job.Args.ReportID, "error_type", fmt.Sprintf("%T", err))
		return nil
	}
	return err
}
