package placement

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/policy"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound = errors.New("placement evaluation not found")
	ErrInvalid  = errors.New("invalid placement request")
	ErrConflict = errors.New("placement evaluation conflict")
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Create(ctx context.Context, workspaceKey, decisionID string, request PlanInput) (PlacementPlan, bool, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return PlacementPlan{}, false, err
	}
	decisionUUID, err := uuid.Parse(decisionID)
	if err != nil {
		return PlacementPlan{}, false, ErrNotFound
	}
	if (request.WetMassKg == nil) != (request.PercentSolids == nil) {
		return PlacementPlan{}, false, fmt.Errorf("%w: wet mass and total solids must be entered together", ErrInvalid)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return PlacementPlan{}, false, fmt.Errorf("begin placement evaluation: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	placementContext, err := queries.GetPlacementContext(ctx, db.GetPlacementContextParams{ID: decisionUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return PlacementPlan{}, false, ErrNotFound
	}
	if err != nil {
		return PlacementPlan{}, false, fmt.Errorf("load placement context: %w", err)
	}
	if placementContext.Tier == string(policy.TierUndetermined) {
		return PlacementPlan{}, false, fmt.Errorf("%w: resolve the batch classification before comparing fields", ErrConflict)
	}

	wetMass, percentSolids := placementContext.WetMassKg, placementContext.PercentSolids
	if request.WetMassKg != nil {
		wetMass = strings.TrimSpace(*request.WetMassKg)
		percentSolids = strings.TrimSpace(*request.PercentSolids)
		if _, err := dryTons(wetMass, percentSolids); err != nil {
			return PlacementPlan{}, false, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		wetNumeric, err := pgNumeric(wetMass)
		if err != nil {
			return PlacementPlan{}, false, fmt.Errorf("%w: wet mass is invalid", ErrInvalid)
		}
		solidsNumeric, err := pgNumeric(percentSolids)
		if err != nil {
			return PlacementPlan{}, false, fmt.Errorf("%w: total solids are invalid", ErrInvalid)
		}
		updated, err := queries.UpdatePlacementBatchQuantity(ctx, db.UpdatePlacementBatchQuantityParams{
			ID: placementContext.BatchID, WorkspaceID: workspaceRecord.ID,
			WetMassKg: wetNumeric, PercentSolids: solidsNumeric,
		})
		if err != nil {
			return PlacementPlan{}, false, fmt.Errorf("save batch quantity: %w", err)
		}
		if updated != 1 {
			return PlacementPlan{}, false, ErrNotFound
		}
	}

	fields, err := s.loadFields(ctx, queries, workspaceRecord.ID, placementContext.FacilityID)
	if err != nil {
		return PlacementPlan{}, false, err
	}
	policyRate, err := matchedPolicyRate(placementContext.RulePackDefinition, placementContext.MatchedRuleID)
	if err != nil {
		return PlacementPlan{}, false, err
	}
	input := Input{
		Tier: placementContext.Tier, PolicyRate: policyRate,
		WetMassKg: wetMass, PercentSolids: percentSolids, Fields: fields,
		DecisionInputHash: placementContext.DecisionInputHash,
	}
	inputHash, err := hashInput(input)
	if err != nil {
		return PlacementPlan{}, false, err
	}
	result, err := Evaluate(input)
	if err != nil {
		return PlacementPlan{}, false, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	result.InputHash = inputHash

	evaluationID := uuid.New()
	record, err := queries.CreatePlacementEvaluation(ctx, db.CreatePlacementEvaluationParams{
		ID: evaluationID, WorkspaceID: workspaceRecord.ID, DecisionID: decisionUUID,
		BatchID: placementContext.BatchID, Status: string(result.Status), Tier: result.Tier,
		ConfigVersion: result.ConfigVersion, ConfigChecksum: result.ConfigChecksum, InputHash: inputHash,
		WetMassKg: numericOrNull(result.WetMassKg), PercentSolids: numericOrNull(result.PercentSolids),
		BatchDryTons: numericOrNull(result.BatchDryTons), AllocatedDryTons: numericOrNull(result.AllocatedDryTons),
		UnallocatedDryTons: numericOrNull(result.UnallocatedDryTons),
	})
	created := true
	if errors.Is(err, pgx.ErrNoRows) {
		created = false
		record, err = queries.GetPlacementEvaluationByInput(ctx, db.GetPlacementEvaluationByInputParams{DecisionID: decisionUUID, InputHash: inputHash})
	}
	if err != nil {
		return PlacementPlan{}, false, fmt.Errorf("store placement evaluation: %w", err)
	}
	if created {
		if err := persistResult(ctx, queries, record.ID, result); err != nil {
			return PlacementPlan{}, false, err
		}
	}
	stored, err := hydrate(ctx, queries, record)
	if err != nil {
		return PlacementPlan{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PlacementPlan{}, false, fmt.Errorf("commit placement evaluation: %w", err)
	}
	return stored, created, nil
}

func (s *Service) Latest(ctx context.Context, workspaceKey, decisionID string) (PlacementPlan, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return PlacementPlan{}, err
	}
	decisionUUID, err := uuid.Parse(decisionID)
	if err != nil {
		return PlacementPlan{}, ErrNotFound
	}
	queries := db.New(s.pool)
	record, err := queries.GetLatestPlacementEvaluation(ctx, db.GetLatestPlacementEvaluationParams{
		DecisionID: decisionUUID, WorkspaceID: workspaceRecord.ID, ConfigChecksum: ConfigChecksum,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PlacementPlan{}, ErrNotFound
	}
	if err != nil {
		return PlacementPlan{}, fmt.Errorf("load latest placement evaluation: %w", err)
	}
	return hydrate(ctx, queries, record)
}

func (s *Service) loadFields(ctx context.Context, queries *db.Queries, workspaceID, facilityID uuid.UUID) ([]FieldInput, error) {
	rows, err := queries.ListPlacementFieldInputs(ctx, db.ListPlacementFieldInputsParams{WorkspaceID: workspaceID, FacilityID: facilityID})
	if err != nil {
		return nil, fmt.Errorf("load candidate fields: %w", err)
	}
	fields := make([]FieldInput, 0, len(rows))
	for _, row := range rows {
		field := FieldInput{
			ID: row.ID.String(), Name: row.Name, Status: row.Status, RMPApproved: row.RmpApproved,
			UsableAcres: row.UsableAcres, AgronomicRate: row.AgronomicRate,
			PriorLoadingDryTons: row.PriorLoadingDryTons, CropOrUse: row.CropOrUse,
			PhysicalStatus: row.PhysicalStatus, PhysicalCriticalGaps: int(row.PhysicalCriticalGaps),
			PhysicalOtherGaps: int(row.PhysicalOtherGaps), SupplementalAvailable: row.SupplementalAvailable,
			Facts: []FactInput{},
		}
		if row.PhysicalEvaluationID != uuid.Nil {
			field.PhysicalEvaluationID = row.PhysicalEvaluationID.String()
			facts, err := queries.ListPhysicalFieldFacts(ctx, row.PhysicalEvaluationID)
			if err != nil {
				return nil, fmt.Errorf("load physical facts for %s: %w", row.Name, err)
			}
			for _, fact := range facts {
				field.Facts = append(field.Facts, FactInput{
					Name: fact.FieldName, Label: fact.Label, State: fact.State,
					Value: json.RawMessage(fact.Value), Unit: stringPointerValue(fact.Unit),
					Source: stringPointerValue(fact.Source), SourceURL: stringPointerValue(fact.SourceUrl),
				})
			}
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func persistResult(ctx context.Context, queries *db.Queries, evaluationID uuid.UUID, result PlacementPlan) error {
	for _, field := range result.Fields {
		fieldID, err := uuid.Parse(field.FieldID)
		if err != nil {
			return fmt.Errorf("persist placement field: invalid field ID")
		}
		physicalID := uuid.NullUUID{}
		if field.PhysicalEvaluationID != "" {
			id, err := uuid.Parse(field.PhysicalEvaluationID)
			if err != nil {
				return fmt.Errorf("persist placement field: invalid physical evaluation ID")
			}
			physicalID = uuid.NullUUID{UUID: id, Valid: true}
		}
		reasons, err := json.Marshal(field.Reasons)
		if err != nil {
			return fmt.Errorf("encode placement reasons: %w", err)
		}
		var rank *int32
		if field.Rank != nil {
			value := int32(*field.Rank)
			rank = &value
		}
		if err := queries.CreatePlacementFieldResult(ctx, db.CreatePlacementFieldResultParams{
			EvaluationID: evaluationID, FieldID: fieldID, PhysicalEvaluationID: physicalID,
			FieldName: field.FieldName, Disposition: string(field.Disposition), Rank: rank,
			Explanation: field.Explanation, Counterfactual: optionalString(field.Counterfactual),
			HighConcernCount: int32(field.HighConcernCount), ModerateConcernCount: int32(field.ModerateConcernCount),
			DataGapCount: int32(field.DataGapCount), AllowedRateDryTonsAcre: numericOrNull(field.AllowedRate),
			AvailableCapacityDryTons: numericOrNull(field.AvailableCapacity), RoadAccessDistanceM: field.RoadAccessDistanceM,
			Reasons: reasons,
		}); err != nil {
			return fmt.Errorf("store placement field result: %w", err)
		}
		for _, category := range field.Categories {
			components, err := json.Marshal(category.Components)
			if err != nil {
				return fmt.Errorf("encode vulnerability evidence: %w", err)
			}
			if err := queries.CreatePlacementVulnerabilityCategory(ctx, db.CreatePlacementVulnerabilityCategoryParams{
				EvaluationID: evaluationID, FieldID: fieldID, CategoryKey: category.Key,
				Label: category.Label, Band: string(category.Band), Explanation: category.Explanation,
				Components: components, AuthorityType: category.AuthorityType, SourceTitle: category.SourceTitle,
				SourceUrl: optionalString(category.SourceURL), ConfigVersion: category.ConfigVersion,
			}); err != nil {
				return fmt.Errorf("store vulnerability category: %w", err)
			}
		}
	}
	for _, allocation := range result.Allocations {
		fieldID, err := uuid.Parse(allocation.FieldID)
		if err != nil {
			return fmt.Errorf("persist allocation: invalid field ID")
		}
		if err := queries.CreatePlacementAllocation(ctx, db.CreatePlacementAllocationParams{
			EvaluationID: evaluationID, Position: int32(allocation.Position), FieldID: fieldID,
			FieldName: allocation.FieldName, DryTons: numericOrNull(allocation.DryTons),
			Acres: numericOrNull(allocation.Acres), RateDryTonsAcre: numericOrNull(allocation.Rate),
		}); err != nil {
			return fmt.Errorf("store placement allocation: %w", err)
		}
	}
	for _, gap := range result.Gaps {
		if err := queries.CreatePlacementDataGap(ctx, db.CreatePlacementDataGapParams{EvaluationID: evaluationID, Code: gap.Code, Detail: gap.Detail, Resolution: gap.Resolution}); err != nil {
			return fmt.Errorf("store placement data gap: %w", err)
		}
	}
	return nil
}

func hydrate(ctx context.Context, queries *db.Queries, record db.PfasPlacementEvaluation) (PlacementPlan, error) {
	result := PlacementPlan{
		ID: record.ID.String(), DecisionID: record.DecisionID.String(), Status: Status(record.Status), Tier: record.Tier,
		ConfigVersion: record.ConfigVersion, ConfigChecksum: record.ConfigChecksum, InputHash: record.InputHash,
		WetMassKg: numericText(record.WetMassKg), PercentSolids: numericText(record.PercentSolids),
		BatchDryTons: numericText(record.BatchDryTons), AllocatedDryTons: numericText(record.AllocatedDryTons),
		UnallocatedDryTons: numericText(record.UnallocatedDryTons), Fields: []PlacementField{},
		Allocations: []PlacementAllocation{}, Gaps: []PlacementGap{}, CreatedAt: record.CreatedAt.Time,
	}
	fieldRows, err := queries.ListPlacementFieldResults(ctx, record.ID)
	if err != nil {
		return PlacementPlan{}, fmt.Errorf("load placement field results: %w", err)
	}
	fieldIndex := make(map[uuid.UUID]int, len(fieldRows))
	for _, row := range fieldRows {
		var reasons []string
		if err := json.Unmarshal(row.Reasons, &reasons); err != nil {
			return PlacementPlan{}, fmt.Errorf("decode placement reasons: %w", err)
		}
		var rank *int
		if row.Rank != nil {
			value := int(*row.Rank)
			rank = &value
		}
		physicalID := ""
		if row.PhysicalEvaluationID.Valid {
			physicalID = row.PhysicalEvaluationID.UUID.String()
		}
		fieldIndex[row.FieldID] = len(result.Fields)
		result.Fields = append(result.Fields, PlacementField{
			FieldID: row.FieldID.String(), FieldName: row.FieldName, Disposition: Disposition(row.Disposition), Rank: rank,
			Explanation: row.Explanation, Counterfactual: stringPointerValue(row.Counterfactual),
			HighConcernCount: int(row.HighConcernCount), ModerateConcernCount: int(row.ModerateConcernCount),
			DataGapCount: int(row.DataGapCount), AllowedRate: row.AllowedRate, AvailableCapacity: row.AvailableCapacity,
			RoadAccessDistanceM: row.RoadAccessDistanceM, PhysicalEvaluationID: physicalID,
			Reasons: reasons, Categories: []VulnerabilityCategory{},
		})
	}
	categoryRows, err := queries.ListPlacementVulnerabilityCategories(ctx, record.ID)
	if err != nil {
		return PlacementPlan{}, fmt.Errorf("load vulnerability categories: %w", err)
	}
	for _, row := range categoryRows {
		index, ok := fieldIndex[row.FieldID]
		if !ok {
			return PlacementPlan{}, errors.New("placement category has no field result")
		}
		var components []PlacementComponent
		if err := json.Unmarshal(row.Components, &components); err != nil {
			return PlacementPlan{}, fmt.Errorf("decode vulnerability evidence: %w", err)
		}
		result.Fields[index].Categories = append(result.Fields[index].Categories, VulnerabilityCategory{
			Key: row.CategoryKey, Label: row.Label, Band: Band(row.Band), Explanation: row.Explanation,
			Components: components, AuthorityType: row.AuthorityType, SourceTitle: row.SourceTitle,
			SourceURL: stringPointerValue(row.SourceUrl), ConfigVersion: row.ConfigVersion,
		})
	}
	allocationRows, err := queries.ListPlacementAllocations(ctx, record.ID)
	if err != nil {
		return PlacementPlan{}, fmt.Errorf("load placement allocations: %w", err)
	}
	for _, row := range allocationRows {
		result.Allocations = append(result.Allocations, PlacementAllocation{Position: int(row.Position), FieldID: row.FieldID.String(), FieldName: row.FieldName, DryTons: row.DryTons, Acres: row.Acres, Rate: row.Rate})
	}
	gapRows, err := queries.ListPlacementDataGaps(ctx, record.ID)
	if err != nil {
		return PlacementPlan{}, fmt.Errorf("load placement gaps: %w", err)
	}
	for _, row := range gapRows {
		result.Gaps = append(result.Gaps, PlacementGap{Code: row.Code, Detail: row.Detail, Resolution: row.Resolution})
	}
	return result, nil
}

func matchedPolicyRate(definition []byte, matchedRuleID *string) (string, error) {
	if matchedRuleID == nil {
		return "", nil
	}
	var pack policy.RulePack
	if err := json.Unmarshal(definition, &pack); err != nil {
		return "", fmt.Errorf("decode placement policy: %w", err)
	}
	for _, rule := range pack.Rules {
		if rule.ID == *matchedRuleID {
			return stringPointerValue(rule.MaximumApplicationRateDryTonsPerAcre), nil
		}
	}
	return "", fmt.Errorf("%w: matched policy rule is unavailable", ErrConflict)
}

func hashInput(input Input) (string, error) {
	payload, err := json.Marshal(struct {
		ConfigVersion  string `json:"configVersion"`
		ConfigChecksum string `json:"configChecksum"`
		Input
	}{ConfigVersion: ConfigVersion, ConfigChecksum: ConfigChecksum, Input: input})
	if err != nil {
		return "", fmt.Errorf("encode placement input: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
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

func pgNumeric(value string) (pgtype.Numeric, error) {
	var result pgtype.Numeric
	if err := result.Scan(strings.TrimSpace(value)); err != nil {
		return pgtype.Numeric{}, err
	}
	return result, nil
}

func numericOrNull(value string) pgtype.Numeric {
	if value == "" {
		return pgtype.Numeric{}
	}
	result, _ := pgNumeric(value)
	return result
}

func numericText(value pgtype.Numeric) string {
	if !value.Valid {
		return ""
	}
	stored, err := value.Value()
	if err != nil {
		return ""
	}
	return driverString(stored)
}

func driverString(value driver.Value) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
