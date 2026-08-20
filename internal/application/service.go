package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound = errors.New("application record not found")
	ErrInvalid  = errors.New("invalid application input")
)

type AppRecord struct {
	ID                  string  `json:"id"`
	BatchID             string  `json:"batchId,omitempty"`
	FieldID             string  `json:"fieldId"`
	FieldName           string  `json:"fieldName,omitempty"`
	ContractorID        string  `json:"contractorId"`
	ContractorName      string  `json:"contractorName,omitempty"`
	ApplicationDate     string  `json:"applicationDate"`
	DryTons             float64 `json:"dryTons"`
	RateDryTonsPerAcre  float64 `json:"rateDryTonsPerAcre"`
	AcresApplied        float64 `json:"acresApplied"`
	WeatherConditions   string  `json:"weatherConditions"`
	FieldConditionNotes string  `json:"fieldConditionNotes"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
}

type AppLoadingLedger struct {
	ID                  string  `json:"id"`
	FieldID             string  `json:"fieldId"`
	Year                int     `json:"year"`
	CumulativeDryTons   float64 `json:"cumulativeDryTons"`
	LastApplicationDate string  `json:"lastApplicationDate,omitempty"`
	LastUpdated         string  `json:"lastUpdated"`
}

type AppConfirmation struct {
	ID          string  `json:"id"`
	AppID       string  `json:"applicationId"`
	FarmerID    string  `json:"farmerId"`
	FarmerName  string  `json:"farmerName,omitempty"`
	Confirmed   bool    `json:"confirmed"`
	Notes       string  `json:"notes"`
	ConfirmedAt *string `json:"confirmedAt,omitempty"`
	CreatedAt   string  `json:"createdAt"`
}

type AppCreateRecordInput struct {
	BatchID             *string `json:"batchId,omitempty"`
	FieldID             string  `json:"fieldId" format:"uuid"`
	ContractorPartyID   string  `json:"contractorPartyId" format:"uuid"`
	ApplicationDate     string  `json:"applicationDate"`
	DryTons             float64 `json:"dryTons" minimum:"0.01"`
	RateDryTonsPerAcre  float64 `json:"rateDryTonsPerAcre" minimum:"0.01"`
	AcresApplied        float64 `json:"acresApplied" minimum:"0.01"`
	WeatherConditions   string  `json:"weatherConditions" maxLength:"500"`
	FieldConditionNotes string  `json:"fieldConditionNotes" maxLength:"500"`
}

type AppConfirmInput struct {
	FarmerPartyID string `json:"farmerPartyId" format:"uuid"`
	Notes         string `json:"notes" maxLength:"500"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) resolveWorkspace(ctx context.Context, key string) (db.PfasWorkspace, error) {
	keyHash, err := workspace.Hash(key)
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("%w: workspace key is malformed", ErrInvalid)
	}
	queries := db.New(s.pool)
	record, err := queries.GetWorkspaceByHash(ctx, keyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return queries.UpsertWorkspace(ctx, db.UpsertWorkspaceParams{ID: uuid.New(), KeyHash: keyHash})
	}
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("load workspace: %w", err)
	}
	return record, nil
}

func numericFromFloat(f float64) pgtype.Numeric {
	n := pgtype.Numeric{}
	// Use Scan to convert float64 to Numeric properly
	_ = n.Scan(fmt.Sprintf("%f", f))
	return n
}

func dateFromTime(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func (s *Service) CreateRecord(ctx context.Context, workspaceKey string, input AppCreateRecordInput) (AppRecord, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return AppRecord{}, err
	}
	fieldID, err := uuid.Parse(input.FieldID)
	if err != nil {
		return AppRecord{}, fmt.Errorf("%w: invalid fieldId", ErrInvalid)
	}
	contractorID, err := uuid.Parse(input.ContractorPartyID)
	if err != nil {
		return AppRecord{}, fmt.Errorf("%w: invalid contractorPartyId", ErrInvalid)
	}
	appDate, err := time.Parse("2006-01-02", input.ApplicationDate)
	if err != nil {
		return AppRecord{}, fmt.Errorf("%w: applicationDate must be YYYY-MM-DD", ErrInvalid)
	}

	var batchID uuid.NullUUID
	if input.BatchID != nil {
		b, err := uuid.Parse(*input.BatchID)
		if err != nil {
			return AppRecord{}, fmt.Errorf("%w: invalid batchId", ErrInvalid)
		}
		batchID = uuid.NullUUID{UUID: b, Valid: true}
	}

	id := uuid.New()
	q := db.New(s.pool)

	row, err := q.CreateApplicationRecord(ctx, db.CreateApplicationRecordParams{
		ID: id, WorkspaceID: ws.ID, BatchID: batchID, FieldID: fieldID,
		ContractorPartyID: contractorID, ApplicationDate: dateFromTime(appDate),
		DryTons: numericFromFloat(input.DryTons), RateDryTonsPerAcre: numericFromFloat(input.RateDryTonsPerAcre),
		AcresApplied: numericFromFloat(input.AcresApplied), WeatherConditions: input.WeatherConditions,
		FieldConditionNotes: input.FieldConditionNotes,
	})
	if err != nil {
		return AppRecord{}, fmt.Errorf("create application record: %w", err)
	}

	year := appDate.Year()
	_, _ = q.UpsertFieldLoadingLedger(ctx, db.UpsertFieldLoadingLedgerParams{
		ID: uuid.New(), WorkspaceID: ws.ID, FieldID: fieldID, Year: int32(year),
		CumulativeDryTons: numericFromFloat(input.DryTons), LastApplicationDate: dateFromTime(appDate),
	})

	return AppRecord{
		ID: row.ID.String(), FieldID: fieldID.String(),
		ContractorID:    contractorID.String(),
		ApplicationDate: appDate.Format("2006-01-02"),
		DryTons:         input.DryTons, RateDryTonsPerAcre: input.RateDryTonsPerAcre,
		AcresApplied: input.AcresApplied, WeatherConditions: row.WeatherConditions,
		FieldConditionNotes: row.FieldConditionNotes,
		CreatedAt:           row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:           row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

func (s *Service) GetRecord(ctx context.Context, workspaceKey, recordID string) (AppRecord, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return AppRecord{}, err
	}
	rID, err := uuid.Parse(recordID)
	if err != nil {
		return AppRecord{}, ErrInvalid
	}
	row, err := db.New(s.pool).GetApplicationRecord(ctx, db.GetApplicationRecordParams{ID: rID, WorkspaceID: ws.ID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AppRecord{}, ErrNotFound
		}
		return AppRecord{}, err
	}
	return AppRecord{
		ID: row.ID.String(), FieldID: row.FieldID.String(), FieldName: row.FieldName,
		ContractorID: row.ContractorPartyID.String(), ContractorName: row.ContractorName,
		ApplicationDate: row.ApplicationDate.Time.Format("2006-01-02"),
		DryTons:         pgtypeNumericToFloat(row.DryTons), RateDryTonsPerAcre: pgtypeNumericToFloat(row.RateDryTonsPerAcre),
		AcresApplied: pgtypeNumericToFloat(row.AcresApplied), WeatherConditions: row.WeatherConditions,
		FieldConditionNotes: row.FieldConditionNotes,
		CreatedAt:           row.CreatedAt.Time.Format(time.RFC3339), UpdatedAt: row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

func (s *Service) ListByField(ctx context.Context, workspaceKey, fieldID string) ([]AppRecord, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	fID, err := uuid.Parse(fieldID)
	if err != nil {
		return nil, ErrInvalid
	}
	rows, err := db.New(s.pool).ListApplicationRecordsByField(ctx, db.ListApplicationRecordsByFieldParams{FieldID: fID, WorkspaceID: ws.ID})
	if err != nil {
		return nil, err
	}
	return s.toRecords(rows), nil
}

func (s *Service) ListByContractor(ctx context.Context, workspaceKey, contractorID string) ([]AppRecord, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	cID, err := uuid.Parse(contractorID)
	if err != nil {
		return nil, ErrInvalid
	}
	rows, err := db.New(s.pool).ListApplicationRecordsByContractor(ctx, db.ListApplicationRecordsByContractorParams{ContractorPartyID: cID, WorkspaceID: ws.ID})
	if err != nil {
		return nil, err
	}
	result := make([]AppRecord, 0, len(rows))
	for _, r := range rows {
		result = append(result, AppRecord{
			ID: r.ID.String(), FieldID: r.FieldID.String(), FieldName: r.FieldName,
			ContractorID:    r.ContractorPartyID.String(),
			ApplicationDate: r.ApplicationDate.Time.Format("2006-01-02"),
			DryTons:         pgtypeNumericToFloat(r.DryTons), RateDryTonsPerAcre: pgtypeNumericToFloat(r.RateDryTonsPerAcre),
			AcresApplied: pgtypeNumericToFloat(r.AcresApplied), WeatherConditions: r.WeatherConditions,
			FieldConditionNotes: r.FieldConditionNotes,
			CreatedAt:           r.CreatedAt.Time.Format(time.RFC3339), UpdatedAt: r.UpdatedAt.Time.Format(time.RFC3339),
		})
	}
	return result, nil
}

func (s *Service) GetLoadingLedger(ctx context.Context, workspaceKey, fieldID string, year int) (AppLoadingLedger, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return AppLoadingLedger{}, err
	}
	fID, err := uuid.Parse(fieldID)
	if err != nil {
		return AppLoadingLedger{}, ErrInvalid
	}
	row, err := db.New(s.pool).GetFieldLoadingLedger(ctx, db.GetFieldLoadingLedgerParams{FieldID: fID, Year: int32(year), WorkspaceID: ws.ID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AppLoadingLedger{FieldID: fieldID, Year: year, CumulativeDryTons: 0}, nil
		}
		return AppLoadingLedger{}, err
	}
	l := AppLoadingLedger{
		ID: row.ID.String(), FieldID: row.FieldID.String(), Year: int(row.Year),
		CumulativeDryTons: pgtypeNumericToFloat(row.CumulativeDryTons),
		LastUpdated:       row.LastUpdated.Time.Format(time.RFC3339),
	}
	if row.LastApplicationDate.Valid {
		l.LastApplicationDate = row.LastApplicationDate.Time.Format("2006-01-02")
	}
	return l, nil
}

func (s *Service) ConfirmApplication(ctx context.Context, workspaceKey, recordID string, input AppConfirmInput) (AppConfirmation, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return AppConfirmation{}, err
	}
	rID, err := uuid.Parse(recordID)
	if err != nil {
		return AppConfirmation{}, ErrInvalid
	}
	fID, err := uuid.Parse(input.FarmerPartyID)
	if err != nil {
		return AppConfirmation{}, fmt.Errorf("%w: invalid farmerPartyId", ErrInvalid)
	}

	q := db.New(s.pool)
	existing, err := q.CreateApplicationConfirmation(ctx, db.CreateApplicationConfirmationParams{
		ID: uuid.New(), WorkspaceID: ws.ID, ApplicationID: rID,
		FarmerPartyID: fID, Confirmed: true, Notes: input.Notes,
	})
	if err != nil {
		return AppConfirmation{}, fmt.Errorf("create confirmation: %w", err)
	}

	confirmed, err := q.ConfirmApplication(ctx, db.ConfirmApplicationParams{
		ID: existing.ID, WorkspaceID: ws.ID, Notes: input.Notes,
	})
	if err != nil {
		return AppConfirmation{}, fmt.Errorf("confirm application: %w", err)
	}

	return AppConfirmation{
		ID: confirmed.ID.String(), AppID: rID.String(), FarmerID: fID.String(),
		Confirmed: confirmed.Confirmed, Notes: confirmed.Notes,
		ConfirmedAt: confirmedAtStr(confirmed.ConfirmedAt),
		CreatedAt:   confirmed.CreatedAt.Time.Format(time.RFC3339),
	}, nil
}

func (s *Service) toRecords(rows []db.ListApplicationRecordsByFieldRow) []AppRecord {
	result := make([]AppRecord, 0, len(rows))
	for _, r := range rows {
		result = append(result, AppRecord{
			ID: r.ID.String(), FieldID: r.FieldID.String(),
			ContractorID: r.ContractorPartyID.String(), ContractorName: r.ContractorName,
			ApplicationDate: r.ApplicationDate.Time.Format("2006-01-02"),
			DryTons:         pgtypeNumericToFloat(r.DryTons), RateDryTonsPerAcre: pgtypeNumericToFloat(r.RateDryTonsPerAcre),
			AcresApplied: pgtypeNumericToFloat(r.AcresApplied), WeatherConditions: r.WeatherConditions,
			FieldConditionNotes: r.FieldConditionNotes,
			CreatedAt:           r.CreatedAt.Time.Format(time.RFC3339), UpdatedAt: r.UpdatedAt.Time.Format(time.RFC3339),
		})
	}
	return result
}

func confirmedAtStr(t pgtype.Timestamptz) *string {
	if t.Valid {
		s := t.Time.Format(time.RFC3339)
		return &s
	}
	return nil
}

func pgtypeNumericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, _ := n.Float64Value()
	return f.Float64
}
