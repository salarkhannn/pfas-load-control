package coordination

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound          = errors.New("coordination workflow not found")
	ErrInvalid           = errors.New("invalid coordination input")
	ErrInvalidTransition = errors.New("invalid status transition")
)

type CoordWorkflow struct {
	ID            string `json:"id"`
	BatchID       string `json:"batchId,omitempty"`
	FieldID       string `json:"fieldId,omitempty"`
	Status        string `json:"status"`
	CreatedBy     string `json:"createdBy"`
	CreatedByName string `json:"createdByName"`
	FieldName     string `json:"fieldName,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type CoordStep struct {
	ID          string  `json:"id"`
	WorkflowID  string  `json:"workflowId"`
	PartyID     *string `json:"partyId,omitempty"`
	PartyName   string  `json:"partyName,omitempty"`
	PartyEmail  string  `json:"partyEmail,omitempty"`
	StepRole    string  `json:"stepRole"`
	StepType    string  `json:"stepType"`
	Status      string  `json:"status"`
	Notes       string  `json:"notes"`
	ConfirmedAt *string `json:"confirmedAt,omitempty"`
	CreatedAt   string  `json:"createdAt"`
}

type CoordDocument struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflowId"`
	PartyID    string `json:"partyId"`
	PartyName  string `json:"partyName"`
	DocType    string `json:"docType"`
	Filename   string `json:"filename"`
	FileHash   string `json:"fileHash"`
	MimeType   string `json:"mimeType"`
	SizeBytes  int64  `json:"sizeBytes"`
	CreatedAt  string `json:"createdAt"`
}

type CoordNotification struct {
	ID            string  `json:"id"`
	WorkflowID    string  `json:"workflowId"`
	RecipientID   string  `json:"recipientId"`
	RecipientName string  `json:"recipientName"`
	EventType     string  `json:"eventType"`
	Message       string  `json:"message"`
	ReadAt        *string `json:"readAt,omitempty"`
	CreatedAt     string  `json:"createdAt"`
}

type CoordCreateWorkflowInput struct {
	BatchID          *string `json:"batchId,omitempty"`
	FieldID          *string `json:"fieldId,omitempty"`
	CreatedByPartyID string  `json:"createdByPartyId" format:"uuid"`
}

type CoordConfirmStepInput struct {
	PartyID string `json:"partyId" format:"uuid"`
	Notes   string `json:"notes" maxLength:"500"`
}

type CoordAssignStepInput struct {
	PartyID string `json:"partyId" format:"uuid"`
}

type CoordCreateDocumentInput struct {
	PartyID   string `json:"partyId" format:"uuid"`
	DocType   string `json:"docType" minLength:"1" maxLength:"100"`
	Filename  string `json:"filename" minLength:"1" maxLength:"500"`
	FileHash  string `json:"fileHash" minLength:"64" maxLength:"64"`
	MimeType  string `json:"mimeType" default:"application/octet-stream"`
	SizeBytes int64  `json:"sizeBytes" minimum:"0"`
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

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) CreateWorkflow(ctx context.Context, workspaceKey string, input CoordCreateWorkflowInput) (CoordWorkflow, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return CoordWorkflow{}, err
	}
	createdBy, err := uuid.Parse(input.CreatedByPartyID)
	if err != nil {
		return CoordWorkflow{}, fmt.Errorf("%w: invalid createdByPartyId", ErrInvalid)
	}

	var fieldID uuid.NullUUID
	if input.FieldID != nil {
		f, err := uuid.Parse(*input.FieldID)
		if err != nil {
			return CoordWorkflow{}, fmt.Errorf("%w: invalid fieldId", ErrInvalid)
		}
		fieldID = uuid.NullUUID{UUID: f, Valid: true}
	}

	var batchID uuid.NullUUID
	if input.BatchID != nil {
		b, err := uuid.Parse(*input.BatchID)
		if err != nil {
			return CoordWorkflow{}, fmt.Errorf("%w: invalid batchId", ErrInvalid)
		}
		batchID = uuid.NullUUID{UUID: b, Valid: true}
	}

	id := uuid.New()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CoordWorkflow{}, fmt.Errorf("begin workflow transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck -- rollback after commit is harmless
	q := db.New(tx)

	row, err := q.CreateCoordinationWorkflow(ctx, db.CreateCoordinationWorkflowParams{
		ID:               id,
		WorkspaceID:      ws.ID,
		BatchID:          batchID,
		FieldID:          fieldID,
		Status:           db.PfasCoordinationStatusNOTSTARTED,
		CreatedByPartyID: createdBy,
	})
	if err != nil {
		return CoordWorkflow{}, fmt.Errorf("create workflow: %w", err)
	}

	for _, role := range []db.PfasCoordinationStepRole{
		db.PfasCoordinationStepRoleFARMER,
		db.PfasCoordinationStepRoleCONTRACTOR,
		db.PfasCoordinationStepRolePLANT,
	} {
		_, err := q.CreateCoordinationStep(ctx, db.CreateCoordinationStepParams{
			ID:          uuid.New(),
			WorkspaceID: ws.ID,
			WorkflowID:  id,
			StepRole:    role,
			StepType:    string(role) + "_CONFIRM",
			Status:      "PENDING",
		})
		if err != nil {
			return CoordWorkflow{}, fmt.Errorf("create step: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return CoordWorkflow{}, fmt.Errorf("commit workflow: %w", err)
	}

	return CoordWorkflow{
		ID:        row.ID.String(),
		Status:    string(row.Status),
		CreatedBy: createdBy.String(),
		CreatedAt: row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

func (s *Service) AssignStep(ctx context.Context, workspaceKey, stepID string, input CoordAssignStepInput) (CoordStep, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return CoordStep{}, err
	}
	stepUUID, err := uuid.Parse(stepID)
	if err != nil {
		return CoordStep{}, ErrInvalid
	}
	partyUUID, err := uuid.Parse(input.PartyID)
	if err != nil {
		return CoordStep{}, ErrInvalid
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CoordStep{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)
	step, err := q.GetCoordinationStep(ctx, db.GetCoordinationStepParams{ID: stepUUID, WorkspaceID: ws.ID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CoordStep{}, ErrNotFound
		}
		return CoordStep{}, err
	}
	party, err := q.GetParty(ctx, db.GetPartyParams{ID: partyUUID, WorkspaceID: ws.ID})
	if err != nil || string(party.Role) != string(step.StepRole) {
		return CoordStep{}, fmt.Errorf("%w: assigned party must match the step role", ErrInvalid)
	}
	result, err := tx.Exec(ctx, `UPDATE pfas.coordination_steps
		SET party_id = $3
		WHERE id = $1 AND workspace_id = $2 AND status = 'PENDING' AND party_id IS NULL`, stepUUID, ws.ID, partyUUID)
	if err != nil {
		return CoordStep{}, fmt.Errorf("assign step: %w", err)
	}
	if result.RowsAffected() != 1 {
		return CoordStep{}, ErrInvalidTransition
	}
	updated, err := q.GetCoordinationStep(ctx, db.GetCoordinationStepParams{ID: stepUUID, WorkspaceID: ws.ID})
	if err != nil {
		return CoordStep{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CoordStep{}, err
	}
	return stepFromGet(updated), nil
}

func (s *Service) GetWorkflow(ctx context.Context, workspaceKey, workflowID string) (CoordWorkflow, []CoordStep, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return CoordWorkflow{}, nil, err
	}
	wID, err := uuid.Parse(workflowID)
	if err != nil {
		return CoordWorkflow{}, nil, ErrInvalid
	}
	q := db.New(s.pool)
	wRow, err := q.GetCoordinationWorkflow(ctx, db.GetCoordinationWorkflowParams{ID: wID, WorkspaceID: ws.ID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CoordWorkflow{}, nil, ErrNotFound
		}
		return CoordWorkflow{}, nil, err
	}

	stepRows, err := q.ListCoordinationSteps(ctx, db.ListCoordinationStepsParams{WorkflowID: wID, WorkspaceID: ws.ID})
	if err != nil {
		return CoordWorkflow{}, nil, err
	}

	wf := CoordWorkflow{
		ID:            wRow.ID.String(),
		FieldID:       nullUUIDStr(wRow.FieldID),
		Status:        string(wRow.Status),
		CreatedBy:     wRow.CreatedByPartyID.String(),
		CreatedByName: wRow.CreatedByName,
		FieldName:     wRow.FieldName,
		CreatedAt:     wRow.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:     wRow.UpdatedAt.Time.Format(time.RFC3339),
	}

	steps := make([]CoordStep, 0, len(stepRows))
	for _, r := range stepRows {
		s := CoordStep{
			ID:         r.ID.String(),
			WorkflowID: wID.String(),
			PartyID:    nullUUIDPtr(r.PartyID),
			StepRole:   string(r.StepRole),
			StepType:   r.StepType,
			Status:     r.Status,
			Notes:      r.Notes,
			CreatedAt:  r.CreatedAt.Time.Format(time.RFC3339),
		}
		if r.PartyName != nil {
			s.PartyName = *r.PartyName
		}
		if r.PartyEmail != nil {
			s.PartyEmail = *r.PartyEmail
		}
		if r.ConfirmedAt.Valid {
			ts := r.ConfirmedAt.Time.Format(time.RFC3339)
			s.ConfirmedAt = &ts
		}
		steps = append(steps, s)
	}
	return wf, steps, nil
}

func (s *Service) ListWorkflows(ctx context.Context, workspaceKey string) ([]CoordWorkflow, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	rows, err := db.New(s.pool).ListCoordinationWorkflows(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	result := make([]CoordWorkflow, 0, len(rows))
	for _, r := range rows {
		result = append(result, CoordWorkflow{
			ID:            r.ID.String(),
			FieldID:       nullUUIDStr(r.FieldID),
			Status:        string(r.Status),
			CreatedBy:     r.CreatedByPartyID.String(),
			CreatedByName: r.CreatedByName,
			FieldName:     r.FieldName,
			CreatedAt:     r.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:     r.UpdatedAt.Time.Format(time.RFC3339),
		})
	}
	return result, nil
}

func (s *Service) ConfirmStep(ctx context.Context, workspaceKey, stepID string, input CoordConfirmStepInput) (CoordStep, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return CoordStep{}, err
	}
	sID, err := uuid.Parse(stepID)
	if err != nil {
		return CoordStep{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CoordStep{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	currentStep, err := q.GetCoordinationStep(ctx, db.GetCoordinationStepParams{ID: sID, WorkspaceID: ws.ID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CoordStep{}, ErrNotFound
		}
		return CoordStep{}, err
	}

	partyID, err := uuid.Parse(input.PartyID)
	if err != nil {
		return CoordStep{}, ErrInvalid
	}
	if !currentStep.PartyID.Valid || currentStep.PartyID.UUID != partyID || currentStep.Status != "PENDING" {
		return CoordStep{}, fmt.Errorf("%w: only the assigned party can confirm a pending step", ErrInvalidTransition)
	}
	var workflowStatus string
	if err := tx.QueryRow(ctx, `SELECT status::text FROM pfas.coordination_workflows
		WHERE id = $1 AND workspace_id = $2 FOR UPDATE`, currentStep.WorkflowID, ws.ID).Scan(&workflowStatus); err != nil {
		return CoordStep{}, err
	}
	nextStatus, err := nextWorkflowStatus(workflowStatus, string(currentStep.StepRole))
	if err != nil {
		return CoordStep{}, err
	}
	updatedStep, err := q.ConfirmCoordinationStep(ctx, db.ConfirmCoordinationStepParams{
		ID: sID, WorkspaceID: ws.ID, PartyID: uuid.NullUUID{UUID: partyID, Valid: true}, Notes: input.Notes,
	})
	if err != nil {
		return CoordStep{}, fmt.Errorf("confirm step: %w", err)
	}

	if _, err := q.UpdateCoordinationWorkflowStatus(ctx, db.UpdateCoordinationWorkflowStatusParams{
		ID: currentStep.WorkflowID, WorkspaceID: ws.ID, Status: db.PfasCoordinationStatus(nextStatus),
	}); err != nil {
		return CoordStep{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CoordStep{}, err
	}

	return stepFromUpdated(updatedStep), nil
}

func (s *Service) RejectStep(ctx context.Context, workspaceKey, stepID string, input CoordConfirmStepInput) (CoordStep, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return CoordStep{}, err
	}
	sID, err := uuid.Parse(stepID)
	if err != nil {
		return CoordStep{}, ErrInvalid
	}
	if strings.TrimSpace(input.Notes) == "" {
		return CoordStep{}, fmt.Errorf("%w: rejection reason is required", ErrInvalid)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CoordStep{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	currentStep, err := q.GetCoordinationStep(ctx, db.GetCoordinationStepParams{ID: sID, WorkspaceID: ws.ID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CoordStep{}, ErrNotFound
		}
		return CoordStep{}, err
	}
	partyID, err := uuid.Parse(input.PartyID)
	if err != nil || !currentStep.PartyID.Valid || currentStep.PartyID.UUID != partyID || currentStep.Status != "PENDING" {
		return CoordStep{}, fmt.Errorf("%w: only the assigned party can reject a pending step", ErrInvalidTransition)
	}
	var locked int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM pfas.coordination_workflows
		WHERE id = $1 AND workspace_id = $2 FOR UPDATE`, currentStep.WorkflowID, ws.ID).Scan(&locked); err != nil {
		return CoordStep{}, err
	}

	updatedStep, err := q.RejectCoordinationStep(ctx, db.RejectCoordinationStepParams{
		ID: sID, WorkspaceID: ws.ID, Notes: input.Notes,
	})
	if err != nil {
		return CoordStep{}, fmt.Errorf("reject step: %w", err)
	}

	if _, err := q.UpdateCoordinationWorkflowStatus(ctx, db.UpdateCoordinationWorkflowStatusParams{
		ID: currentStep.WorkflowID, WorkspaceID: ws.ID, Status: db.PfasCoordinationStatusREJECTED,
	}); err != nil {
		return CoordStep{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CoordStep{}, err
	}

	return stepFromUpdated(updatedStep), nil
}

func (s *Service) CreateDocument(ctx context.Context, workspaceKey, workflowID string, input CoordCreateDocumentInput) (CoordDocument, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return CoordDocument{}, err
	}
	wID, err := uuid.Parse(workflowID)
	if err != nil {
		return CoordDocument{}, ErrInvalid
	}
	pID, err := uuid.Parse(input.PartyID)
	if err != nil {
		return CoordDocument{}, fmt.Errorf("%w: invalid partyId", ErrInvalid)
	}
	q := db.New(s.pool)
	doc, err := q.CreateCoordinationDocument(ctx, db.CreateCoordinationDocumentParams{
		ID: uuid.New(), WorkspaceID: ws.ID, WorkflowID: wID, PartyID: pID,
		DocType: input.DocType, Filename: input.Filename, FileHash: input.FileHash,
		MimeType: input.MimeType, SizeBytes: input.SizeBytes,
	})
	if err != nil {
		return CoordDocument{}, fmt.Errorf("create document: %w", err)
	}
	return CoordDocument{
		ID: doc.ID.String(), WorkflowID: wID.String(), PartyID: pID.String(),
		DocType: doc.DocType, Filename: doc.Filename, FileHash: doc.FileHash,
		MimeType: doc.MimeType, SizeBytes: doc.SizeBytes,
		CreatedAt: doc.CreatedAt.Time.Format(time.RFC3339),
	}, nil
}

func (s *Service) ListDocuments(ctx context.Context, workspaceKey, workflowID string) ([]CoordDocument, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	wID, err := uuid.Parse(workflowID)
	if err != nil {
		return nil, ErrInvalid
	}
	rows, err := db.New(s.pool).ListCoordinationDocuments(ctx, db.ListCoordinationDocumentsParams{WorkflowID: wID, WorkspaceID: ws.ID})
	if err != nil {
		return nil, err
	}
	result := make([]CoordDocument, 0, len(rows))
	for _, r := range rows {
		result = append(result, CoordDocument{
			ID: r.ID.String(), WorkflowID: wID.String(), PartyID: r.PartyID.String(),
			PartyName: r.PartyName, DocType: r.DocType, Filename: r.Filename,
			FileHash: r.FileHash, MimeType: r.MimeType, SizeBytes: r.SizeBytes,
			CreatedAt: r.CreatedAt.Time.Format(time.RFC3339),
		})
	}
	return result, nil
}

func (s *Service) ListNotifications(ctx context.Context, workspaceKey, partyID string) ([]CoordNotification, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	pID, err := uuid.Parse(partyID)
	if err != nil {
		return nil, ErrInvalid
	}
	rows, err := db.New(s.pool).ListCoordinationNotifications(ctx, db.ListCoordinationNotificationsParams{RecipientPartyID: pID, WorkspaceID: ws.ID})
	if err != nil {
		return nil, err
	}
	result := make([]CoordNotification, 0, len(rows))
	for _, r := range rows {
		n := CoordNotification{
			ID: r.ID.String(), WorkflowID: r.WorkflowID.String(),
			RecipientID: r.RecipientPartyID.String(), RecipientName: r.RecipientName,
			EventType: r.EventType, Message: r.Message,
			CreatedAt: r.CreatedAt.Time.Format(time.RFC3339),
		}
		if r.ReadAt.Valid {
			ts := r.ReadAt.Time.Format(time.RFC3339)
			n.ReadAt = &ts
		}
		result = append(result, n)
	}
	return result, nil
}

func (s *Service) MarkNotificationRead(ctx context.Context, workspaceKey, notificationID string) error {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return err
	}
	nID, err := uuid.Parse(notificationID)
	if err != nil {
		return ErrInvalid
	}
	return db.New(s.pool).MarkNotificationRead(ctx, db.MarkNotificationReadParams{ID: nID, WorkspaceID: ws.ID})
}

func nextWorkflowStatus(current, role string) (string, error) {
	expected := map[string]struct {
		role string
		next string
	}{
		"NOT_STARTED":          {role: "FARMER", next: "FARMER_CONFIRMED"},
		"FARMER_CONFIRMED":     {role: "CONTRACTOR", next: "CONTRACTOR_CONFIRMED"},
		"CONTRACTOR_CONFIRMED": {role: "PLANT", next: "PLANT_CONFIRMED"},
	}
	transition, ok := expected[current]
	if !ok || transition.role != role {
		return "", fmt.Errorf("%w: %s confirmation is not available while workflow is %s", ErrInvalidTransition, role, current)
	}
	return transition.next, nil
}

func stepFromGet(r db.GetCoordinationStepRow) CoordStep {
	step := CoordStep{
		ID: r.ID.String(), WorkflowID: r.WorkflowID.String(), PartyID: nullUUIDPtr(r.PartyID),
		PartyName: nullStr(r.PartyName), StepRole: string(r.StepRole), StepType: r.StepType,
		Status: r.Status, Notes: r.Notes, CreatedAt: r.CreatedAt.Time.Format(time.RFC3339),
	}
	if r.ConfirmedAt.Valid {
		confirmedAt := r.ConfirmedAt.Time.Format(time.RFC3339)
		step.ConfirmedAt = &confirmedAt
	}
	return step
}

func nullUUIDStr(u uuid.NullUUID) string {
	if u.Valid {
		return u.UUID.String()
	}
	return ""
}

func nullUUIDPtr(u uuid.NullUUID) *string {
	if u.Valid {
		s := u.UUID.String()
		return &s
	}
	return nil
}

func nullStr(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func stepFromUpdated(r db.PfasCoordinationStep) CoordStep {
	s := CoordStep{
		ID:         r.ID.String(),
		WorkflowID: r.WorkflowID.String(),
		PartyID:    nullUUIDPtr(r.PartyID),
		StepRole:   string(r.StepRole),
		StepType:   r.StepType,
		Status:     r.Status,
		Notes:      r.Notes,
		CreatedAt:  r.CreatedAt.Time.Format(time.RFC3339),
	}
	if r.ConfirmedAt.Valid {
		ts := r.ConfirmedAt.Time.Format(time.RFC3339)
		s.ConfirmedAt = &ts
	}
	return s
}
