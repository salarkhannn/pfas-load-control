package decisionpackage

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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/lab"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
	"github.com/salarkhannn/pfas-load-control/internal/policy"
	"github.com/salarkhannn/pfas-load-control/internal/responseplan"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound = errors.New("decision package not found")
	ErrInvalid  = errors.New("invalid decision package request")
	ErrConflict = errors.New("decision package prerequisites are incomplete")
)

type Service struct {
	pool      *pgxpool.Pool
	lab       *lab.Service
	policy    *policy.Service
	evidence  *evidence.Service
	placement *placement.Service
	response  *responseplan.Service
	now       func() time.Time
}

func NewService(pool *pgxpool.Pool, labService *lab.Service, policyService *policy.Service, evidenceService *evidence.Service, placementService *placement.Service, responseService *responseplan.Service) *Service {
	return &Service{pool: pool, lab: labService, policy: policyService, evidence: evidenceService, placement: placementService, response: responseService, now: time.Now}
}

func (s *Service) Create(ctx context.Context, workspaceKey, decisionID string) (DecisionPackage, bool, error) {
	workspaceRecord, decisionUUID, contextRow, err := s.context(ctx, workspaceKey, decisionID)
	if err != nil {
		return DecisionPackage{}, false, err
	}
	decision, err := s.policy.Get(ctx, workspaceKey, contextRow.ReportID.String())
	if err != nil {
		return DecisionPackage{}, false, fmt.Errorf("load policy decision: %w", err)
	}
	if decision.Tier == policy.TierUndetermined {
		return DecisionPackage{}, false, fmt.Errorf("%w: resolve the policy classification before generating a package", ErrConflict)
	}
	report, err := s.lab.Get(ctx, workspaceKey, contextRow.ReportID.String())
	if err != nil {
		return DecisionPackage{}, false, fmt.Errorf("load laboratory evidence: %w", err)
	}
	if report.Status != lab.StatusConfirmed || report.Draft == nil {
		return DecisionPackage{}, false, fmt.Errorf("%w: confirm the laboratory evidence before generating a package", ErrConflict)
	}

	var plan *placement.PlacementPlan
	if decision.Tier == policy.TierStandard || decision.Tier == policy.TierElevated {
		loaded, loadErr := s.placement.Latest(ctx, workspaceKey, decisionID)
		if errors.Is(loadErr, placement.ErrNotFound) {
			return DecisionPackage{}, false, fmt.Errorf("%w: build the draft placement plan before generating a package", ErrConflict)
		}
		if loadErr != nil {
			return DecisionPackage{}, false, fmt.Errorf("load placement plan: %w", loadErr)
		}
		plan = &loaded
	}

	var response *responseplan.ResponseRun
	if decision.Tier == policy.TierElevated || decision.Tier == policy.TierProhibited {
		loaded, loadErr := s.response.Latest(ctx, workspaceKey, decisionID)
		if errors.Is(loadErr, responseplan.ErrNotFound) {
			return DecisionPackage{}, false, fmt.Errorf("%w: prepare the required PFAS response before generating a package", ErrConflict)
		}
		if loadErr != nil {
			return DecisionPackage{}, false, fmt.Errorf("load PFAS response: %w", loadErr)
		}
		if loaded.Status == "QUEUED" || loaded.Status == "RUNNING" {
			return DecisionPackage{}, false, fmt.Errorf("%w: wait for the PFAS response to finish before generating a package", ErrConflict)
		}
		if loaded.Status == "FAILED" {
			return DecisionPackage{}, false, fmt.Errorf("%w: resolve the PFAS response failure before generating a package", ErrConflict)
		}
		response = &loaded
	}

	physical := make(map[string]evidence.Evaluation)
	if plan != nil {
		for _, field := range plan.Fields {
			if field.PhysicalEvaluationID == "" {
				continue
			}
			item, loadErr := s.evidence.Get(ctx, workspaceKey, field.PhysicalEvaluationID)
			if loadErr != nil {
				return DecisionPackage{}, false, fmt.Errorf("load physical evidence for %s: %w", field.FieldName, loadErr)
			}
			physical[field.PhysicalEvaluationID] = item
		}
	}

	createdAt := s.now().UTC()
	snapshot := buildSnapshot(decision, report, plan, response, physical)
	ledger := buildEvidenceLedger(snapshot)
	actions := buildProposedActions(snapshot)
	status := packageStatus(snapshot)
	inputHash, err := hashInputs(snapshot)
	if err != nil {
		return DecisionPackage{}, false, err
	}
	queries := db.New(s.pool)
	if existing, getErr := queries.GetDecisionPackageByInput(ctx, db.GetDecisionPackageByInputParams{DecisionID: decisionUUID, InputHash: inputHash}); getErr == nil {
		result, buildErr := hydrate(existing)
		return result, false, buildErr
	} else if !errors.Is(getErr, pgx.ErrNoRows) {
		return DecisionPackage{}, false, fmt.Errorf("check existing decision package: %w", getErr)
	}

	id := uuid.New()
	result := DecisionPackage{
		ID: id.String(), DecisionID: decisionID, SchemaVersion: SchemaVersion, Status: status,
		InputHash: inputHash, Snapshot: snapshot, Evidence: ledger, ProposedActions: actions,
		CreatedAt: createdAt,
	}
	jsonArtifact, err := json.MarshalIndent(exportDocument(result), "", "  ")
	if err != nil {
		return DecisionPackage{}, false, fmt.Errorf("encode decision package: %w", err)
	}
	htmlArtifact, err := renderHTML(result)
	if err != nil {
		return DecisionPackage{}, false, err
	}
	pdfArtifact, err := renderPDF(result)
	if err != nil {
		return DecisionPackage{}, false, err
	}
	result.Artifacts = artifactMetadata(id.String(), jsonArtifact, []byte(htmlArtifact), pdfArtifact)

	snapshotJSON, _ := json.Marshal(snapshot)
	ledgerJSON, _ := json.Marshal(ledger)
	actionsJSON, _ := json.Marshal(actions)
	record, err := queries.CreateDecisionPackage(ctx, db.CreateDecisionPackageParams{
		ID: id, WorkspaceID: workspaceRecord.ID, DecisionID: decisionUUID,
		PlacementEvaluationID: optionalUUID(planID(plan)), ResponseRunID: optionalUUID(responseID(response)),
		SchemaVersion: SchemaVersion, Status: status, InputHash: inputHash,
		Snapshot: snapshotJSON, EvidenceLedger: ledgerJSON, ProposedActions: actionsJSON,
		JsonArtifact: jsonArtifact, HtmlArtifact: htmlArtifact, PdfArtifact: pdfArtifact,
		JsonSha256: digest(jsonArtifact), HtmlSha256: digest([]byte(htmlArtifact)), PdfSha256: digest(pdfArtifact),
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		record, err = queries.GetDecisionPackageByInput(ctx, db.GetDecisionPackageByInputParams{DecisionID: decisionUUID, InputHash: inputHash})
		if err == nil {
			stored, buildErr := hydrate(record)
			return stored, false, buildErr
		}
	}
	if err != nil {
		return DecisionPackage{}, false, fmt.Errorf("store decision package: %w", err)
	}
	stored, err := hydrate(record)
	return stored, true, err
}

func (s *Service) Latest(ctx context.Context, workspaceKey, decisionID string) (DecisionPackage, error) {
	workspaceRecord, decisionUUID, _, err := s.context(ctx, workspaceKey, decisionID)
	if err != nil {
		return DecisionPackage{}, err
	}
	record, err := db.New(s.pool).GetLatestDecisionPackage(ctx, db.GetLatestDecisionPackageParams{DecisionID: decisionUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DecisionPackage{}, ErrNotFound
	}
	if err != nil {
		return DecisionPackage{}, fmt.Errorf("load latest decision package: %w", err)
	}
	return hydrate(record)
}

func (s *Service) Get(ctx context.Context, workspaceKey, packageID string) (DecisionPackage, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return DecisionPackage{}, err
	}
	id, err := uuid.Parse(packageID)
	if err != nil {
		return DecisionPackage{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetDecisionPackage(ctx, db.GetDecisionPackageParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DecisionPackage{}, ErrNotFound
	}
	if err != nil {
		return DecisionPackage{}, fmt.Errorf("load decision package: %w", err)
	}
	return hydrate(record)
}

func (s *Service) Artifact(ctx context.Context, workspaceKey, packageID, format string) (ArtifactContent, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return ArtifactContent{}, err
	}
	id, err := uuid.Parse(packageID)
	if err != nil {
		return ArtifactContent{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetDecisionPackage(ctx, db.GetDecisionPackageParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ArtifactContent{}, ErrNotFound
	}
	if err != nil {
		return ArtifactContent{}, fmt.Errorf("load package artifact: %w", err)
	}
	filename := safeFilename("pfas-decision-" + record.ID.String())
	switch strings.ToLower(format) {
	case "json":
		return ArtifactContent{Filename: filename + ".json", MediaType: "application/json", Content: append([]byte(nil), record.JsonArtifact...)}, nil
	case "html":
		return ArtifactContent{Filename: filename + ".html", MediaType: "text/html; charset=utf-8", Content: []byte(record.HtmlArtifact)}, nil
	case "pdf":
		return ArtifactContent{Filename: filename + ".pdf", MediaType: "application/pdf", Content: append([]byte(nil), record.PdfArtifact...)}, nil
	default:
		return ArtifactContent{}, fmt.Errorf("%w: export format must be html, pdf, or json", ErrInvalid)
	}
}

func (s *Service) context(ctx context.Context, workspaceKey, decisionID string) (db.PfasWorkspace, uuid.UUID, db.GetDecisionPackageContextRow, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, db.GetDecisionPackageContextRow{}, err
	}
	id, err := uuid.Parse(decisionID)
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, db.GetDecisionPackageContextRow{}, ErrNotFound
	}
	row, err := db.New(s.pool).GetDecisionPackageContext(ctx, db.GetDecisionPackageContextParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PfasWorkspace{}, uuid.Nil, db.GetDecisionPackageContextRow{}, ErrNotFound
	}
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, db.GetDecisionPackageContextRow{}, fmt.Errorf("load package context: %w", err)
	}
	return workspaceRecord, id, row, nil
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

func hashInputs(snapshot Snapshot) (string, error) {
	value := struct {
		SchemaVersion string   `json:"schemaVersion"`
		Snapshot      Snapshot `json:"snapshot"`
	}{SchemaVersion: SchemaVersion, Snapshot: snapshot}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode package inputs: %w", err)
	}
	return digest(encoded), nil
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func optionalUUID(value string) uuid.NullUUID {
	id, err := uuid.Parse(value)
	return uuid.NullUUID{UUID: id, Valid: err == nil}
}

func planID(plan *placement.PlacementPlan) string {
	if plan == nil {
		return ""
	}
	return plan.ID
}

func responseID(response *responseplan.ResponseRun) string {
	if response == nil {
		return ""
	}
	return response.ID
}

func safeFilename(value string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
}
