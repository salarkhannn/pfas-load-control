package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/mireye"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound = errors.New("physical evidence not found")
	ErrInvalid  = errors.New("invalid physical evidence request")
	ErrConflict = errors.New("physical evidence conflict")
)

const maxProjectedCredits = 200

type Service struct {
	pool         *pgxpool.Pool
	jobs         *river.Client[pgx.Tx]
	mireye       *mireye.Client
	supplemental *SupplementalClient
	logger       *slog.Logger
}

func NewService(pool *pgxpool.Pool, jobs *river.Client[pgx.Tx], mireyeClient *mireye.Client, supplemental *SupplementalClient, logger *slog.Logger) *Service {
	return &Service{pool: pool, jobs: jobs, mireye: mireyeClient, supplemental: supplemental, logger: logger}
}

func (s *Service) Start(ctx context.Context, workspaceKey, fieldID string) (Evaluation, bool, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return Evaluation{}, false, err
	}
	fieldUUID, err := uuid.Parse(fieldID)
	if err != nil {
		return Evaluation{}, false, ErrNotFound
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Evaluation{}, false, fmt.Errorf("begin physical evaluation: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	fieldRecord, err := queries.GetCandidateFieldForUpdate(ctx, db.GetCandidateFieldForUpdateParams{ID: fieldUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, false, ErrNotFound
	}
	if err != nil {
		return Evaluation{}, false, fmt.Errorf("load candidate field for evaluation: %w", err)
	}
	if fieldRecord.Status != "READY" || !fieldRecord.CurrentGeometryID.Valid {
		return Evaluation{}, false, fmt.Errorf("%w: confirm the boundary and required application facts first", ErrConflict)
	}
	active, err := queries.GetActivePhysicalEvaluation(ctx, db.GetActivePhysicalEvaluationParams{FieldID: fieldUUID, WorkspaceID: workspaceRecord.ID})
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return Evaluation{}, false, fmt.Errorf("finish active evaluation lookup: %w", err)
		}
		result, getErr := s.Get(ctx, workspaceKey, active.ID.String())
		return result, false, getErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, false, fmt.Errorf("load active physical evaluation: %w", err)
	}
	evaluationID := uuid.New()
	_, err = queries.CreatePhysicalEvaluation(ctx, db.CreatePhysicalEvaluationParams{
		ID: evaluationID, WorkspaceID: workspaceRecord.ID, FieldID: fieldUUID,
		GeometryID: fieldRecord.CurrentGeometryID.UUID, FieldSetVersion: FieldSetVersion,
		AggregationVersion: AggregationVersion,
	})
	if err != nil {
		return Evaluation{}, false, fmt.Errorf("create physical evaluation: %w", err)
	}
	if _, err := s.jobs.InsertTx(ctx, tx, EvaluateArgs{EvaluationID: evaluationID.String()}, nil); err != nil {
		return Evaluation{}, false, fmt.Errorf("queue physical evaluation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Evaluation{}, false, fmt.Errorf("commit physical evaluation: %w", err)
	}
	result, err := s.Get(ctx, workspaceKey, evaluationID.String())
	return result, true, err
}

func (s *Service) Latest(ctx context.Context, workspaceKey, fieldID string) (Evaluation, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return Evaluation{}, err
	}
	fieldUUID, err := uuid.Parse(fieldID)
	if err != nil {
		return Evaluation{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetLatestPhysicalEvaluation(ctx, db.GetLatestPhysicalEvaluationParams{FieldID: fieldUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, ErrNotFound
	}
	if err != nil {
		return Evaluation{}, fmt.Errorf("load latest physical evaluation: %w", err)
	}
	return s.build(ctx, record)
}

func (s *Service) Get(ctx context.Context, workspaceKey, evaluationID string) (Evaluation, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return Evaluation{}, err
	}
	id, err := uuid.Parse(evaluationID)
	if err != nil {
		return Evaluation{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetPhysicalEvaluation(ctx, db.GetPhysicalEvaluationParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, ErrNotFound
	}
	if err != nil {
		return Evaluation{}, fmt.Errorf("load physical evaluation: %w", err)
	}
	return s.build(ctx, record)
}

func (s *Service) Process(ctx context.Context, evaluationID string) error {
	id, err := uuid.Parse(evaluationID)
	if err != nil {
		return ErrNotFound
	}
	queries := db.New(s.pool)
	work, err := queries.GetPhysicalEvaluationForWork(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load physical evaluation work: %w", err)
	}
	if work.Status == string(StatusSucceeded) || work.Status == string(StatusReviewRequired) || work.Status == string(StatusFailed) {
		return nil
	}
	if work.FieldStatus != "READY" {
		return s.review(ctx, id, "FIELD_NOT_READY", "The field boundary or application facts changed before screening began.")
	}
	if err := queries.MarkPhysicalEvaluationRunning(ctx, id); err != nil {
		return fmt.Errorf("mark physical evaluation running: %w", err)
	}

	guard, err := s.guard(ctx)
	if err != nil {
		var review *reviewError
		if errors.As(err, &review) {
			return s.review(ctx, id, review.Code, review.Detail)
		}
		return err
	}
	if err := queries.DeletePhysicalEvaluationEvidence(ctx, id); err != nil {
		return fmt.Errorf("clear incomplete physical samples: %w", err)
	}
	samples, err := queries.GeneratePhysicalSamplePoints(ctx, db.GeneratePhysicalSamplePointsParams{ID: work.GeometryID, FieldID: work.FieldID, EvaluationID: id})
	if err != nil {
		return fmt.Errorf("generate physical sample points: %w", err)
	}
	if len(samples) == 0 {
		return s.review(ctx, id, "SAMPLE_GENERATION_FAILED", "No safe interior sample could be generated from the confirmed boundary.")
	}
	baseCredits := len(samples) * len(physicalFieldSpecs) * guard.FetchPerField
	if baseCredits > maxProjectedCredits {
		return s.review(ctx, id, "CREDIT_LIMIT_EXCEEDED", "This field would exceed the fixed 200-credit evaluation limit.")
	}
	projected := maxProjectedCredits
	if projected > guard.CreditsRemaining {
		return s.review(ctx, id, "INSUFFICIENT_CREDITS", "The Mireye account does not have enough credits for this field.")
	}
	request := mireye.FetchBatchRequest{Fields: physicalFieldNames(), Locations: make([]mireye.Coordinate, len(samples))}
	for index, sample := range samples {
		request.Locations[index] = mireye.Coordinate{Latitude: sample.Latitude, Longitude: sample.Longitude}
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode guarded physical request: %w", err)
	}
	requestHash := digest(requestBody)
	if err := queries.SetPhysicalEvaluationGuard(ctx, db.SetPhysicalEvaluationGuardParams{
		ID: id, CatalogVersion: &guard.CatalogVersion, CatalogEtag: optionalString(guard.CatalogETag),
		SampleCount: int32(len(samples)), ProjectedCredits: int32(projected), RequestHash: &requestHash,
	}); err != nil {
		return fmt.Errorf("record physical request guard: %w", err)
	}

	batch, err := s.mireye.FetchBatch(ctx, request)
	if err != nil {
		return s.review(ctx, id, mireyeErrorCode(err), "Mireye physical evidence could not be retrieved. No field condition was inferred.")
	}
	indices := make([]int, len(samples))
	for index := range samples {
		indices[index] = index
	}
	if err := s.persistBatch(ctx, id, batch, indices, request.Fields); err != nil {
		return err
	}
	if err := s.retryFailedFields(ctx, id, samples, guard.FetchPerField); err != nil {
		return err
	}
	if err := s.collectSupplemental(ctx, id, work); err != nil {
		return err
	}
	hasCriticalGaps, err := s.aggregate(ctx, id, len(samples))
	if err != nil {
		return err
	}
	status := string(StatusSucceeded)
	if hasCriticalGaps {
		status = string(StatusReviewRequired)
	}
	if err := queries.CompletePhysicalEvaluation(ctx, db.CompletePhysicalEvaluationParams{ID: id, Status: status}); err != nil {
		return fmt.Errorf("complete physical evaluation: %w", err)
	}
	return nil
}

func (s *Service) RecordFailure(ctx context.Context, evaluationID, code, detail string) error {
	id, err := uuid.Parse(evaluationID)
	if err != nil {
		return ErrNotFound
	}
	return db.New(s.pool).FailPhysicalEvaluation(ctx, db.FailPhysicalEvaluationParams{ID: id, Status: string(StatusFailed), FailureCode: &code, FailureDetail: &detail})
}

type guardResult struct {
	CatalogVersion   string
	CatalogETag      string
	FetchPerField    int
	CreditsRemaining int
}

type reviewError struct{ Code, Detail string }

func (e *reviewError) Error() string { return e.Code + ": " + e.Detail }

func (s *Service) guard(ctx context.Context) (guardResult, error) {
	fieldsResult, err := s.mireye.Call(ctx, mireye.ToolFields)
	if err != nil {
		return guardResult{}, &reviewError{Code: mireyeErrorCode(err), Detail: "The Mireye field catalog could not be verified."}
	}
	var catalog struct {
		Version string `json:"version"`
		Fields  []struct {
			Name      string `json:"name"`
			Type      string `json:"type"`
			Lifecycle string `json:"lifecycle"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(fieldsResult.Raw, &catalog); err != nil {
		return guardResult{}, fmt.Errorf("decode verified Mireye catalog: %w", err)
	}
	available := make(map[string]struct{ Type, Lifecycle string }, len(catalog.Fields))
	for _, field := range catalog.Fields {
		available[field.Name] = struct{ Type, Lifecycle string }{field.Type, field.Lifecycle}
	}
	for _, spec := range physicalFieldSpecs {
		field, ok := available[spec.Name]
		if !ok || field.Type != spec.Type || field.Lifecycle != "stable" {
			return guardResult{}, &reviewError{Code: "MIREYE_CATALOG_CHANGED", Detail: "A required Mireye field is missing or no longer stable: " + spec.Name + "."}
		}
	}
	plansResult, err := s.mireye.Call(ctx, mireye.ToolPlans)
	if err != nil {
		return guardResult{}, &reviewError{Code: mireyeErrorCode(err), Detail: "Mireye pricing could not be verified before spending credits."}
	}
	var plans struct {
		Credits struct {
			Costs struct {
				FetchPerField int `json:"fetch_per_field"`
			} `json:"costs"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(plansResult.Raw, &plans); err != nil || plans.Credits.Costs.FetchPerField <= 0 {
		return guardResult{}, &reviewError{Code: "MIREYE_PRICING_CHANGED", Detail: "Mireye did not return a usable per-field credit price."}
	}
	usageResult, err := s.mireye.Call(ctx, mireye.ToolUsage)
	if err != nil {
		return guardResult{}, &reviewError{Code: mireyeErrorCode(err), Detail: "Available Mireye credits could not be verified."}
	}
	var usage struct {
		Credits struct {
			Remaining json.Number `json:"remaining"`
		} `json:"credits"`
	}
	decoder := json.NewDecoder(bytes.NewReader(usageResult.Raw))
	decoder.UseNumber()
	if err := decoder.Decode(&usage); err != nil {
		return guardResult{}, fmt.Errorf("decode Mireye usage guard: %w", err)
	}
	remaining, err := strconv.Atoi(usage.Credits.Remaining.String())
	if err != nil || remaining < 0 {
		return guardResult{}, &reviewError{Code: "MIREYE_USAGE_CHANGED", Detail: "Mireye did not return a usable credit balance."}
	}
	return guardResult{CatalogVersion: catalog.Version, CatalogETag: fieldsResult.ETag, FetchPerField: plans.Credits.Costs.FetchPerField, CreditsRemaining: remaining}, nil
}

func (s *Service) review(ctx context.Context, id uuid.UUID, code, detail string) error {
	if err := db.New(s.pool).FailPhysicalEvaluation(ctx, db.FailPhysicalEvaluationParams{ID: id, Status: string(StatusReviewRequired), FailureCode: &code, FailureDetail: &detail}); err != nil {
		return fmt.Errorf("record physical review requirement: %w", err)
	}
	return nil
}

func mireyeErrorCode(err error) string {
	var callErr *mireye.CallError
	if errors.As(err, &callErr) {
		return callErr.Code
	}
	return "MIREYE_UNAVAILABLE"
}

func (s *Service) loadWorkspace(ctx context.Context, key string) (db.PfasWorkspace, error) {
	keyHash, err := workspace.Hash(key)
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("%w: workspace key is malformed", ErrInvalid)
	}
	record, err := db.New(s.pool).GetWorkspaceByHash(ctx, keyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PfasWorkspace{}, ErrNotFound
	}
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("load private workspace: %w", err)
	}
	return record, nil
}

func digest(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func pgTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
