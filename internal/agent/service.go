package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
)

var ErrNotFound = errors.New("readiness run not found")

type Service struct {
	pool *pgxpool.Pool
	jobs *river.Client[pgx.Tx]
}

func NewService(pool *pgxpool.Pool, jobs *river.Client[pgx.Tx]) *Service {
	return &Service{pool: pool, jobs: jobs}
}

func (s *Service) CreateReadinessRun(ctx context.Context) (Run, bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Run{}, false, fmt.Errorf("begin readiness transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	runID := uuid.New()
	queries := db.New(tx)
	if _, err := queries.CreateRun(ctx, runID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "agent_runs_one_active_readiness" {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				return Run{}, false, fmt.Errorf("rollback duplicate readiness transaction: %w", rollbackErr)
			}
			active, activeErr := db.New(s.pool).GetActiveRun(ctx)
			if activeErr != nil {
				return Run{}, false, fmt.Errorf("load active readiness run: %w", activeErr)
			}
			run, getErr := s.GetRun(ctx, active.ID)
			return run, false, getErr
		}
		return Run{}, false, fmt.Errorf("create readiness run: %w", err)
	}

	for position, toolName := range []string{"mireye.meta.fields", "mireye.meta.plans", "mireye.users.me.usage"} {
		if _, err := queries.CreateStep(ctx, db.CreateStepParams{
			ID:       uuid.New(),
			RunID:    runID,
			Position: int16(position + 1),
			ToolName: toolName,
		}); err != nil {
			return Run{}, false, fmt.Errorf("create readiness step %d: %w", position+1, err)
		}
	}

	if _, err := s.jobs.InsertTx(ctx, tx, AdvanceReadinessArgs{RunID: runID.String(), Step: 1}, nil); err != nil {
		return Run{}, false, fmt.Errorf("enqueue readiness run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, false, fmt.Errorf("commit readiness run: %w", err)
	}

	run, err := s.GetRun(ctx, runID)
	return run, true, err
}

func (s *Service) GetRun(ctx context.Context, id uuid.UUID) (Run, error) {
	queries := db.New(s.pool)
	record, err := queries.GetRun(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("load readiness run: %w", err)
	}
	return hydrateRun(ctx, queries, record)
}

func (s *Service) GetLatestRun(ctx context.Context) (Run, error) {
	queries := db.New(s.pool)
	record, err := queries.GetLatestRun(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("load latest readiness run: %w", err)
	}
	return hydrateRun(ctx, queries, record)
}

func hydrateRun(ctx context.Context, queries *db.Queries, record db.PfasAgentRun) (Run, error) {
	steps, err := queries.ListStepsForRun(ctx, record.ID)
	if err != nil {
		return Run{}, fmt.Errorf("load readiness steps: %w", err)
	}
	toolCalls, err := queries.ListToolCallsForRun(ctx, record.ID)
	if err != nil {
		return Run{}, fmt.Errorf("load readiness provenance: %w", err)
	}
	gaps, err := queries.ListDataGapsForRun(ctx, record.ID)
	if err != nil {
		return Run{}, fmt.Errorf("load readiness gaps: %w", err)
	}

	run := Run{
		ID:          record.ID.String(),
		Kind:        record.Kind,
		Status:      record.Status,
		NextStep:    int(record.NextStep),
		CreatedAt:   record.CreatedAt.Time,
		StartedAt:   timestampPointer(record.StartedAt.Time, record.StartedAt.Valid),
		CompletedAt: timestampPointer(record.CompletedAt.Time, record.CompletedAt.Valid),
		UpdatedAt:   record.UpdatedAt.Time,
		Steps:       make([]Step, 0, len(steps)),
		ToolCalls:   make([]ToolCall, 0, len(toolCalls)),
		DataGaps:    make([]DataGap, 0, len(gaps)),
	}
	for _, step := range steps {
		run.Steps = append(run.Steps, Step{
			ID:           step.ID.String(),
			Position:     int(step.Position),
			ToolName:     step.ToolName,
			Status:       step.Status,
			AttemptCount: int(step.AttemptCount),
			Summary:      step.Summary,
			StartedAt:    timestampPointer(step.StartedAt.Time, step.StartedAt.Valid),
			CompletedAt:  timestampPointer(step.CompletedAt.Time, step.CompletedAt.Valid),
		})
	}
	for _, call := range toolCalls {
		run.ToolCalls = append(run.ToolCalls, ToolCall{
			ID:           call.ID.String(),
			StepID:       call.StepID.String(),
			Attempt:      int(call.Attempt),
			Status:       call.Status,
			Method:       call.RequestMethod,
			Path:         call.RequestPath,
			RequestHash:  call.RequestHash,
			ResponseHash: call.ResponseHash,
			RequestID:    call.RequestID,
			SourceURL:    call.SourceUrl,
			HTTPStatus:   call.HttpStatus,
			DurationMS:   call.DurationMs,
			CreditCost:   call.CreditCost,
			ErrorCode:    call.ErrorCode,
			FetchedAt:    call.FetchedAt.Time,
		})
	}
	for _, gap := range gaps {
		var stepID *string
		if gap.StepID.Valid {
			value := gap.StepID.UUID.String()
			stepID = &value
		}
		run.DataGaps = append(run.DataGaps, DataGap{
			ID:         gap.ID.String(),
			StepID:     stepID,
			Code:       gap.Code,
			Detail:     gap.Detail,
			Resolution: gap.Resolution,
			Status:     gap.Status,
			CreatedAt:  gap.CreatedAt.Time,
			ResolvedAt: timestampPointer(gap.ResolvedAt.Time, gap.ResolvedAt.Valid),
		})
	}
	return run, nil
}

func timestampPointer(value time.Time, valid bool) *time.Time {
	if !valid {
		return nil
	}
	return &value
}
