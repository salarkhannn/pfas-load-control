package party

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound      = errors.New("party not found")
	ErrInvalid       = errors.New("invalid party input")
	ErrConsentExists = errors.New("active consent already exists between these parties")
)

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

type Party struct {
	ID        string   `json:"id"`
	Role      string   `json:"role"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Phone     string   `json:"phone"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

type CreatePartyInput struct {
	Role      string   `json:"role" enum:"PLANT,CONTRACTOR,FARMER"`
	Name      string   `json:"name" minLength:"1" maxLength:"200"`
	Email     string   `json:"email" required:"false" maxLength:"320" default:""`
	Phone     string   `json:"phone" maxLength:"50" default:""`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

type UpdatePartyInput struct {
	Name      string   `json:"name" minLength:"1" maxLength:"200"`
	Email     string   `json:"email" required:"false" maxLength:"320" default:""`
	Phone     string   `json:"phone" maxLength:"50"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

type Consent struct {
	ID          string  `json:"id"`
	GranterID   string  `json:"granterId"`
	GranteeID   string  `json:"granteeId"`
	Scope       string  `json:"scope"`
	Purpose     string  `json:"purpose"`
	GrantedAt   string  `json:"grantedAt"`
	ExpiresAt   *string `json:"expiresAt,omitempty"`
	RevokedAt   *string `json:"revokedAt,omitempty"`
	GranterName string  `json:"granterName,omitempty"`
	GranteeName string  `json:"granteeName,omitempty"`
}

type CreateConsentInput struct {
	GranterID string  `json:"granterId" format:"uuid"`
	GranteeID string  `json:"granteeId" format:"uuid"`
	Scope     string  `json:"scope" minLength:"1" maxLength:"200"`
	Purpose   string  `json:"purpose" minLength:"1" maxLength:"500"`
	ExpiresAt *string `json:"expiresAt,omitempty"`
}

type FieldParty struct {
	FieldID     string `json:"fieldId"`
	PartyID     string `json:"partyId"`
	Association string `json:"association"`
	Name        string `json:"name,omitempty"`
	Role        string `json:"role,omitempty"`
}

type AssignFieldPartyInput struct {
	FieldID     string `json:"fieldId" format:"uuid"`
	PartyID     string `json:"partyId" format:"uuid"`
	Association string `json:"association" enum:"OWNER,APPLICANT,CONTRACTOR"`
}

func validRole(r string) bool {
	return r == "PLANT" || r == "CONTRACTOR" || r == "FARMER"
}

func pgTS(t pgtype.Timestamptz) string {
	if t.Valid {
		return t.Time.Format(time.RFC3339)
	}
	return ""
}

func (s *Service) Create(ctx context.Context, workspaceKey string, input CreatePartyInput) (Party, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return Party{}, err
	}
	if !validRole(input.Role) {
		return Party{}, fmt.Errorf("%w: invalid role", ErrInvalid)
	}
	id := uuid.New()
	q := db.New(s.pool)
	row, err := q.CreateParty(ctx, db.CreatePartyParams{
		ID: id, WorkspaceID: ws.ID, Role: db.PfasPartyRole(input.Role),
		Name: input.Name, Email: strings.TrimSpace(input.Email), Phone: input.Phone,
		Latitude: input.Latitude, Longitude: input.Longitude,
	})
	if err != nil {
		return Party{}, fmt.Errorf("create party: %w", err)
	}
	return Party{
		ID: row.ID.String(), Role: string(row.Role), Name: row.Name, Email: row.Email,
		Phone: row.Phone, Latitude: row.Latitude, Longitude: row.Longitude,
		CreatedAt: pgTS(row.CreatedAt), UpdatedAt: pgTS(row.UpdatedAt),
	}, nil
}

func (s *Service) Get(ctx context.Context, workspaceKey, partyID string) (Party, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return Party{}, err
	}
	pID, err := uuid.Parse(partyID)
	if err != nil {
		return Party{}, ErrInvalid
	}
	row, err := db.New(s.pool).GetParty(ctx, db.GetPartyParams{ID: pID, WorkspaceID: ws.ID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Party{}, ErrNotFound
		}
		return Party{}, fmt.Errorf("get party: %w", err)
	}
	return Party{
		ID: row.ID.String(), Role: string(row.Role), Name: row.Name, Email: row.Email,
		Phone: row.Phone, Latitude: row.Latitude, Longitude: row.Longitude,
		CreatedAt: pgTS(row.CreatedAt), UpdatedAt: pgTS(row.UpdatedAt),
	}, nil
}

func (s *Service) List(ctx context.Context, workspaceKey string) ([]Party, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	rows, err := db.New(s.pool).ListPartiesByWorkspace(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("list parties: %w", err)
	}
	return s.toParties(rows), nil
}

func (s *Service) ListByRole(ctx context.Context, workspaceKey, role string) ([]Party, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	rows, err := db.New(s.pool).ListPartiesByRole(ctx, db.ListPartiesByRoleParams{WorkspaceID: ws.ID, Role: db.PfasPartyRole(role)})
	if err != nil {
		return nil, fmt.Errorf("list parties by role: %w", err)
	}
	return s.toParties(rows), nil
}

func (s *Service) Update(ctx context.Context, workspaceKey, partyID string, input UpdatePartyInput) (Party, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return Party{}, err
	}
	pID, err := uuid.Parse(partyID)
	if err != nil {
		return Party{}, ErrInvalid
	}
	row, err := db.New(s.pool).UpdateParty(ctx, db.UpdatePartyParams{
		ID: pID, WorkspaceID: ws.ID, Name: input.Name, Email: strings.TrimSpace(input.Email),
		Phone: input.Phone, Latitude: input.Latitude, Longitude: input.Longitude,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Party{}, ErrNotFound
		}
		return Party{}, fmt.Errorf("update party: %w", err)
	}
	return Party{
		ID: row.ID.String(), Role: string(row.Role), Name: row.Name, Email: row.Email,
		Phone: row.Phone, Latitude: row.Latitude, Longitude: row.Longitude,
		CreatedAt: pgTS(row.CreatedAt), UpdatedAt: pgTS(row.UpdatedAt),
	}, nil
}

func (s *Service) Delete(ctx context.Context, workspaceKey, partyID string) error {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return err
	}
	pID, err := uuid.Parse(partyID)
	if err != nil {
		return ErrInvalid
	}
	return db.New(s.pool).DeleteParty(ctx, db.DeletePartyParams{ID: pID, WorkspaceID: ws.ID})
}

// Consent

func (s *Service) CreateConsent(ctx context.Context, workspaceKey string, input CreateConsentInput) (Consent, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return Consent{}, err
	}
	gID, err := uuid.Parse(input.GranterID)
	if err != nil {
		return Consent{}, fmt.Errorf("%w: invalid granter ID", ErrInvalid)
	}
	geeID, err := uuid.Parse(input.GranteeID)
	if err != nil {
		return Consent{}, fmt.Errorf("%w: invalid grantee ID", ErrInvalid)
	}
	if gID == geeID {
		return Consent{}, fmt.Errorf("%w: granter and grantee must differ", ErrInvalid)
	}

	q := db.New(s.pool)
	exists, err := q.CheckActiveConsent(ctx, db.CheckActiveConsentParams{
		GranterPartyID: gID, GranteePartyID: geeID, Scope: input.Scope, WorkspaceID: ws.ID,
	})
	if err != nil {
		return Consent{}, fmt.Errorf("check consent: %w", err)
	}
	if exists {
		return Consent{}, ErrConsentExists
	}

	granter, err := q.GetParty(ctx, db.GetPartyParams{ID: gID, WorkspaceID: ws.ID})
	if err != nil {
		return Consent{}, fmt.Errorf("get granter: %w", err)
	}
	grantee, err := q.GetParty(ctx, db.GetPartyParams{ID: geeID, WorkspaceID: ws.ID})
	if err != nil {
		return Consent{}, fmt.Errorf("get grantee: %w", err)
	}

	id := uuid.New()
	var expiresAt pgtype.Timestamptz
	if input.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *input.ExpiresAt)
		if err != nil {
			return Consent{}, fmt.Errorf("%w: invalid expiresAt", ErrInvalid)
		}
		expiresAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	_, err = q.CreateConsent(ctx, db.CreateConsentParams{
		ID: id, WorkspaceID: ws.ID, GranterPartyID: gID, GranteePartyID: geeID,
		Scope: input.Scope, Purpose: input.Purpose, ExpiresAt: expiresAt,
	})
	if err != nil {
		return Consent{}, fmt.Errorf("create consent: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return Consent{
		ID: id.String(), GranterID: gID.String(), GranteeID: geeID.String(),
		Scope: input.Scope, Purpose: input.Purpose, GrantedAt: now, ExpiresAt: input.ExpiresAt,
		GranterName: granter.Name, GranteeName: grantee.Name,
	}, nil
}

func (s *Service) ListConsents(ctx context.Context, workspaceKey, partyID, direction string) ([]Consent, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	pID, err := uuid.Parse(partyID)
	if err != nil {
		return nil, ErrInvalid
	}
	q := db.New(s.pool)
	if direction == "granted" {
		rows, err := q.ListConsentsByGranter(ctx, db.ListConsentsByGranterParams{GranterPartyID: pID, WorkspaceID: ws.ID})
		if err != nil {
			return nil, err
		}
		return s.toConsentsFromGranter(rows), nil
	}
	if direction == "received" {
		rows, err := q.ListConsentsByGrantee(ctx, db.ListConsentsByGranteeParams{GranteePartyID: pID, WorkspaceID: ws.ID})
		if err != nil {
			return nil, err
		}
		return s.toConsentsFromGrantee(rows), nil
	}
	granted, gErr := q.ListConsentsByGranter(ctx, db.ListConsentsByGranterParams{GranterPartyID: pID, WorkspaceID: ws.ID})
	received, rErr := q.ListConsentsByGrantee(ctx, db.ListConsentsByGranteeParams{GranteePartyID: pID, WorkspaceID: ws.ID})
	if gErr != nil {
		return nil, gErr
	}
	if rErr != nil {
		return nil, rErr
	}
	all := make([]Consent, 0, len(granted)+len(received))
	all = append(all, s.toConsentsFromGranter(granted)...)
	all = append(all, s.toConsentsFromGrantee(received)...)
	return all, nil
}

func (s *Service) RevokeConsent(ctx context.Context, workspaceKey, consentID string) error {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return err
	}
	cID, err := uuid.Parse(consentID)
	if err != nil {
		return ErrInvalid
	}
	return db.New(s.pool).RevokeConsent(ctx, db.RevokeConsentParams{ID: cID, WorkspaceID: ws.ID})
}

// Field-Party

func (s *Service) AssignFieldParty(ctx context.Context, workspaceKey, fieldID, partyID, association string) error {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return err
	}
	fID, err := uuid.Parse(fieldID)
	if err != nil {
		return fmt.Errorf("%w: invalid field ID", ErrInvalid)
	}
	pID, err := uuid.Parse(partyID)
	if err != nil {
		return fmt.Errorf("%w: invalid party ID", ErrInvalid)
	}
	return db.New(s.pool).CreateFieldParty(ctx, db.CreateFieldPartyParams{
		FieldID: fID, PartyID: pID, WorkspaceID: ws.ID, Association: association,
	})
}

func (s *Service) ListPartiesByField(ctx context.Context, workspaceKey, fieldID string) ([]FieldParty, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	fID, err := uuid.Parse(fieldID)
	if err != nil {
		return nil, ErrInvalid
	}
	rows, err := db.New(s.pool).ListPartiesByField(ctx, db.ListPartiesByFieldParams{FieldID: fID, WorkspaceID: ws.ID})
	if err != nil {
		return nil, err
	}
	result := make([]FieldParty, 0, len(rows))
	for _, r := range rows {
		result = append(result, FieldParty{
			FieldID: fieldID, PartyID: r.ID.String(), Association: r.Association,
			Name: r.Name, Role: string(r.Role),
		})
	}
	return result, nil
}

func (s *Service) ListFieldsByParty(ctx context.Context, workspaceKey, partyID string) ([]FieldParty, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	pID, err := uuid.Parse(partyID)
	if err != nil {
		return nil, ErrInvalid
	}
	rows, err := db.New(s.pool).ListFieldsByParty(ctx, db.ListFieldsByPartyParams{PartyID: pID, WorkspaceID: ws.ID})
	if err != nil {
		return nil, err
	}
	result := make([]FieldParty, 0, len(rows))
	for _, r := range rows {
		result = append(result, FieldParty{FieldID: r.ID.String(), PartyID: partyID, Name: r.Name})
	}
	return result, nil
}

func (s *Service) RemoveFieldParty(ctx context.Context, workspaceKey, fieldID, partyID string) error {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return err
	}
	fID, err := uuid.Parse(fieldID)
	if err != nil {
		return ErrInvalid
	}
	pID, err := uuid.Parse(partyID)
	if err != nil {
		return ErrInvalid
	}
	return db.New(s.pool).RemoveFieldParty(ctx, db.RemoveFieldPartyParams{FieldID: fID, PartyID: pID, WorkspaceID: ws.ID})
}

// Helpers

func (s *Service) toParties(rows []db.PfasParty) []Party {
	result := make([]Party, 0, len(rows))
	for _, r := range rows {
		result = append(result, Party{
			ID: r.ID.String(), Role: string(r.Role), Name: r.Name, Email: r.Email,
			Phone: r.Phone, Latitude: r.Latitude, Longitude: r.Longitude,
			CreatedAt: pgTS(r.CreatedAt), UpdatedAt: pgTS(r.UpdatedAt),
		})
	}
	return result
}

func consentFromGranterRow(r db.ListConsentsByGranterRow) Consent {
	c := Consent{
		ID: r.ID.String(), GranterID: r.GranterPartyID.String(), GranteeID: r.GranteePartyID.String(),
		Scope: r.Scope, Purpose: r.Purpose, GrantedAt: pgTS(r.GrantedAt),
		GranterName: r.GranterName, GranteeName: r.GranteeName,
	}
	if r.ExpiresAt.Valid {
		s := r.ExpiresAt.Time.Format(time.RFC3339)
		c.ExpiresAt = &s
	}
	if r.RevokedAt.Valid {
		s := r.RevokedAt.Time.Format(time.RFC3339)
		c.RevokedAt = &s
	}
	return c
}

func consentFromGranteeRow(r db.ListConsentsByGranteeRow) Consent {
	c := Consent{
		ID: r.ID.String(), GranterID: r.GranterPartyID.String(), GranteeID: r.GranteePartyID.String(),
		Scope: r.Scope, Purpose: r.Purpose, GrantedAt: pgTS(r.GrantedAt),
		GranterName: r.GranterName, GranteeName: r.GranteeName,
	}
	if r.ExpiresAt.Valid {
		s := r.ExpiresAt.Time.Format(time.RFC3339)
		c.ExpiresAt = &s
	}
	if r.RevokedAt.Valid {
		s := r.RevokedAt.Time.Format(time.RFC3339)
		c.RevokedAt = &s
	}
	return c
}

func (s *Service) toConsentsFromGranter(rows []db.ListConsentsByGranterRow) []Consent {
	result := make([]Consent, 0, len(rows))
	for _, r := range rows {
		result = append(result, consentFromGranterRow(r))
	}
	return result
}

func (s *Service) toConsentsFromGrantee(rows []db.ListConsentsByGranteeRow) []Consent {
	result := make([]Consent, 0, len(rows))
	for _, r := range rows {
		result = append(result, consentFromGranteeRow(r))
	}
	return result
}
