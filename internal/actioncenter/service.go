package actioncenter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/decisionpackage"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
	"github.com/salarkhannn/pfas-load-control/internal/policy"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound = errors.New("action not found")
	ErrInvalid  = errors.New("invalid action request")
	ErrConflict = errors.New("action state conflict")
	ErrStale    = errors.New("action payload changed")
)

type Service struct {
	pool     *pgxpool.Pool
	packages *decisionpackage.Service
	now      func() time.Time
}

func NewService(pool *pgxpool.Pool, packages *decisionpackage.Service) *Service {
	return &Service{pool: pool, packages: packages, now: time.Now}
}

func (s *Service) Ensure(ctx context.Context, workspaceKey, packageID string) (Center, error) {
	workspaceRecord, packageUUID, pkg, err := s.context(ctx, workspaceKey, packageID)
	if err != nil {
		return Center{}, err
	}
	queries := db.New(s.pool)
	for _, proposal := range pkg.ProposedActions {
		mode, approvalRequired := actionMode(proposal)
		payload := defaultPayload(pkg, proposal, mode)
		attachments, err := json.Marshal(payload.Attachments)
		if err != nil {
			return Center{}, fmt.Errorf("encode action attachments: %w", err)
		}
		status := StatusProposed
		if mode == ModeControl {
			status = StatusExecuted
		}
		createdAt := s.now().UTC()
		_, err = queries.CreateAction(ctx, db.CreateActionParams{
			ID: uuid.New(), WorkspaceID: workspaceRecord.ID, DecisionPackageID: packageUUID,
			Position: int32(proposal.Position), Code: proposal.Code, Category: proposal.Category,
			Title: proposal.Title, Detail: proposal.Detail, Timing: proposal.Timing, SourceID: proposal.SourceID,
			ExecutionMode: string(mode), Status: string(status), ApprovalRequired: approvalRequired,
			Channel: payload.Channel, Recipient: payload.Recipient, Subject: payload.Subject,
			Message: payload.Message, Attachments: attachments, Revision: 1,
			PayloadHash: hashPayload(payload), CreatedAt: timestamp(createdAt),
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Center{}, fmt.Errorf("store action %s: %w", proposal.Code, err)
		}
	}
	return s.loadCenter(ctx, workspaceRecord.ID, pkg)
}

func (s *Service) Get(ctx context.Context, workspaceKey, packageID string) (Center, error) {
	workspaceRecord, _, pkg, err := s.context(ctx, workspaceKey, packageID)
	if err != nil {
		return Center{}, err
	}
	center, err := s.loadCenter(ctx, workspaceRecord.ID, pkg)
	if err == nil && len(center.Actions) == 0 {
		return Center{}, ErrNotFound
	}
	return center, err
}

func (s *Service) UpdatePayload(ctx context.Context, workspaceKey, actionID string, input UpdatePayloadInput) (ControlledAction, error) {
	workspaceRecord, actionUUID, err := s.actionContext(ctx, workspaceKey, actionID)
	if err != nil {
		return ControlledAction{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ControlledAction{}, fmt.Errorf("begin payload update: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	record, err := queries.GetActionForUpdate(ctx, db.GetActionForUpdateParams{ID: actionUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ControlledAction{}, ErrNotFound
	}
	if err != nil {
		return ControlledAction{}, fmt.Errorf("lock action: %w", err)
	}
	if record.ExecutionMode == string(ModeControl) || record.Status == string(StatusExecuted) {
		return ControlledAction{}, fmt.Errorf("%w: completed controls and actions cannot be edited", ErrConflict)
	}
	payload, err := payloadFromRecord(record)
	if err != nil {
		return ControlledAction{}, err
	}
	payload.Recipient = strings.TrimSpace(input.Recipient)
	payload.Subject = strings.TrimSpace(input.Subject)
	payload.Message = strings.TrimSpace(input.Message)
	if err := validateDraftPayload(payload); err != nil {
		return ControlledAction{}, err
	}
	newHash := hashPayload(payload)
	if newHash == record.PayloadHash && record.Status != string(StatusRejected) {
		return hydrateAction(record, nil, nil)
	}
	attachments, _ := json.Marshal(payload.Attachments)
	record, err = queries.UpdateActionPayload(ctx, db.UpdateActionPayloadParams{
		ID: actionUUID, WorkspaceID: workspaceRecord.ID, Recipient: payload.Recipient,
		Subject: payload.Subject, Message: payload.Message, Attachments: attachments,
		PayloadHash: newHash, UpdatedAt: timestamp(s.now().UTC()),
	})
	if err != nil {
		return ControlledAction{}, fmt.Errorf("update action payload: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ControlledAction{}, fmt.Errorf("commit payload update: %w", err)
	}
	return hydrateAction(record, nil, nil)
}

func (s *Service) Approve(ctx context.Context, workspaceKey, actionID string, input DecisionInput) (ControlledAction, error) {
	return s.decide(ctx, workspaceKey, actionID, "APPROVED", input)
}

func (s *Service) Reject(ctx context.Context, workspaceKey, actionID string, input DecisionInput) (ControlledAction, error) {
	return s.decide(ctx, workspaceKey, actionID, "REJECTED", input)
}

func (s *Service) decide(ctx context.Context, workspaceKey, actionID, kind string, input DecisionInput) (ControlledAction, error) {
	workspaceRecord, actionUUID, err := s.actionContext(ctx, workspaceKey, actionID)
	if err != nil {
		return ControlledAction{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ControlledAction{}, fmt.Errorf("begin action decision: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	record, err := queries.GetActionForUpdate(ctx, db.GetActionForUpdateParams{ID: actionUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ControlledAction{}, ErrNotFound
	}
	if err != nil {
		return ControlledAction{}, fmt.Errorf("lock action: %w", err)
	}
	if record.ExecutionMode == string(ModeControl) || record.Status == string(StatusExecuted) {
		return ControlledAction{}, fmt.Errorf("%w: this action does not accept a decision", ErrConflict)
	}
	if strings.TrimSpace(input.ExpectedPayloadHash) != record.PayloadHash {
		return ControlledAction{}, fmt.Errorf("%w: review the current payload before deciding", ErrStale)
	}
	payload, err := payloadFromRecord(record)
	if err != nil {
		return ControlledAction{}, err
	}
	if kind == "APPROVED" {
		if err := validatePayload(payload); err != nil {
			return ControlledAction{}, err
		}
	}
	actorName, actorRole := strings.TrimSpace(input.ActorName), strings.TrimSpace(input.ActorRole)
	if len(actorName) < 2 || len(actorRole) < 2 {
		return ControlledAction{}, fmt.Errorf("%w: enter the reviewer name and role", ErrInvalid)
	}
	pkg, err := s.packages.Get(ctx, workspaceKey, record.DecisionPackageID.String())
	if err != nil {
		return ControlledAction{}, fmt.Errorf("load decision package: %w", err)
	}
	critical, review := splitGaps(pkg.Snapshot.Gaps)
	if kind == "APPROVED" && len(critical) > 0 {
		return ControlledAction{}, fmt.Errorf("%w: resolve critical evidence gaps before approval", ErrConflict)
	}
	acknowledged := make([]string, 0, len(review))
	if kind == "APPROVED" && len(review) > 0 {
		if !input.AcknowledgeGaps {
			return ControlledAction{}, fmt.Errorf("%w: acknowledge the listed review gaps before approval", ErrConflict)
		}
		for _, gap := range review {
			acknowledged = append(acknowledged, gap.Code)
		}
	}
	note := strings.TrimSpace(input.Note)
	if kind == "REJECTED" && len(note) < 2 {
		return ControlledAction{}, fmt.Errorf("%w: add a reason for rejection", ErrInvalid)
	}
	acknowledgedJSON, _ := json.Marshal(acknowledged)
	decisionRecord, err := queries.CreateActionDecision(ctx, db.CreateActionDecisionParams{
		ID: uuid.New(), WorkspaceID: workspaceRecord.ID, ActionID: actionUUID,
		DecisionPackageID: record.DecisionPackageID, Kind: kind, ActionRevision: record.Revision,
		PayloadHash: record.PayloadHash, ActorName: actorName, ActorRole: actorRole, Note: note,
		AcknowledgedGapCodes: acknowledgedJSON, CreatedAt: timestamp(s.now().UTC()),
	})
	if err != nil {
		if isUniqueViolation(err, "action_decisions_action_id_action_revision_key") {
			return ControlledAction{}, fmt.Errorf("%w: this revision already has a decision", ErrConflict)
		}
		return ControlledAction{}, fmt.Errorf("store action decision: %w", err)
	}
	record, err = queries.UpdateActionStatus(ctx, db.UpdateActionStatusParams{
		ID: actionUUID, WorkspaceID: workspaceRecord.ID, Status: kind, UpdatedAt: timestamp(s.now().UTC()),
	})
	if err != nil {
		return ControlledAction{}, fmt.Errorf("update action status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ControlledAction{}, fmt.Errorf("commit action decision: %w", err)
	}
	decision, err := hydrateDecision(decisionRecord)
	if err != nil {
		return ControlledAction{}, err
	}
	return hydrateAction(record, []ApprovalDecision{decision}, nil)
}

func (s *Service) Execute(ctx context.Context, workspaceKey, actionID, idempotencyKey string) (ControlledAction, error) {
	key := strings.TrimSpace(idempotencyKey)
	if len(key) < 16 || len(key) > 128 {
		return ControlledAction{}, fmt.Errorf("%w: idempotency key must contain 16 to 128 characters", ErrInvalid)
	}
	workspaceRecord, actionUUID, err := s.actionContext(ctx, workspaceKey, actionID)
	if err != nil {
		return ControlledAction{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ControlledAction{}, fmt.Errorf("begin controlled execution: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	if existing, getErr := queries.GetExecutionByIdempotencyKey(ctx, db.GetExecutionByIdempotencyKeyParams{ActionID: actionUUID, IdempotencyKey: key}); getErr == nil {
		record, lockErr := queries.GetActionForUpdate(ctx, db.GetActionForUpdateParams{ID: actionUUID, WorkspaceID: workspaceRecord.ID})
		if lockErr != nil {
			return ControlledAction{}, fmt.Errorf("load executed action: %w", lockErr)
		}
		receipt, hydrateErr := hydrateExecution(existing)
		if hydrateErr != nil {
			return ControlledAction{}, hydrateErr
		}
		return hydrateAction(record, nil, &receipt)
	} else if !errors.Is(getErr, pgx.ErrNoRows) {
		return ControlledAction{}, fmt.Errorf("check execution key: %w", getErr)
	}
	record, err := queries.GetActionForUpdate(ctx, db.GetActionForUpdateParams{ID: actionUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ControlledAction{}, ErrNotFound
	}
	if err != nil {
		return ControlledAction{}, fmt.Errorf("lock action: %w", err)
	}
	if record.ExecutionMode == string(ModeControl) {
		return ControlledAction{}, fmt.Errorf("%w: this control is already in effect", ErrConflict)
	}
	if record.Status == string(StatusExecuted) {
		executions, listErr := queries.ListExecutionsForPackage(ctx, db.ListExecutionsForPackageParams{DecisionPackageID: record.DecisionPackageID, WorkspaceID: workspaceRecord.ID})
		if listErr != nil {
			return ControlledAction{}, fmt.Errorf("load execution receipt: %w", listErr)
		}
		for _, item := range executions {
			if item.ActionID == record.ID {
				receipt, hydrateErr := hydrateExecution(item)
				if hydrateErr != nil {
					return ControlledAction{}, hydrateErr
				}
				return hydrateAction(record, nil, &receipt)
			}
		}
		return ControlledAction{}, fmt.Errorf("%w: execution receipt is missing", ErrConflict)
	}
	if record.Status != string(StatusApproved) {
		return ControlledAction{}, fmt.Errorf("%w: approve the current payload before execution", ErrConflict)
	}
	approval, err := queries.GetCurrentApproval(ctx, db.GetCurrentApprovalParams{
		ActionID: actionUUID, WorkspaceID: workspaceRecord.ID, ActionRevision: record.Revision, PayloadHash: record.PayloadHash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ControlledAction{}, fmt.Errorf("%w: approval does not match the current payload", ErrStale)
	}
	if err != nil {
		return ControlledAction{}, fmt.Errorf("load current approval: %w", err)
	}
	pkg, err := s.packages.Get(ctx, workspaceKey, record.DecisionPackageID.String())
	if err != nil {
		return ControlledAction{}, fmt.Errorf("load decision package: %w", err)
	}
	completedAt := s.now().UTC()
	executionID := uuid.New()
	receipt := ActionExecutionReceipt{ID: executionID.String(), CompletedAt: completedAt}
	var artifact []byte
	var artifactHash *string
	if record.ExecutionMode == string(ModeInternalRelease) {
		if pkg.Snapshot.Decision.Tier == policy.TierProhibited || pkg.Snapshot.Placement == nil || pkg.Snapshot.Placement.Status != placement.StatusReady {
			return ControlledAction{}, fmt.Errorf("%w: the frozen placement plan is not releasable", ErrConflict)
		}
		placementID, parseErr := uuid.Parse(pkg.Snapshot.Placement.ID)
		if parseErr != nil || pkg.Snapshot.Placement.ID != record.SourceID {
			return ControlledAction{}, fmt.Errorf("%w: the approved action does not match the frozen placement plan", ErrConflict)
		}
		if _, getErr := queries.GetPlacementRelease(ctx, db.GetPlacementReleaseParams{PlacementEvaluationID: placementID, WorkspaceID: workspaceRecord.ID}); getErr == nil {
			return ControlledAction{}, fmt.Errorf("%w: this frozen placement plan was already released", ErrConflict)
		} else if !errors.Is(getErr, pgx.ErrNoRows) {
			return ControlledAction{}, fmt.Errorf("check placement release: %w", getErr)
		}
		receipt.Outcome = "INTERNAL_RELEASED"
		receipt.Summary = "The frozen placement plan was released for internal scheduling. No external party was contacted."
		releaseID := uuid.New()
		receipt.ReleaseID = releaseID.String()
		receiptJSON, _ := json.Marshal(receipt)
		_, err = queries.CreateExecutionAttempt(ctx, db.CreateExecutionAttemptParams{
			ID: executionID, WorkspaceID: workspaceRecord.ID, ActionID: actionUUID, ApprovalID: approval.ID,
			IdempotencyKey: key, Outcome: receipt.Outcome, Receipt: receiptJSON, CompletedAt: timestamp(completedAt),
		})
		if err != nil {
			return ControlledAction{}, fmt.Errorf("record internal release: %w", err)
		}
		_, err = queries.CreatePlacementRelease(ctx, db.CreatePlacementReleaseParams{
			ID: releaseID, WorkspaceID: workspaceRecord.ID, DecisionPackageID: record.DecisionPackageID,
			PlacementEvaluationID: placementID, ActionID: actionUUID, ApprovalID: approval.ID,
			ExecutionAttemptID: executionID, ReleasedAt: timestamp(completedAt),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ControlledAction{}, fmt.Errorf("%w: this frozen placement plan was already released", ErrConflict)
		}
		if err != nil {
			return ControlledAction{}, fmt.Errorf("release placement plan: %w", err)
		}
	} else {
		payload, payloadErr := payloadFromRecord(record)
		if payloadErr != nil {
			return ControlledAction{}, payloadErr
		}
		approvalSnapshot, hydrateErr := hydrateDecision(approval)
		if hydrateErr != nil {
			return ControlledAction{}, hydrateErr
		}
		handoff := struct {
			SchemaVersion string           `json:"schemaVersion"`
			PackageID     string           `json:"packageId"`
			ActionID      string           `json:"actionId"`
			ActionCode    string           `json:"actionCode"`
			Revision      int32            `json:"revision"`
			PayloadHash   string           `json:"payloadHash"`
			Payload       ActionPayload    `json:"payload"`
			Approval      ApprovalDecision `json:"approval"`
			PreparedAt    time.Time        `json:"preparedAt"`
			Notice        string           `json:"notice"`
		}{"module-08.1", record.DecisionPackageID.String(), record.ID.String(), record.Code, record.Revision, record.PayloadHash, payload, approvalSnapshot, completedAt, "Operator handoff only. No external system or person was contacted."}
		artifact, err = json.MarshalIndent(handoff, "", "  ")
		if err != nil {
			return ControlledAction{}, fmt.Errorf("encode operator handoff: %w", err)
		}
		digest := sha256.Sum256(artifact)
		hash := hex.EncodeToString(digest[:])
		artifactHash = &hash
		receipt.Outcome = "OPERATOR_HANDOFF_READY"
		receipt.Summary = "The exact approved payload was frozen into an operator handoff. No external party was contacted."
		receipt.HandoffURL = "/api/v1/execution-attempts/" + executionID.String() + "/handoff"
		receiptJSON, _ := json.Marshal(receipt)
		_, err = queries.CreateExecutionAttempt(ctx, db.CreateExecutionAttemptParams{
			ID: executionID, WorkspaceID: workspaceRecord.ID, ActionID: actionUUID, ApprovalID: approval.ID,
			IdempotencyKey: key, Outcome: receipt.Outcome, Receipt: receiptJSON,
			HandoffArtifact: artifact, HandoffSha256: artifactHash, CompletedAt: timestamp(completedAt),
		})
		if err != nil {
			return ControlledAction{}, fmt.Errorf("record operator handoff: %w", err)
		}
	}
	record, err = queries.UpdateActionStatus(ctx, db.UpdateActionStatusParams{ID: actionUUID, WorkspaceID: workspaceRecord.ID, Status: string(StatusExecuted), UpdatedAt: timestamp(completedAt)})
	if err != nil {
		return ControlledAction{}, fmt.Errorf("complete controlled execution: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ControlledAction{}, fmt.Errorf("commit controlled execution: %w", err)
	}
	return hydrateAction(record, nil, &receipt)
}

func (s *Service) Handoff(ctx context.Context, workspaceKey, executionID string) (ArtifactContent, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return ArtifactContent{}, err
	}
	id, err := uuid.Parse(executionID)
	if err != nil {
		return ArtifactContent{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetExecutionAttempt(ctx, db.GetExecutionAttemptParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) || record.Outcome != "OPERATOR_HANDOFF_READY" || len(record.HandoffArtifact) == 0 {
		return ArtifactContent{}, ErrNotFound
	}
	if err != nil {
		return ArtifactContent{}, fmt.Errorf("load operator handoff: %w", err)
	}
	return ArtifactContent{Filename: "pfas-action-handoff-" + record.ActionID.String() + ".json", MediaType: "application/json", Content: append([]byte(nil), record.HandoffArtifact...)}, nil
}

func (s *Service) context(ctx context.Context, workspaceKey, packageID string) (db.PfasWorkspace, uuid.UUID, decisionpackage.DecisionPackage, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, decisionpackage.DecisionPackage{}, err
	}
	id, err := uuid.Parse(packageID)
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, decisionpackage.DecisionPackage{}, ErrNotFound
	}
	pkg, err := s.packages.Get(ctx, workspaceKey, packageID)
	if errors.Is(err, decisionpackage.ErrNotFound) {
		return db.PfasWorkspace{}, uuid.Nil, decisionpackage.DecisionPackage{}, ErrNotFound
	}
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, decisionpackage.DecisionPackage{}, fmt.Errorf("load decision package: %w", err)
	}
	return workspaceRecord, id, pkg, nil
}

func (s *Service) actionContext(ctx context.Context, workspaceKey, actionID string) (db.PfasWorkspace, uuid.UUID, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, err
	}
	id, err := uuid.Parse(actionID)
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, ErrNotFound
	}
	return workspaceRecord, id, nil
}

func (s *Service) loadWorkspace(ctx context.Context, key string) (db.PfasWorkspace, error) {
	keyHash, err := workspace.Hash(key)
	if err != nil {
		return db.PfasWorkspace{}, ErrInvalid
	}
	record, err := db.New(s.pool).GetWorkspaceByHash(ctx, keyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PfasWorkspace{}, ErrNotFound
	}
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("load workspace: %w", err)
	}
	return record, nil
}

func (s *Service) loadCenter(ctx context.Context, workspaceID uuid.UUID, pkg decisionpackage.DecisionPackage) (Center, error) {
	packageID, _ := uuid.Parse(pkg.ID)
	queries := db.New(s.pool)
	records, err := queries.ListActionsForPackage(ctx, db.ListActionsForPackageParams{DecisionPackageID: packageID, WorkspaceID: workspaceID})
	if err != nil {
		return Center{}, fmt.Errorf("load package actions: %w", err)
	}
	decisions, err := queries.ListActionDecisionsForPackage(ctx, db.ListActionDecisionsForPackageParams{DecisionPackageID: packageID, WorkspaceID: workspaceID})
	if err != nil {
		return Center{}, fmt.Errorf("load action decisions: %w", err)
	}
	executions, err := queries.ListExecutionsForPackage(ctx, db.ListExecutionsForPackageParams{DecisionPackageID: packageID, WorkspaceID: workspaceID})
	if err != nil {
		return Center{}, fmt.Errorf("load action executions: %w", err)
	}
	byAction := make(map[uuid.UUID][]ApprovalDecision)
	for _, record := range decisions {
		item, hydrateErr := hydrateDecision(record)
		if hydrateErr != nil {
			return Center{}, hydrateErr
		}
		byAction[record.ActionID] = append(byAction[record.ActionID], item)
	}
	latestExecution := make(map[uuid.UUID]ActionExecutionReceipt)
	for _, record := range executions {
		item, hydrateErr := hydrateExecution(record)
		if hydrateErr != nil {
			return Center{}, hydrateErr
		}
		latestExecution[record.ActionID] = item
	}
	actions := make([]ControlledAction, 0, len(records))
	for _, record := range records {
		var receipt *ActionExecutionReceipt
		if item, ok := latestExecution[record.ID]; ok {
			copy := item
			receipt = &copy
		}
		item, hydrateErr := hydrateAction(record, byAction[record.ID], receipt)
		if hydrateErr != nil {
			return Center{}, hydrateErr
		}
		actions = append(actions, item)
	}
	critical, review := splitGaps(pkg.Snapshot.Gaps)
	return Center{PackageID: pkg.ID, PackageHash: pkg.InputHash, PackageStatus: pkg.Status, CriticalGaps: critical, ReviewGaps: review, Actions: actions, ApprovalPolicy: "Critical gaps block approval. Review gaps require an explicit acknowledgement."}, nil
}

func actionMode(proposal decisionpackage.ProposedAction) (ExecutionMode, bool) {
	if proposal.Code == "ENFORCE_RATE_CAP" || proposal.Code == "BLOCK_LAND_APPLICATION" || proposal.State == "ENFORCED" {
		return ModeControl, false
	}
	if proposal.Code == "REVIEW_DRAFT_ALLOCATION" {
		return ModeInternalRelease, true
	}
	return ModeOperatorHandoff, true
}

func defaultPayload(pkg decisionpackage.DecisionPackage, proposal decisionpackage.ProposedAction, mode ExecutionMode) ActionPayload {
	channel, recipient := "INTERNAL_WORK_ORDER", "Facility pretreatment and operations team"
	switch {
	case mode == ModeInternalRelease:
		channel, recipient = "INTERNAL_RELEASE", "Facility operations"
	case mode == ModeControl:
		channel, recipient = "CONTROL", "PFAS Load Control"
	case strings.Contains(proposal.Code, "RESULT-SUBMISSION") || proposal.Code == "NOTIFY_EGLE":
		channel, recipient = "MIENVIRO", "Michigan EGLE"
	case strings.Contains(proposal.Code, "LANDOWNER"):
		channel, recipient = "OPERATOR_DELIVERY", ""
	case strings.Contains(proposal.Code, "SAMPLE") || strings.Contains(proposal.Code, "SAMPLING"):
		channel, recipient = "SAMPLING_REQUEST", ""
	case strings.Contains(proposal.Code, "ALTERNATIVE"):
		channel, recipient = "QUOTE_REQUEST", ""
	}
	attachments := make([]ActionAttachment, 0, len(pkg.Artifacts))
	for _, artifact := range pkg.Artifacts {
		if artifact.Format != "pdf" && artifact.Format != "json" {
			continue
		}
		attachments = append(attachments, ActionAttachment{Label: "Frozen decision package (" + strings.ToUpper(artifact.Format) + ")", Format: artifact.Format, MediaType: artifact.MediaType, SHA256: artifact.SHA256, URL: artifact.URL})
	}
	message := proposal.Detail
	if proposal.Timing != "" {
		message += " Timing: " + strings.TrimSpace(proposal.Timing)
	}
	return ActionPayload{Channel: channel, Recipient: recipient, Subject: proposal.Title, Message: message, Attachments: attachments}
}

func splitGaps(gaps []decisionpackage.PackageGap) ([]ReviewGap, []ReviewGap) {
	critical, review := make([]ReviewGap, 0), make([]ReviewGap, 0)
	for _, item := range gaps {
		gap := ReviewGap{Code: item.Code, Detail: item.Detail, Resolution: item.Resolution}
		if item.Critical {
			critical = append(critical, gap)
		} else {
			review = append(review, gap)
		}
	}
	return critical, review
}

func payloadFromRecord(record db.PfasAction) (ActionPayload, error) {
	var attachments []ActionAttachment
	if err := json.Unmarshal(record.Attachments, &attachments); err != nil {
		return ActionPayload{}, fmt.Errorf("decode action attachments: %w", err)
	}
	return ActionPayload{Channel: record.Channel, Recipient: record.Recipient, Subject: record.Subject, Message: record.Message, Attachments: attachments}, nil
}

func validatePayload(payload ActionPayload) error {
	if len(strings.TrimSpace(payload.Recipient)) < 2 {
		return fmt.Errorf("%w: enter the exact recipient before approval", ErrInvalid)
	}
	return validateDraftPayload(payload)
}

func validateDraftPayload(payload ActionPayload) error {
	if len(strings.TrimSpace(payload.Subject)) < 2 || len(strings.TrimSpace(payload.Message)) < 2 {
		return fmt.Errorf("%w: subject and message are required", ErrInvalid)
	}
	return nil
}

func hashPayload(payload ActionPayload) string {
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func hydrateAction(record db.PfasAction, decisions []ApprovalDecision, execution *ActionExecutionReceipt) (ControlledAction, error) {
	payload, err := payloadFromRecord(record)
	if err != nil {
		return ControlledAction{}, err
	}
	if decisions == nil {
		decisions = []ApprovalDecision{}
	}
	return ControlledAction{
		ID: record.ID.String(), PackageID: record.DecisionPackageID.String(), Position: int(record.Position),
		Code: record.Code, Category: record.Category, Title: record.Title, Detail: record.Detail,
		Timing: record.Timing, SourceID: record.SourceID, ExecutionMode: ExecutionMode(record.ExecutionMode),
		Status: ActionStatus(record.Status), ApprovalRequired: record.ApprovalRequired, Payload: payload,
		Revision: int(record.Revision), PayloadHash: record.PayloadHash, Decisions: decisions, Execution: execution,
		CreatedAt: timeValue(record.CreatedAt), UpdatedAt: timeValue(record.UpdatedAt),
	}, nil
}

func hydrateDecision(record db.PfasActionDecision) (ApprovalDecision, error) {
	var gaps []string
	if err := json.Unmarshal(record.AcknowledgedGapCodes, &gaps); err != nil {
		return ApprovalDecision{}, fmt.Errorf("decode acknowledged gaps: %w", err)
	}
	return ApprovalDecision{ID: record.ID.String(), Kind: record.Kind, ActionRevision: int(record.ActionRevision), PayloadHash: record.PayloadHash, ActorName: record.ActorName, ActorRole: record.ActorRole, Note: record.Note, AcknowledgedGapCodes: gaps, CreatedAt: timeValue(record.CreatedAt)}, nil
}

func hydrateExecution(record db.PfasExecutionAttempt) (ActionExecutionReceipt, error) {
	var receipt ActionExecutionReceipt
	if err := json.Unmarshal(record.Receipt, &receipt); err != nil {
		return ActionExecutionReceipt{}, fmt.Errorf("decode execution receipt: %w", err)
	}
	return receipt, nil
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
