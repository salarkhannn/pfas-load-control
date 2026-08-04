package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/mireye"
)

type readinessWorker struct {
	river.WorkerDefaults[AdvanceReadinessArgs]
	pool   *pgxpool.Pool
	jobs   *river.Client[pgx.Tx]
	mireye *mireye.Client
	logger *slog.Logger
}

type preparedStep struct {
	runID   uuid.UUID
	step    db.PfasAgentStep
	attempt int32
}

func (w *readinessWorker) Work(ctx context.Context, job *river.Job[AdvanceReadinessArgs]) error {
	prepared, proceed, err := w.prepare(ctx, job.Args)
	if err != nil || !proceed {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	result, callErr := w.mireye.Call(callCtx, mireye.Tool(prepared.step.ToolName))
	cancel()
	if callErr == nil {
		if err := w.persistSuccess(ctx, prepared, result); err != nil {
			return err
		}
		w.logger.Info("readiness step completed", "run_id", prepared.runID, "step", prepared.step.Position, "tool", prepared.step.ToolName, "request_id", result.RequestID)
		return nil
	}

	var mireyeErr *mireye.CallError
	if !errors.As(callErr, &mireyeErr) {
		return fmt.Errorf("execute readiness tool: %w", callErr)
	}
	if err := w.persistFailedCall(ctx, prepared, mireyeErr); err != nil {
		return err
	}
	if !mireyeErr.Retryable || prepared.attempt >= 3 {
		if err := w.persistGap(ctx, prepared, mireyeErr); err != nil {
			return err
		}
		w.logger.Warn("readiness run requires input", "run_id", prepared.runID, "step", prepared.step.Position, "code", mireyeErr.Code)
		return nil
	}
	if mireyeErr.RetryAfter > 0 {
		return river.JobSnooze(min(mireyeErr.RetryAfter, time.Minute))
	}
	return mireyeErr
}

func (w *readinessWorker) prepare(ctx context.Context, args AdvanceReadinessArgs) (preparedStep, bool, error) {
	runID, err := uuid.Parse(args.RunID)
	if err != nil {
		return preparedStep{}, false, fmt.Errorf("invalid readiness run ID: %w", err)
	}
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return preparedStep{}, false, fmt.Errorf("begin readiness step: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)

	run, err := queries.GetRunForUpdate(ctx, runID)
	if err != nil {
		return preparedStep{}, false, fmt.Errorf("lock readiness run: %w", err)
	}
	if run.Status == "SUCCEEDED" || run.Status == "WAITING_FOR_INPUT" || run.NextStep != args.Step {
		return preparedStep{}, false, nil
	}
	step, err := queries.GetStepForUpdate(ctx, db.GetStepForUpdateParams{RunID: runID, Position: args.Step})
	if err != nil {
		return preparedStep{}, false, fmt.Errorf("lock readiness step: %w", err)
	}
	if step.Status == "SUCCEEDED" {
		return preparedStep{}, false, nil
	}
	if err := queries.MarkRunRunning(ctx, runID); err != nil {
		return preparedStep{}, false, fmt.Errorf("mark readiness run running: %w", err)
	}
	step, err = queries.MarkStepRunning(ctx, step.ID)
	if err != nil {
		return preparedStep{}, false, fmt.Errorf("mark readiness step running: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return preparedStep{}, false, fmt.Errorf("commit readiness step start: %w", err)
	}
	return preparedStep{runID: runID, step: step, attempt: step.AttemptCount}, true, nil
}

func (w *readinessWorker) persistSuccess(ctx context.Context, prepared preparedStep, result mireye.Result) error {
	summary, err := json.Marshal(result.Summary)
	if err != nil {
		return fmt.Errorf("encode readiness summary: %w", err)
	}
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin readiness success: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)

	run, err := queries.GetRunForUpdate(ctx, prepared.runID)
	if err != nil {
		return fmt.Errorf("lock readiness run for success: %w", err)
	}
	step, err := queries.GetStepForUpdate(ctx, db.GetStepForUpdateParams{RunID: prepared.runID, Position: prepared.step.Position})
	if err != nil {
		return fmt.Errorf("lock readiness step for success: %w", err)
	}
	if step.Status == "SUCCEEDED" || run.NextStep != step.Position {
		return nil
	}
	if _, err := queries.CreateToolCall(ctx, successCallParams(prepared, result)); err != nil {
		return fmt.Errorf("record successful Mireye call: %w", err)
	}
	if err := queries.CompleteStep(ctx, db.CompleteStepParams{ID: step.ID, Summary: summary}); err != nil {
		return fmt.Errorf("complete readiness step: %w", err)
	}
	if step.Position == 3 {
		if err := queries.CompleteRun(ctx, prepared.runID); err != nil {
			return fmt.Errorf("complete readiness run: %w", err)
		}
	} else {
		nextStep := step.Position + 1
		if err := queries.AdvanceRun(ctx, db.AdvanceRunParams{ID: prepared.runID, NextStep: nextStep}); err != nil {
			return fmt.Errorf("advance readiness run: %w", err)
		}
		if _, err := w.jobs.InsertTx(ctx, tx, AdvanceReadinessArgs{RunID: prepared.runID.String(), Step: nextStep}, nil); err != nil {
			return fmt.Errorf("enqueue next readiness step: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit readiness success: %w", err)
	}
	return nil
}

func (w *readinessWorker) persistFailedCall(ctx context.Context, prepared preparedStep, callErr *mireye.CallError) error {
	params := failureCallParams(prepared, callErr)
	if _, err := db.New(w.pool).CreateToolCall(ctx, params); err != nil {
		return fmt.Errorf("record failed Mireye call: %w", err)
	}
	return nil
}

func (w *readinessWorker) persistGap(ctx context.Context, prepared preparedStep, callErr *mireye.CallError) error {
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin readiness gap: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	if _, err := queries.GetRunForUpdate(ctx, prepared.runID); err != nil {
		return fmt.Errorf("lock readiness run for gap: %w", err)
	}
	if err := queries.FailStep(ctx, prepared.step.ID); err != nil {
		return fmt.Errorf("fail readiness step: %w", err)
	}
	if _, err := queries.CreateDataGap(ctx, db.CreateDataGapParams{
		ID:         uuid.New(),
		RunID:      prepared.runID,
		StepID:     uuid.NullUUID{UUID: prepared.step.ID, Valid: true},
		Code:       callErr.Code,
		Detail:     callErr.Detail,
		Resolution: resolutionFor(callErr.Code),
	}); err != nil {
		return fmt.Errorf("record readiness data gap: %w", err)
	}
	if err := queries.WaitRunForInput(ctx, prepared.runID); err != nil {
		return fmt.Errorf("pause readiness run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit readiness gap: %w", err)
	}
	return nil
}

func successCallParams(prepared preparedStep, result mireye.Result) db.CreateToolCallParams {
	status := int32(result.HTTPStatus)
	requestID := stringPointer(result.RequestID)
	responseHash := result.ResponseHash
	return db.CreateToolCallParams{
		ID:            uuid.New(),
		RunID:         prepared.runID,
		StepID:        prepared.step.ID,
		Attempt:       prepared.attempt,
		Status:        "SUCCEEDED",
		RequestMethod: http.MethodGet,
		RequestPath:   result.Path,
		RequestHash:   result.RequestHash,
		ResponseHash:  &responseHash,
		RequestID:     requestID,
		SourceUrl:     result.SourceURL,
		HttpStatus:    &status,
		DurationMs:    max(result.Duration.Milliseconds(), 0),
		ResponseBody:  result.Raw,
		FetchedAt:     pgtype.Timestamptz{Time: result.FetchedAt, Valid: true},
	}
}

func failureCallParams(prepared preparedStep, callErr *mireye.CallError) db.CreateToolCallParams {
	var status *int32
	if callErr.Status > 0 {
		value := int32(callErr.Status)
		status = &value
	}
	return db.CreateToolCallParams{
		ID:            uuid.New(),
		RunID:         prepared.runID,
		StepID:        prepared.step.ID,
		Attempt:       prepared.attempt,
		Status:        "FAILED",
		RequestMethod: http.MethodGet,
		RequestPath:   callErr.Result.Path,
		RequestHash:   callErr.Result.RequestHash,
		RequestID:     stringPointer(callErr.Result.RequestID),
		SourceUrl:     callErr.Result.SourceURL,
		HttpStatus:    status,
		DurationMs:    max(callErr.Result.Duration.Milliseconds(), 0),
		ErrorCode:     &callErr.Code,
		FetchedAt:     pgtype.Timestamptz{Time: callErr.Result.FetchedAt, Valid: true},
	}
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func resolutionFor(code string) string {
	switch code {
	case "MIREYE_AUTHENTICATION_FAILED":
		return "Configure a valid Mireye API token, then start a new readiness run."
	case "MIREYE_ACCESS_DENIED":
		return "Confirm that the Mireye account can access the readiness endpoint."
	case "MIREYE_SCHEMA_MISMATCH":
		return "Review the current Mireye response contract before allowing decision runs."
	default:
		return "Confirm Mireye service availability and start a new readiness run."
	}
}
