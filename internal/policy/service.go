package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	ErrNotFound     = errors.New("policy decision not found")
	ErrNotConfirmed = errors.New("laboratory evidence must be confirmed before classification")
	ErrRulePack     = errors.New("active policy rule pack unavailable")
)

type Service struct {
	pool    *pgxpool.Pool
	catalog []RulePack
	now     func() time.Time
}

func NewService(pool *pgxpool.Pool, catalog []RulePack) *Service {
	return &Service{pool: pool, catalog: catalog, now: time.Now}
}

func (s *Service) EnsureRulePacks(ctx context.Context) error {
	queries := db.New(s.pool)
	for _, pack := range s.catalog {
		definition, err := json.Marshal(pack)
		if err != nil {
			return fmt.Errorf("encode policy rule pack: %w", err)
		}
		effective, err := pgDate(pack.EffectiveFrom)
		if err != nil {
			return fmt.Errorf("parse policy effective date: %w", err)
		}
		rows, err := queries.InsertPolicyRulePack(ctx, db.InsertPolicyRulePackParams{
			ID: uuid.New(), Code: pack.Code, Version: pack.Version, Jurisdiction: pack.Jurisdiction,
			AuthorityType: pack.AuthorityType, EffectiveFrom: effective,
			RetrievedAt: pgtype.Timestamptz{Time: pack.RetrievedAt, Valid: true},
			SourceUrl:   pack.SourceURL, SourceTitle: pack.SourceTitle, ReviewStatus: pack.ReviewStatus,
			ReviewedBy: &pack.ReviewedBy, ReviewedAt: pgtype.Timestamptz{Time: pack.ReviewedAt, Valid: true},
			Checksum: pack.Checksum, Explanation: pack.Explanation, Definition: definition,
		})
		if err != nil {
			return fmt.Errorf("seed policy rule pack: %w", err)
		}
		if rows == 0 {
			existing, err := queries.GetPolicyRulePackByCodeVersion(ctx, db.GetPolicyRulePackByCodeVersionParams{Code: pack.Code, Version: pack.Version})
			if err != nil {
				return fmt.Errorf("verify policy rule pack: %w", err)
			}
			if existing.Checksum != pack.Checksum {
				return fmt.Errorf("%w: stored %s %s checksum differs from source", ErrRulePack, pack.Code, pack.Version)
			}
		}
	}
	return nil
}

func (s *Service) ActiveRulePack(ctx context.Context, jurisdiction string) (RulePack, error) {
	_, pack, err := s.activeRulePack(ctx, db.New(s.pool), jurisdiction)
	return pack, err
}

func (s *Service) Classify(ctx context.Context, workspaceKey, reportID string) (Decision, bool, error) {
	workspaceRecord, err := s.workspace(ctx, workspaceKey)
	if err != nil {
		return Decision{}, false, err
	}
	id, err := uuid.Parse(reportID)
	if err != nil {
		return Decision{}, false, ErrNotFound
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Decision{}, false, fmt.Errorf("begin policy classification: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	report, err := queries.GetConfirmedReportForClassification(ctx, db.GetConfirmedReportForClassificationParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Decision{}, false, ErrNotFound
	}
	if err != nil {
		return Decision{}, false, fmt.Errorf("load confirmed evidence: %w", err)
	}
	if report.ReportStatus != "CONFIRMED" || report.VersionStatus != "CONFIRMED" {
		return Decision{}, false, ErrNotConfirmed
	}
	packRecord, pack, err := s.activeRulePack(ctx, queries, report.Jurisdiction)
	if err != nil {
		return Decision{}, false, err
	}
	analyteRows, err := queries.ListConfirmedAnalytesForClassification(ctx, db.ListConfirmedAnalytesForClassificationParams{ReportID: id, VersionID: report.ReportVersionID})
	if err != nil {
		return Decision{}, false, fmt.Errorf("load confirmed analytes: %w", err)
	}
	analytes := make([]AnalyteEvidence, 0, len(analyteRows))
	for _, row := range analyteRows {
		analytes = append(analytes, AnalyteEvidence{
			CanonicalAnalyte: row.CanonicalAnalyte, ResultText: row.ResultText, IsNonDetect: row.IsNonDetect,
			NormalizedValueUGKGDry: optional(row.NormalizedValueUgKgDry), UpperBoundUGKGDry: optional(row.UpperBoundUgKgDry), SourcePage: int(row.SourcePage),
		})
	}
	evaluation := Evaluate(pack, ClassificationInput{Jurisdiction: report.Jurisdiction, Matrix: report.Matrix, Method: report.Method, Basis: report.Basis, Analytes: analytes})
	inputJSON, err := json.Marshal(struct {
		ReportVersionID string            `json:"reportVersionId"`
		RuleChecksum    string            `json:"ruleChecksum"`
		Jurisdiction    string            `json:"jurisdiction"`
		Matrix          *string           `json:"matrix"`
		Method          *string           `json:"method"`
		Basis           *string           `json:"basis"`
		Analytes        []AnalyteEvidence `json:"analytes"`
	}{report.ReportVersionID.String(), pack.Checksum, report.Jurisdiction, report.Matrix, report.Method, report.Basis, analytes})
	if err != nil {
		return Decision{}, false, fmt.Errorf("encode classification input: %w", err)
	}
	digest := sha256.Sum256(inputJSON)
	evidence, _ := json.Marshal(analytes)
	decisionID := uuid.New()
	_, err = queries.CreateBatchPolicyDecision(ctx, db.CreateBatchPolicyDecisionParams{
		ID: decisionID, WorkspaceID: workspaceRecord.ID, ReportID: id, ReportVersionID: report.ReportVersionID,
		RulePackID: packRecord.ID, Jurisdiction: report.Jurisdiction, Tier: string(evaluation.Tier),
		MatchedRuleID: optional(evaluation.MatchedRuleID), Explanation: evaluation.Explanation,
		BlockingReason: evaluation.BlockingReason, InputHash: hex.EncodeToString(digest[:]), AnalyteEvidence: evidence,
	})
	created := true
	if errors.Is(err, pgx.ErrNoRows) {
		created = false
		existing, loadErr := queries.GetBatchPolicyDecisionByVersion(ctx, db.GetBatchPolicyDecisionByVersionParams{ReportVersionID: report.ReportVersionID, RulePackID: packRecord.ID})
		if loadErr != nil {
			return Decision{}, false, fmt.Errorf("load existing policy decision: %w", loadErr)
		}
		decisionID = existing.ID
	} else if err != nil {
		return Decision{}, false, fmt.Errorf("store policy decision: %w", err)
	}
	if created {
		for index, requirement := range evaluation.Requirements {
			if err := queries.CreateBatchPolicyRequirement(ctx, db.CreateBatchPolicyRequirementParams{
				ID: uuid.New(), DecisionID: decisionID, Position: int32(index + 1), RequirementID: requirement.ID,
				Title: requirement.Title, Detail: requirement.Detail, Timing: requirement.Timing, RuleID: requirement.RuleID,
				SourceUrl: requirement.SourceURL, SourceTitle: requirement.SourceTitle, AuthorityType: requirement.AuthorityType,
			}); err != nil {
				return Decision{}, false, fmt.Errorf("store policy requirement: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Decision{}, false, fmt.Errorf("commit policy classification: %w", err)
	}
	decision, err := s.Get(ctx, workspaceKey, reportID)
	return decision, created, err
}

func (s *Service) Get(ctx context.Context, workspaceKey, reportID string) (Decision, error) {
	workspaceRecord, err := s.workspace(ctx, workspaceKey)
	if err != nil {
		return Decision{}, err
	}
	id, err := uuid.Parse(reportID)
	if err != nil {
		return Decision{}, ErrNotFound
	}
	queries := db.New(s.pool)
	record, err := queries.GetBatchPolicyDecisionForWorkspace(ctx, db.GetBatchPolicyDecisionForWorkspaceParams{ReportID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Decision{}, ErrNotFound
	}
	if err != nil {
		return Decision{}, fmt.Errorf("load policy decision: %w", err)
	}
	packRecord, err := queries.GetPolicyRulePackByID(ctx, record.RulePackID)
	if err != nil {
		return Decision{}, fmt.Errorf("load decision rule pack: %w", err)
	}
	pack, err := decodePack(packRecord)
	if err != nil {
		return Decision{}, err
	}
	var analytes []AnalyteEvidence
	if err := json.Unmarshal(record.AnalyteEvidence, &analytes); err != nil {
		return Decision{}, fmt.Errorf("decode decision evidence: %w", err)
	}
	requirementRows, err := queries.ListBatchPolicyRequirements(ctx, record.ID)
	if err != nil {
		return Decision{}, fmt.Errorf("load policy requirements: %w", err)
	}
	requirements := make([]Requirement, 0, len(requirementRows))
	for _, row := range requirementRows {
		requirements = append(requirements, Requirement{ID: row.RequirementID, Title: row.Title, Detail: row.Detail, Timing: row.Timing, RuleID: row.RuleID, SourceURL: row.SourceUrl, SourceTitle: row.SourceTitle, AuthorityType: row.AuthorityType})
	}
	maximumRate, prohibitedActions := ruleEffects(pack, record.MatchedRuleID)
	return Decision{
		ID: record.ID.String(), ReportID: record.ReportID.String(), ReportVersion: int(record.ReportVersion),
		FacilityName: record.FacilityName, BatchIdentifier: record.BatchIdentifier, Jurisdiction: record.Jurisdiction,
		Tier: Tier(record.Tier), Explanation: record.Explanation, MatchedRuleID: record.MatchedRuleID,
		BlockingReason: record.BlockingReason, Analytes: analytes, Requirements: requirements, RulePack: pack,
		MaximumApplicationRateDryTonsPerAcre: maximumRate, ProhibitedActions: prohibitedActions,
		InputHash: record.InputHash, CreatedAt: record.CreatedAt.Time,
	}, nil
}

func ruleEffects(pack RulePack, matchedRuleID *string) (*string, []string) {
	if matchedRuleID == nil {
		return nil, []string{}
	}
	for _, rule := range pack.Rules {
		if rule.ID == *matchedRuleID {
			return rule.MaximumApplicationRateDryTonsPerAcre, append([]string{}, rule.ProhibitedActions...)
		}
	}
	return nil, []string{}
}

func (s *Service) activeRulePack(ctx context.Context, queries *db.Queries, jurisdiction string) (db.PfasPolicyRulePack, RulePack, error) {
	today := s.now().UTC()
	records, err := queries.ListApplicablePolicyRulePacks(ctx, db.ListApplicablePolicyRulePacksParams{Jurisdiction: jurisdiction, EffectiveFrom: pgtype.Date{Time: today, Valid: true}})
	if err != nil {
		return db.PfasPolicyRulePack{}, RulePack{}, fmt.Errorf("load active policy rule pack: %w", err)
	}
	if len(records) != 1 {
		return db.PfasPolicyRulePack{}, RulePack{}, fmt.Errorf("%w: expected one active reviewed rule pack for %s; found %d", ErrRulePack, jurisdiction, len(records))
	}
	pack, err := decodePack(records[0])
	return records[0], pack, err
}

func decodePack(record db.PfasPolicyRulePack) (RulePack, error) {
	var pack RulePack
	if err := json.Unmarshal(record.Definition, &pack); err != nil {
		return RulePack{}, fmt.Errorf("decode stored policy rule pack: %w", err)
	}
	pack.Checksum = record.Checksum
	return pack, nil
}

func (s *Service) workspace(ctx context.Context, key string) (db.PfasWorkspace, error) {
	hash, err := workspace.Hash(key)
	if err != nil {
		return db.PfasWorkspace{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetWorkspaceByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PfasWorkspace{}, ErrNotFound
	}
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("load private workspace: %w", err)
	}
	return record, nil
}

func pgDate(value string) (pgtype.Date, error) {
	parsed, err := time.Parse("2006-01-02", value)
	return pgtype.Date{Time: parsed, Valid: err == nil}, err
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
