package lab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound = errors.New("laboratory report not found")
	ErrInvalid  = errors.New("invalid laboratory report input")
	ErrConflict = errors.New("laboratory report state conflict")
)

type Service struct {
	pool   *pgxpool.Pool
	jobs   *river.Client[pgx.Tx]
	parser *Parser
}

func NewService(pool *pgxpool.Pool, jobs *river.Client[pgx.Tx], parser *Parser) *Service {
	return &Service{pool: pool, jobs: jobs, parser: parser}
}

func (s *Service) Context(ctx context.Context, workspaceKey string) (Context, error) {
	workspace, err := s.workspace(ctx, workspaceKey)
	if errors.Is(err, ErrNotFound) {
		return Context{Facilities: []Facility{}, Batches: []Batch{}}, nil
	}
	if err != nil {
		return Context{}, err
	}
	queries := db.New(s.pool)
	facilityRows, err := queries.ListFacilitiesForWorkspace(ctx, workspace.ID)
	if err != nil {
		return Context{}, fmt.Errorf("list facilities: %w", err)
	}
	batchRows, err := queries.ListBatchesForWorkspace(ctx, workspace.ID)
	if err != nil {
		return Context{}, fmt.Errorf("list biosolids batches: %w", err)
	}
	result := Context{
		Facilities: make([]Facility, 0, len(facilityRows)),
		Batches:    make([]Batch, 0, len(batchRows)),
	}
	for _, row := range facilityRows {
		result.Facilities = append(result.Facilities, Facility{ID: row.ID.String(), Name: row.Name, Jurisdiction: row.Jurisdiction})
	}
	for _, row := range batchRows {
		result.Batches = append(result.Batches, Batch{
			ID:            row.ID.String(),
			Identifier:    row.Identifier,
			WetMassKG:     optionalString(row.WetMassKg),
			PercentSolids: optionalString(row.PercentSolids),
			FacilityID:    row.FacilityID.String(),
			FacilityName:  row.FacilityName,
			Jurisdiction:  row.Jurisdiction,
		})
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, workspaceKey string, intake Intake) (Report, bool, error) {
	if err := validateIntake(&intake); err != nil {
		return Report{}, false, err
	}
	keyHash, err := hashWorkspaceKey(workspaceKey)
	if err != nil {
		return Report{}, false, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Report{}, false, fmt.Errorf("begin laboratory intake: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	workspace, err := queries.UpsertWorkspace(ctx, db.UpsertWorkspaceParams{ID: uuid.New(), KeyHash: keyHash})
	if err != nil {
		return Report{}, false, fmt.Errorf("open private workspace: %w", err)
	}
	facility, err := queries.UpsertFacility(ctx, db.UpsertFacilityParams{
		ID:             uuid.New(),
		WorkspaceID:    workspace.ID,
		Name:           intake.FacilityName,
		NormalizedName: normalizeIdentity(intake.FacilityName),
	})
	if err != nil {
		return Report{}, false, fmt.Errorf("save facility: %w", err)
	}
	wetMass, err := numeric(intake.WetMassKG)
	if err != nil {
		return Report{}, false, fmt.Errorf("%w: wet mass must be a positive decimal", ErrInvalid)
	}
	percentSolids, err := numeric(intake.PercentSolids)
	if err != nil {
		return Report{}, false, fmt.Errorf("%w: percent solids must be a decimal greater than 0 and at most 100", ErrInvalid)
	}
	batch, err := queries.UpsertBatch(ctx, db.UpsertBatchParams{
		ID:                   uuid.New(),
		WorkspaceID:          workspace.ID,
		FacilityID:           facility.ID,
		Identifier:           intake.BatchID,
		NormalizedIdentifier: normalizeIdentity(intake.BatchID),
		WetMassKg:            wetMass,
		PercentSolids:        percentSolids,
	})
	if err != nil {
		return Report{}, false, fmt.Errorf("save biosolids batch: %w", err)
	}

	digest := sha256.Sum256(intake.Content)
	reportHash := hex.EncodeToString(digest[:])
	reportID := uuid.New()
	created := true
	retried := false
	_, err = queries.CreateLabReport(ctx, db.CreateLabReportParams{
		ID:               reportID,
		WorkspaceID:      workspace.ID,
		FacilityID:       facility.ID,
		BatchID:          batch.ID,
		OriginalFilename: intake.Filename,
		MediaType:        intake.MediaType,
		SizeBytes:        int32(len(intake.Content)),
		Sha256:           reportHash,
		Content:          intake.Content,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		created = false
		existing, loadErr := queries.GetLabReportByHash(ctx, db.GetLabReportByHashParams{WorkspaceID: workspace.ID, Sha256: reportHash})
		if loadErr != nil {
			return Report{}, false, fmt.Errorf("load duplicate laboratory report: %w", loadErr)
		}
		reportID = existing.ID
		if existing.Status == string(StatusFailed) {
			rows, retryErr := queries.RetryFailedLabReport(ctx, db.RetryFailedLabReportParams{
				ID:          existing.ID,
				WorkspaceID: workspace.ID,
			})
			if retryErr != nil {
				return Report{}, false, fmt.Errorf("retry failed laboratory report: %w", retryErr)
			}
			retried = rows == 1
		}
	} else if err != nil {
		return Report{}, false, fmt.Errorf("save private laboratory report: %w", err)
	}
	if created || retried {
		if _, err := s.jobs.InsertTx(ctx, tx, IngestArgs{ReportID: reportID.String()}, nil); err != nil {
			return Report{}, false, fmt.Errorf("queue laboratory extraction: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, false, fmt.Errorf("commit laboratory intake: %w", err)
	}
	report, err := s.Get(ctx, workspaceKey, reportID.String())
	return report, created, err
}

func (s *Service) Get(ctx context.Context, workspaceKey, reportID string) (Report, error) {
	workspace, err := s.workspace(ctx, workspaceKey)
	if err != nil {
		return Report{}, err
	}
	id, err := uuid.Parse(reportID)
	if err != nil {
		return Report{}, ErrNotFound
	}
	queries := db.New(s.pool)
	row, err := queries.GetLabReportForWorkspace(ctx, db.GetLabReportForWorkspaceParams{ID: id, WorkspaceID: workspace.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, fmt.Errorf("load laboratory report: %w", err)
	}
	return hydrateReport(ctx, queries, row)
}

func (s *Service) Content(ctx context.Context, workspaceKey, reportID string) (string, string, []byte, error) {
	workspace, err := s.workspace(ctx, workspaceKey)
	if err != nil {
		return "", "", nil, err
	}
	id, err := uuid.Parse(reportID)
	if err != nil {
		return "", "", nil, ErrNotFound
	}
	row, err := db.New(s.pool).GetLabReportContent(ctx, db.GetLabReportContentParams{ID: id, WorkspaceID: workspace.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, ErrNotFound
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("load private report content: %w", err)
	}
	return row.OriginalFilename, row.MediaType, row.Content, nil
}

func (s *Service) Correct(ctx context.Context, workspaceKey, reportID string, correction Correction) (Report, error) {
	workspace, err := s.workspace(ctx, workspaceKey)
	if err != nil {
		return Report{}, err
	}
	id, err := uuid.Parse(reportID)
	if err != nil {
		return Report{}, ErrNotFound
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Report{}, fmt.Errorf("begin report correction: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	record, err := queries.GetLabReportForProcessing(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || record.WorkspaceID != workspace.ID {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, fmt.Errorf("lock report for correction: %w", err)
	}
	if record.Status == string(StatusUploaded) || record.Status == string(StatusProcessing) {
		return Report{}, fmt.Errorf("%w: wait for extraction to finish", ErrConflict)
	}
	if record.Status == string(StatusConfirmed) {
		return Report{}, fmt.Errorf("%w: confirmed evidence is immutable", ErrConflict)
	}
	pages, err := reportPages(ctx, queries, id)
	if err != nil {
		return Report{}, err
	}
	analytes := make([]Analyte, 0, len(correction.Analytes))
	for _, submitted := range correction.Analytes {
		analyte := analyteFromResult(
			submitted.CanonicalAnalyte,
			submitted.ResultText,
			submitted.Unit,
			submitted.Basis,
			submitted.Qualifier,
			submitted.ReportingLimit,
			submitted.DetectionLimit,
		)
		analyte.ReportedAnalyte = cleanString(submitted.ReportedAnalyte)
		if analyte.ReportedAnalyte == "" {
			analyte.ReportedAnalyte = submitted.CanonicalAnalyte
		}
		analyte.SourcePage = submitted.SourcePage
		analyte.SourceExcerpt = cleanString(submitted.SourceExcerpt)
		analyte.SourceBounds = submitted.SourceBounds
		analytes = append(analytes, analyte)
	}
	draft := Draft{
		Version:          int(record.CurrentVersion) + 1,
		Status:           "DRAFT",
		Source:           "OPERATOR_CORRECTION",
		Laboratory:       correction.Laboratory,
		SampleIdentifier: correction.SampleIdentifier,
		CollectionDate:   correction.CollectionDate,
		Matrix:           correction.Matrix,
		Method:           correction.Method,
		Basis:            correction.Basis,
		Analytes:         analytes,
	}
	draft, gaps := validateDraft(draft, pages, optionalString(record.PercentSolids))
	if record.CurrentVersion > 0 {
		if err := queries.SupersedeCurrentLabReportVersion(ctx, db.SupersedeCurrentLabReportVersionParams{ReportID: id, Version: record.CurrentVersion}); err != nil {
			return Report{}, fmt.Errorf("supersede previous report evidence: %w", err)
		}
	}
	if err := persistDraft(ctx, queries, id, draft, gaps); err != nil {
		return Report{}, err
	}
	status := StatusReadyToConfirm
	if len(gaps) > 0 {
		status = StatusNeedsReview
	}
	if err := queries.CompleteLabReportExtraction(ctx, db.CompleteLabReportExtractionParams{ID: id, Status: string(status), CurrentVersion: int32(draft.Version)}); err != nil {
		return Report{}, fmt.Errorf("complete report correction: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("commit report correction: %w", err)
	}
	return s.Get(ctx, workspaceKey, reportID)
}

func (s *Service) Confirm(ctx context.Context, workspaceKey, reportID string) (Report, error) {
	workspace, err := s.workspace(ctx, workspaceKey)
	if err != nil {
		return Report{}, err
	}
	id, err := uuid.Parse(reportID)
	if err != nil {
		return Report{}, ErrNotFound
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Report{}, fmt.Errorf("begin report confirmation: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	record, err := queries.GetLabReportForProcessing(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || record.WorkspaceID != workspace.ID {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, fmt.Errorf("lock report for confirmation: %w", err)
	}
	if record.Status == string(StatusConfirmed) {
		if err := tx.Rollback(ctx); err != nil {
			return Report{}, fmt.Errorf("close idempotent confirmation: %w", err)
		}
		return s.Get(ctx, workspaceKey, reportID)
	}
	if record.Status != string(StatusReadyToConfirm) || record.CurrentVersion < 1 {
		return Report{}, fmt.Errorf("%w: resolve every missing or ambiguous value before confirmation", ErrConflict)
	}
	version, err := queries.GetCurrentLabReportVersion(ctx, db.GetCurrentLabReportVersionParams{ReportID: id, Version: record.CurrentVersion})
	if err != nil {
		return Report{}, fmt.Errorf("load report evidence for confirmation: %w", err)
	}
	gaps, err := queries.ListGapsForVersion(ctx, version.ID)
	if err != nil {
		return Report{}, fmt.Errorf("verify report gaps before confirmation: %w", err)
	}
	if len(gaps) > 0 {
		return Report{}, fmt.Errorf("%w: resolve every missing or ambiguous value before confirmation", ErrConflict)
	}
	analytes, err := queries.ListAnalytesForVersion(ctx, version.ID)
	if err != nil {
		return Report{}, fmt.Errorf("load analytes for confirmation: %w", err)
	}
	evidence, err := json.Marshal(struct {
		Version  db.GetCurrentLabReportVersionRow `json:"version"`
		Analytes []db.ListAnalytesForVersionRow   `json:"analytes"`
	}{Version: version, Analytes: analytes})
	if err != nil {
		return Report{}, fmt.Errorf("encode confirmation evidence: %w", err)
	}
	digest := sha256.Sum256(evidence)
	updated, err := queries.ConfirmLabReportVersion(ctx, version.ID)
	if err != nil {
		return Report{}, fmt.Errorf("confirm report evidence: %w", err)
	}
	if updated != 1 {
		return Report{}, fmt.Errorf("%w: report evidence changed before confirmation", ErrConflict)
	}
	if err := queries.ConfirmLabReport(ctx, id); err != nil {
		return Report{}, fmt.Errorf("confirm laboratory report: %w", err)
	}
	if _, err := queries.CreateLabConfirmation(ctx, db.CreateLabConfirmationParams{
		ID:           uuid.New(),
		WorkspaceID:  workspace.ID,
		ReportID:     id,
		VersionID:    version.ID,
		EvidenceHash: hex.EncodeToString(digest[:]),
	}); err != nil {
		return Report{}, fmt.Errorf("record report confirmation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("commit report confirmation: %w", err)
	}
	return s.Get(ctx, workspaceKey, reportID)
}

func (s *Service) Process(ctx context.Context, reportID string) error {
	id, err := uuid.Parse(reportID)
	if err != nil {
		return fmt.Errorf("invalid report ID: %w", err)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin laboratory processing: %w", err)
	}
	queries := db.New(tx)
	record, err := queries.GetLabReportForProcessing(ctx, id)
	if err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("load report for processing: %w", err)
	}
	if record.Status != string(StatusUploaded) && record.Status != string(StatusProcessing) {
		tx.Rollback(ctx)
		return nil
	}
	if _, err := queries.MarkLabReportProcessing(ctx, id); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("mark report processing: %w", err)
	}
	content := append([]byte(nil), record.Content...)
	mediaType := record.MediaType
	percentSolids := optionalString(record.PercentSolids)
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit laboratory processing start: %w", err)
	}

	extraction, err := s.parser.Parse(ctx, mediaType, content, percentSolids)
	if err != nil {
		return err
	}
	tx, err = s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin laboratory extraction persistence: %w", err)
	}
	defer tx.Rollback(ctx)
	queries = db.New(tx)
	current, err := queries.GetLabReportForProcessing(ctx, id)
	if err != nil {
		return fmt.Errorf("lock report after extraction: %w", err)
	}
	if current.Status != string(StatusProcessing) {
		return nil
	}
	if err := queries.DeleteLabReportPages(ctx, id); err != nil {
		return fmt.Errorf("replace report source pages: %w", err)
	}
	for _, page := range extraction.Pages {
		width, err := numeric(page.Width)
		if err != nil {
			return fmt.Errorf("store source page width: %w", err)
		}
		height, err := numeric(page.Height)
		if err != nil {
			return fmt.Errorf("store source page height: %w", err)
		}
		if err := queries.UpsertLabReportPage(ctx, db.UpsertLabReportPageParams{
			ReportID:         id,
			PageNumber:       int32(page.Number),
			ExtractedText:    page.Text,
			ExtractionMethod: page.ExtractionMethod,
			ReadError:        optionalString(page.ReadError),
			Width:            width,
			Height:           height,
		}); err != nil {
			return fmt.Errorf("store report source page: %w", err)
		}
	}
	if err := persistDraft(ctx, queries, id, extraction.Draft, extraction.Gaps); err != nil {
		return err
	}
	status := StatusReadyToConfirm
	if len(extraction.Gaps) > 0 {
		status = StatusNeedsReview
	}
	if err := queries.CompleteLabReportExtraction(ctx, db.CompleteLabReportExtractionParams{ID: id, Status: string(status), CurrentVersion: 1}); err != nil {
		return fmt.Errorf("complete laboratory extraction: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit laboratory extraction: %w", err)
	}
	return nil
}

func (s *Service) Fail(ctx context.Context, reportID, code string) error {
	id, err := uuid.Parse(reportID)
	if err != nil {
		return fmt.Errorf("invalid report ID: %w", err)
	}
	return db.New(s.pool).FailLabReport(ctx, db.FailLabReportParams{ID: id, FailureCode: &code})
}

func persistDraft(ctx context.Context, queries *db.Queries, reportID uuid.UUID, draft Draft, gaps []Gap) error {
	collectionDate, err := date(draft.CollectionDate)
	if err != nil {
		collectionDate = pgtype.Date{}
	}
	versionID := uuid.New()
	if _, err := queries.CreateLabReportVersion(ctx, db.CreateLabReportVersionParams{
		ID:               versionID,
		ReportID:         reportID,
		Version:          int32(draft.Version),
		Source:           draft.Source,
		Laboratory:       draft.Laboratory,
		SampleIdentifier: draft.SampleIdentifier,
		CollectionDate:   collectionDate,
		Matrix:           draft.Matrix,
		Method:           draft.Method,
		Basis:            draft.Basis,
	}); err != nil {
		return fmt.Errorf("store report evidence version: %w", err)
	}
	for _, analyte := range draft.Analytes {
		bounds, err := json.Marshal(analyte.SourceBounds)
		if err != nil {
			return fmt.Errorf("encode analyte source bounds: %w", err)
		}
		value, err := numeric(analyte.Value)
		if err != nil {
			return fmt.Errorf("store %s result: %w", analyte.CanonicalAnalyte, err)
		}
		reportingLimit, err := numeric(analyte.ReportingLimit)
		if err != nil {
			return fmt.Errorf("store %s reporting limit: %w", analyte.CanonicalAnalyte, err)
		}
		detectionLimit, err := numeric(analyte.DetectionLimit)
		if err != nil {
			return fmt.Errorf("store %s detection limit: %w", analyte.CanonicalAnalyte, err)
		}
		normalizedValue, _ := numeric(analyte.NormalizedValueUGKGDry)
		normalizedReporting, _ := numeric(analyte.NormalizedReportingLimitUGKGDry)
		normalizedDetection, _ := numeric(analyte.NormalizedDetectionLimitUGKGDry)
		if _, err := queries.CreateAnalyteResult(ctx, db.CreateAnalyteResultParams{
			ID:                              uuid.New(),
			ReportID:                        reportID,
			VersionID:                       versionID,
			CanonicalAnalyte:                analyte.CanonicalAnalyte,
			ReportedAnalyte:                 analyte.ReportedAnalyte,
			ResultText:                      analyte.ResultText,
			Value:                           value,
			Unit:                            analyte.Unit,
			Basis:                           analyte.Basis,
			Qualifier:                       analyte.Qualifier,
			IsNonDetect:                     analyte.IsNonDetect,
			ReportingLimit:                  reportingLimit,
			DetectionLimit:                  detectionLimit,
			NormalizedValueUgKgDry:          normalizedValue,
			NormalizedReportingLimitUgKgDry: normalizedReporting,
			NormalizedDetectionLimitUgKgDry: normalizedDetection,
			SourcePage:                      int32(analyte.SourcePage),
			SourceExcerpt:                   analyte.SourceExcerpt,
			SourceBounds:                    bounds,
		}); err != nil {
			return fmt.Errorf("store %s evidence: %w", analyte.CanonicalAnalyte, err)
		}
	}
	for _, gap := range gaps {
		if _, err := queries.CreateLabReportGap(ctx, db.CreateLabReportGapParams{
			ID:         uuid.New(),
			ReportID:   reportID,
			VersionID:  versionID,
			Code:       gap.Code,
			FieldName:  gap.FieldName,
			Detail:     gap.Detail,
			Resolution: gap.Resolution,
		}); err != nil {
			return fmt.Errorf("store laboratory evidence gap: %w", err)
		}
	}
	return nil
}

func hydrateReport(ctx context.Context, queries *db.Queries, row db.GetLabReportForWorkspaceRow) (Report, error) {
	report := Report{
		ID:               row.ID.String(),
		Status:           ReportStatus(row.Status),
		OriginalFilename: row.OriginalFilename,
		MediaType:        row.MediaType,
		SizeBytes:        int(row.SizeBytes),
		SHA256:           row.Sha256,
		Facility: Facility{
			ID:           row.FacilityID.String(),
			Name:         row.FacilityName,
			Jurisdiction: row.Jurisdiction,
		},
		Batch: Batch{
			ID:            row.BatchID.String(),
			Identifier:    row.BatchIdentifier,
			WetMassKG:     optionalString(row.WetMassKg),
			PercentSolids: optionalString(row.PercentSolids),
			FacilityID:    row.FacilityID.String(),
			FacilityName:  row.FacilityName,
			Jurisdiction:  row.Jurisdiction,
		},
		Pages:       []Page{},
		Gaps:        []Gap{},
		FailureCode: row.FailureCode,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
		ConfirmedAt: timestamp(row.ConfirmedAt),
	}
	pages, err := reportPages(ctx, queries, row.ID)
	if err != nil {
		return Report{}, err
	}
	report.Pages = pages
	if row.CurrentVersion == 0 {
		return report, nil
	}
	version, err := queries.GetCurrentLabReportVersion(ctx, db.GetCurrentLabReportVersionParams{ReportID: row.ID, Version: row.CurrentVersion})
	if err != nil {
		return Report{}, fmt.Errorf("load current report evidence: %w", err)
	}
	analyteRows, err := queries.ListAnalytesForVersion(ctx, version.ID)
	if err != nil {
		return Report{}, fmt.Errorf("load report analytes: %w", err)
	}
	draft := Draft{
		ID:               version.ID.String(),
		Version:          int(version.Version),
		Status:           version.Status,
		Source:           version.Source,
		Laboratory:       version.Laboratory,
		SampleIdentifier: version.SampleIdentifier,
		CollectionDate:   optionalString(version.CollectionDate),
		Matrix:           version.Matrix,
		Method:           version.Method,
		Basis:            version.Basis,
		Analytes:         make([]Analyte, 0, len(analyteRows)),
		CreatedAt:        version.CreatedAt.Time,
		ConfirmedAt:      timestamp(version.ConfirmedAt),
	}
	for _, analyte := range analyteRows {
		var bounds *SourceBounds
		if len(analyte.SourceBounds) > 0 && string(analyte.SourceBounds) != "null" {
			bounds = new(SourceBounds)
			if err := json.Unmarshal(analyte.SourceBounds, bounds); err != nil {
				return Report{}, fmt.Errorf("decode analyte source bounds: %w", err)
			}
		}
		draft.Analytes = append(draft.Analytes, Analyte{
			ID:                              analyte.ID.String(),
			CanonicalAnalyte:                analyte.CanonicalAnalyte,
			ReportedAnalyte:                 analyte.ReportedAnalyte,
			ResultText:                      analyte.ResultText,
			Value:                           optionalString(analyte.Value),
			Unit:                            analyte.Unit,
			Basis:                           analyte.Basis,
			Qualifier:                       analyte.Qualifier,
			IsNonDetect:                     analyte.IsNonDetect,
			ReportingLimit:                  optionalString(analyte.ReportingLimit),
			DetectionLimit:                  optionalString(analyte.DetectionLimit),
			NormalizedValueUGKGDry:          optionalString(analyte.NormalizedValueUgKgDry),
			NormalizedReportingLimitUGKGDry: optionalString(analyte.NormalizedReportingLimitUgKgDry),
			NormalizedDetectionLimitUGKGDry: optionalString(analyte.NormalizedDetectionLimitUgKgDry),
			SourcePage:                      int(analyte.SourcePage),
			SourceExcerpt:                   analyte.SourceExcerpt,
			SourceBounds:                    bounds,
		})
	}
	report.Draft = &draft
	gapRows, err := queries.ListGapsForVersion(ctx, version.ID)
	if err != nil {
		return Report{}, fmt.Errorf("load report gaps: %w", err)
	}
	for _, gap := range gapRows {
		report.Gaps = append(report.Gaps, Gap{
			ID:         gap.ID.String(),
			Code:       gap.Code,
			FieldName:  gap.FieldName,
			Detail:     gap.Detail,
			Resolution: gap.Resolution,
			Status:     gap.Status,
			CreatedAt:  gap.CreatedAt.Time,
			ResolvedAt: timestamp(gap.ResolvedAt),
		})
	}
	return report, nil
}

func reportPages(ctx context.Context, queries *db.Queries, reportID uuid.UUID) ([]Page, error) {
	rows, err := queries.ListLabReportPages(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("load report source pages: %w", err)
	}
	pages := make([]Page, 0, len(rows))
	for _, row := range rows {
		pages = append(pages, Page{
			Number:           int(row.PageNumber),
			Text:             row.ExtractedText,
			ExtractionMethod: row.ExtractionMethod,
			ReadError:        valueOrEmpty(row.ReadError),
			Width:            optionalString(row.Width),
			Height:           optionalString(row.Height),
		})
	}
	return pages, nil
}

func (s *Service) workspace(ctx context.Context, key string) (db.PfasWorkspace, error) {
	hash, err := hashWorkspaceKey(key)
	if err != nil {
		return db.PfasWorkspace{}, err
	}
	workspace, err := db.New(s.pool).GetWorkspaceByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PfasWorkspace{}, ErrNotFound
	}
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("load private workspace: %w", err)
	}
	return workspace, nil
}

func hashWorkspaceKey(value string) (string, error) {
	hash, err := workspace.Hash(value)
	if err != nil {
		return "", fmt.Errorf("%w: workspace key is malformed", ErrInvalid)
	}
	return hash, nil
}

func validateIntake(intake *Intake) error {
	intake.FacilityName = cleanString(intake.FacilityName)
	intake.BatchID = cleanString(intake.BatchID)
	intake.Filename = filepath.Base(strings.TrimSpace(intake.Filename))
	if len(intake.FacilityName) < 1 || len(intake.FacilityName) > 160 {
		return fmt.Errorf("%w: facility name is required and must be at most 160 characters", ErrInvalid)
	}
	if len(intake.BatchID) < 1 || len(intake.BatchID) > 120 {
		return fmt.Errorf("%w: batch identifier is required and must be at most 120 characters", ErrInvalid)
	}
	if intake.Filename == "." || len(intake.Filename) < 1 || len(intake.Filename) > 255 || strings.IndexFunc(intake.Filename, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: report filename is invalid", ErrInvalid)
	}
	if len(intake.Content) < 1 || len(intake.Content) > MaxReportBytes {
		return fmt.Errorf("%w: report must be between 1 byte and 10 MiB", ErrInvalid)
	}
	extension := strings.ToLower(filepath.Ext(intake.Filename))
	switch {
	case strings.HasPrefix(string(intake.Content), "%PDF-") && extension == ".pdf":
		intake.MediaType = "application/pdf"
	case extension == ".csv":
		intake.MediaType = "text/csv"
	case extension == ".json" && json.Valid(intake.Content):
		intake.MediaType = "application/json"
	default:
		return fmt.Errorf("%w: report must be a PDF, CSV, or valid JSON file", ErrInvalid)
	}
	if intake.WetMassKG != nil {
		value, err := decimalRat(*intake.WetMassKG)
		if err != nil || value.Sign() <= 0 {
			return fmt.Errorf("%w: wet mass must be a positive decimal", ErrInvalid)
		}
	}
	if intake.PercentSolids != nil {
		value, err := decimalRat(*intake.PercentSolids)
		if err != nil || value.Sign() <= 0 || value.Cmp(pgHundred()) > 0 {
			return fmt.Errorf("%w: percent solids must be greater than 0 and at most 100", ErrInvalid)
		}
	}
	return nil
}

func normalizeIdentity(value string) string {
	return strings.ToLower(cleanString(value))
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func numeric(value *string) (pgtype.Numeric, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Numeric{}, nil
	}
	canonical, err := canonicalDecimal(*value)
	if err != nil {
		return pgtype.Numeric{}, err
	}
	var output pgtype.Numeric
	if err := output.Scan(canonical); err != nil {
		return pgtype.Numeric{}, err
	}
	return output, nil
}

func date(value *string) (pgtype.Date, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Date{}, nil
	}
	parsed, err := time.Parse("2006-01-02", *value)
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: parsed, Valid: true}, nil
}

func timestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func pgHundred() *big.Rat {
	return big.NewRat(100, 1)
}
