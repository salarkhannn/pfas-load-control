package responseplan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/mireye"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

const (
	echoReportURL = "https://echo.epa.gov/detailed-facility-report?fid="
	echoSourceURL = "https://echo.epa.gov/facilities/facility-search"
)

var (
	ErrNotFound = errors.New("response record not found")
	ErrInvalid  = errors.New("invalid response request")
	ErrConflict = errors.New("response request conflicts with current state")
)

type mireyeSource interface {
	Lookup(context.Context, mireye.LookupRequest) (mireye.LookupResult, error)
	FetchBatch(context.Context, mireye.FetchBatchRequest) (mireye.FetchBatchResult, error)
	Distance(context.Context, mireye.DistanceRequest) (mireye.DistanceResult, error)
}

type echoSource interface {
	FetchECHOPotentialSectors(context.Context, float64, float64) (evidence.ECHOResult, error)
}

type Service struct {
	pool      *pgxpool.Pool
	jobs      *river.Client[pgx.Tx]
	mireye    mireyeSource
	echo      echoSource
	landfills LandfillSource
}

func NewService(pool *pgxpool.Pool, jobs *river.Client[pgx.Tx], mireyeClient mireyeSource, echo echoSource, landfills LandfillSource) *Service {
	return &Service{pool: pool, jobs: jobs, mireye: mireyeClient, echo: echo, landfills: landfills}
}

func (s *Service) ResolveLocation(ctx context.Context, workspaceKey, decisionID string, input LocationInput) (FacilityLocation, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return FacilityLocation{}, err
	}
	decisionUUID, err := uuid.Parse(decisionID)
	if err != nil {
		return FacilityLocation{}, ErrNotFound
	}
	decision, err := db.New(s.pool).GetResponseDecisionContext(ctx, db.GetResponseDecisionContextParams{ID: decisionUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return FacilityLocation{}, ErrNotFound
	}
	if err != nil {
		return FacilityLocation{}, fmt.Errorf("load response decision: %w", err)
	}
	input.Input = strings.TrimSpace(input.Input)
	kind := mireye.LookupKind(input.Kind)
	if kind != mireye.LookupAddress && kind != mireye.LookupCoordinate {
		return FacilityLocation{}, fmt.Errorf("%w: facility location must be an address or coordinates", ErrInvalid)
	}
	request := mireye.LookupRequest{Input: input.Input, Kind: kind}
	requestHash, err := mireye.LookupRequestHash(request)
	if err != nil {
		return FacilityLocation{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	queries := db.New(s.pool)
	existing, err := queries.GetFacilityLocationLookupByHash(ctx, db.GetFacilityLocationLookupByHashParams{
		FacilityID: decision.FacilityID, WorkspaceID: workspaceRecord.ID, RequestHash: requestHash,
	})
	if err == nil {
		return locationFromRecord(existing), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return FacilityLocation{}, fmt.Errorf("load facility location: %w", err)
	}
	lookup, err := s.mireye.Lookup(ctx, request)
	if err != nil {
		return FacilityLocation{}, fmt.Errorf("resolve facility location: %w", err)
	}
	candidates, err := json.Marshal(lookup.Response.Candidates)
	if err != nil {
		return FacilityLocation{}, fmt.Errorf("encode facility location candidates: %w", err)
	}
	record, err := queries.CreateFacilityLocationLookup(ctx, db.CreateFacilityLocationLookupParams{
		ID: uuid.New(), WorkspaceID: workspaceRecord.ID, FacilityID: decision.FacilityID,
		Input: input.Input, InputKind: string(kind), RequestHash: lookup.RequestHash,
		ResponseHash: lookup.ResponseHash, RequestID: optional(lookup.RequestID), SourceUrl: lookup.SourceURL,
		Disposition: lookup.Response.Disposition, Latitude: lookup.Response.Latitude, Longitude: lookup.Response.Longitude,
		ResolvedAddress: optional(lookup.Response.ResolvedAddress), State: optional(canonicalFacilityState(lookup.Response.State)),
		County: optional(lookup.Response.County), Confidence: lookup.Response.Confidence,
		MatchMethod: optional(lookup.Response.MatchMethod), Candidates: candidates,
		Reason: optional(lookup.Response.Reason), Hint: optional(lookup.Response.Hint), Evidence: lookup.Evidence,
		FetchedAt: pgTime(lookup.FetchedAt),
	})
	if err != nil {
		return FacilityLocation{}, fmt.Errorf("store facility location: %w", err)
	}
	return locationFromRecord(record), nil
}

func canonicalFacilityState(state string) string {
	state = strings.TrimSpace(state)
	if strings.EqualFold(state, "Michigan") || strings.EqualFold(state, "MI") {
		return "MI"
	}
	return state
}

func (s *Service) ConfirmLocation(ctx context.Context, workspaceKey, locationID string) (FacilityLocation, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return FacilityLocation{}, err
	}
	id, err := uuid.Parse(locationID)
	if err != nil {
		return FacilityLocation{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetFacilityLocationLookup(ctx, db.GetFacilityLocationLookupParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return FacilityLocation{}, ErrNotFound
	}
	if err != nil {
		return FacilityLocation{}, fmt.Errorf("load facility location: %w", err)
	}
	if record.Disposition != "resolved" || record.Latitude == nil || record.Longitude == nil {
		return FacilityLocation{}, fmt.Errorf("%w: enter a more specific facility address or coordinates", ErrConflict)
	}
	if record.State == nil || !strings.EqualFold(strings.TrimSpace(*record.State), "MI") {
		return FacilityLocation{}, fmt.Errorf("%w: the Michigan response workflow requires a confirmed Michigan facility location", ErrConflict)
	}
	record, err = db.New(s.pool).ConfirmFacilityLocationLookup(ctx, db.ConfirmFacilityLocationLookupParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return FacilityLocation{}, ErrConflict
	}
	if err != nil {
		return FacilityLocation{}, fmt.Errorf("confirm facility location: %w", err)
	}
	return locationFromRecord(record), nil
}

func (s *Service) LatestLocation(ctx context.Context, workspaceKey, decisionID string) (FacilityLocation, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return FacilityLocation{}, err
	}
	decisionUUID, err := uuid.Parse(decisionID)
	if err != nil {
		return FacilityLocation{}, ErrNotFound
	}
	queries := db.New(s.pool)
	decision, err := queries.GetResponseDecisionContext(ctx, db.GetResponseDecisionContextParams{
		ID: decisionUUID, WorkspaceID: workspaceRecord.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return FacilityLocation{}, ErrNotFound
	}
	if err != nil {
		return FacilityLocation{}, fmt.Errorf("load response decision: %w", err)
	}
	record, err := queries.GetLatestFacilityLocationLookup(ctx, db.GetLatestFacilityLocationLookupParams{
		FacilityID: decision.FacilityID, WorkspaceID: workspaceRecord.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return FacilityLocation{}, ErrNotFound
	}
	if err != nil {
		return FacilityLocation{}, fmt.Errorf("load latest facility location: %w", err)
	}
	return locationFromRecord(record), nil
}

func (s *Service) Start(ctx context.Context, workspaceKey, decisionID string, input StartInput) (ResponseRun, bool, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return ResponseRun{}, false, err
	}
	decisionUUID, err := uuid.Parse(decisionID)
	if err != nil {
		return ResponseRun{}, false, ErrNotFound
	}
	locationUUID, err := uuid.Parse(input.FacilityLocationID)
	if err != nil {
		return ResponseRun{}, false, fmt.Errorf("%w: facility location ID is invalid", ErrInvalid)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return ResponseRun{}, false, fmt.Errorf("begin response run: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	decision, err := queries.GetResponseDecisionContext(ctx, db.GetResponseDecisionContextParams{ID: decisionUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ResponseRun{}, false, ErrNotFound
	}
	if err != nil {
		return ResponseRun{}, false, fmt.Errorf("load response decision: %w", err)
	}
	if decision.Tier != "ELEVATED" && decision.Tier != "PROHIBITED" {
		return ResponseRun{}, false, fmt.Errorf("%w: this response is only required for elevated or prohibited batches", ErrConflict)
	}
	location, err := queries.GetFacilityLocationLookup(ctx, db.GetFacilityLocationLookupParams{ID: locationUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) || location.FacilityID != decision.FacilityID {
		return ResponseRun{}, false, ErrNotFound
	}
	if err != nil {
		return ResponseRun{}, false, fmt.Errorf("load confirmed facility location: %w", err)
	}
	if !location.ConfirmedAt.Valid || location.Latitude == nil || location.Longitude == nil {
		return ResponseRun{}, false, fmt.Errorf("%w: confirm the treatment plant location first", ErrConflict)
	}
	active, err := queries.GetActiveResponseRun(ctx, db.GetActiveResponseRunParams{DecisionID: decisionUUID, WorkspaceID: workspaceRecord.ID})
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return ResponseRun{}, false, fmt.Errorf("finish active response lookup: %w", err)
		}
		run, getErr := s.Get(ctx, workspaceKey, active.ID.String())
		return run, false, getErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ResponseRun{}, false, fmt.Errorf("load active response: %w", err)
	}
	inputHash := digest([]byte(strings.Join([]string{configVersion, decision.DecisionInputHash, location.ID.String(), fmt.Sprint(*location.Latitude), fmt.Sprint(*location.Longitude)}, "|")))
	runID := uuid.New()
	record, err := queries.CreateResponseRun(ctx, db.CreateResponseRunParams{
		ID: runID, WorkspaceID: workspaceRecord.ID, DecisionID: decisionUUID,
		FacilityLocationID: locationUUID, Tier: decision.Tier, InputHash: inputHash,
		PolicySourceUrl: decision.PolicySourceUrl, PolicyVersion: decision.PolicyVersion,
	})
	created := true
	if errors.Is(err, pgx.ErrNoRows) {
		created = false
		record, err = queries.GetResponseRunByInput(ctx, db.GetResponseRunByInputParams{DecisionID: decisionUUID, InputHash: inputHash})
	}
	if err != nil {
		return ResponseRun{}, false, fmt.Errorf("store response run: %w", err)
	}
	if created {
		if _, err := s.jobs.InsertTx(ctx, tx, BuildArgs{RunID: runID.String()}, nil); err != nil {
			return ResponseRun{}, false, fmt.Errorf("queue response run: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ResponseRun{}, false, fmt.Errorf("commit response run: %w", err)
	}
	run, err := s.Get(ctx, workspaceKey, record.ID.String())
	return run, created, err
}

func (s *Service) Latest(ctx context.Context, workspaceKey, decisionID string) (ResponseRun, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return ResponseRun{}, err
	}
	id, err := uuid.Parse(decisionID)
	if err != nil {
		return ResponseRun{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetLatestResponseRun(ctx, db.GetLatestResponseRunParams{DecisionID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ResponseRun{}, ErrNotFound
	}
	if err != nil {
		return ResponseRun{}, fmt.Errorf("load latest response run: %w", err)
	}
	return s.build(ctx, runRecordFromLatest(record))
}

func (s *Service) Get(ctx context.Context, workspaceKey, runID string) (ResponseRun, error) {
	workspaceRecord, err := s.loadWorkspace(ctx, workspaceKey)
	if err != nil {
		return ResponseRun{}, err
	}
	id, err := uuid.Parse(runID)
	if err != nil {
		return ResponseRun{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetResponseRun(ctx, db.GetResponseRunParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ResponseRun{}, ErrNotFound
	}
	if err != nil {
		return ResponseRun{}, fmt.Errorf("load response run: %w", err)
	}
	return s.build(ctx, runRecordFromGet(record))
}

func (s *Service) Process(ctx context.Context, runID string) error {
	id, err := uuid.Parse(runID)
	if err != nil {
		return ErrNotFound
	}
	queries := db.New(s.pool)
	work, err := queries.GetResponseRunForWork(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load response work: %w", err)
	}
	if work.Status == "READY" || work.Status == "REVIEW_REQUIRED" || work.Status == "FAILED" {
		return nil
	}
	if work.Latitude == nil || work.Longitude == nil {
		return errors.New("confirmed response location omitted coordinates")
	}
	if err := queries.MarkResponseRunRunning(ctx, id); err != nil {
		return fmt.Errorf("mark response running: %w", err)
	}
	if err := queries.ClearResponseRunOutputs(ctx, id); err != nil {
		return fmt.Errorf("clear incomplete response: %w", err)
	}
	if err := s.persistTasks(ctx, queries, id, work.Tier); err != nil {
		return err
	}
	criticalGap := false
	if ok, err := s.collectSewerContext(ctx, queries, id, *work.Latitude, *work.Longitude); err != nil {
		return err
	} else if !ok {
		criticalGap = true
	}
	if ok, err := s.collectInvestigationLeads(ctx, queries, id, work.FacilityName, *work.Latitude, *work.Longitude); err != nil {
		return err
	} else if !ok {
		criticalGap = true
	}
	if work.Tier == "PROHIBITED" {
		if ok, err := s.collectAlternatives(ctx, queries, id, *work.Latitude, *work.Longitude); err != nil {
			return err
		} else if !ok {
			criticalGap = true
		}
	}
	status := "READY"
	if criticalGap {
		status = "REVIEW_REQUIRED"
	}
	if err := queries.CompleteResponseRun(ctx, db.CompleteResponseRunParams{ID: id, Status: status}); err != nil {
		return fmt.Errorf("complete response run: %w", err)
	}
	return nil
}

func (s *Service) RecordFailure(ctx context.Context, runID, code, detail string) error {
	id, err := uuid.Parse(runID)
	if err != nil {
		return ErrNotFound
	}
	return db.New(s.pool).FailResponseRun(ctx, db.FailResponseRunParams{ID: id, FailureCode: &code, FailureDetail: &detail})
}

func (s *Service) persistTasks(ctx context.Context, queries *db.Queries, runID uuid.UUID, tier string) error {
	for index, task := range taskDefinitions(tier) {
		if err := queries.CreateResponseTask(ctx, db.CreateResponseTaskParams{
			RunID: runID, Position: int32(index + 1), Code: task.Code, Category: task.Category,
			Title: task.Title, Detail: task.Detail, Timing: task.Timing, State: task.State,
		}); err != nil {
			return fmt.Errorf("store response task: %w", err)
		}
	}
	return nil
}

func (s *Service) collectSewerContext(ctx context.Context, queries *db.Queries, runID uuid.UUID, latitude, longitude float64) (bool, error) {
	fields := []string{
		"within_sewer_service_area", "sewer_service_area_provider", "sewer_service_area_provenance",
		"nearest_sewer_service_area_distance_m", "nearest_wastewater_plant_distance_m",
		"nearest_wastewater_plant_name", "nearest_wastewater_plant_population_served",
	}
	result, err := s.mireye.FetchBatch(ctx, mireye.FetchBatchRequest{
		Locations: []mireye.Coordinate{{Latitude: latitude, Longitude: longitude}}, Fields: fields,
	})
	if err != nil {
		return false, s.addGap(ctx, queries, runID, "SEWER_CONTEXT_UNAVAILABLE",
			"Mireye wastewater and sewershed context could not be retrieved.",
			"Retry the response, then verify the serving POTW and collection system from utility records.", true)
	}
	if len(result.Results) != 1 || !result.Results[0].OK {
		return false, s.addGap(ctx, queries, runID, "SEWER_CONTEXT_UNAVAILABLE",
			"Mireye did not return usable wastewater context for the confirmed plant location.",
			"Verify the serving POTW and collection system from utility records.", true)
	}
	data, err := json.Marshal(result.Results[0].Fields)
	if err != nil {
		return false, fmt.Errorf("encode sewer context: %w", err)
	}
	status := "AVAILABLE"
	if len(result.Results[0].PartialFailures) > 0 {
		status = "PARTIAL"
	}
	if err := queries.CreateResponseEvidence(ctx, db.CreateResponseEvidenceParams{
		ID: uuid.New(), RunID: runID, Provider: "MIREYE", Kind: "SEWER_CONTEXT", Status: status,
		Title: "Wastewater service context", Summary: "Screening context for the confirmed treatment plant location.",
		Data: data, SourceUrl: result.SourceURL, RequestHash: optional(result.RequestHash),
		ResponseHash: optional(result.ResponseHash), RequestID: optional(result.RequestID), FetchedAt: pgTime(result.FetchedAt),
		Caveat: "Sewershed polygons and nearest-plant fields do not prove that any industrial facility is connected to this collection system.",
	}); err != nil {
		return false, fmt.Errorf("store sewer context: %w", err)
	}
	if status == "PARTIAL" {
		return false, s.addGap(ctx, queries, runID, "SEWER_CONTEXT_PARTIAL",
			"One or more Mireye wastewater context fields were unavailable.",
			"Review the field-level failures and verify the collection system from utility records.", true)
	}
	return true, nil
}

func (s *Service) collectInvestigationLeads(ctx context.Context, queries *db.Queries, runID uuid.UUID, treatmentPlantName string, latitude, longitude float64) (bool, error) {
	result, err := s.echo.FetchECHOPotentialSectors(ctx, latitude, longitude)
	if err != nil {
		return false, s.addGap(ctx, queries, runID, "ECHO_SCREEN_UNAVAILABLE",
			"EPA ECHO could not be queried for nearby potential PFAS-handling sectors.",
			"Retry the response and review the utility's industrial user inventory.", true)
	}
	result.Facilities = excludeTreatmentPlant(result.Facilities, treatmentPlantName)
	sort.Slice(result.Facilities, func(i, j int) bool {
		if result.Facilities[i].Name != result.Facilities[j].Name {
			return result.Facilities[i].Name < result.Facilities[j].Name
		}
		return result.Facilities[i].RegistryID < result.Facilities[j].RegistryID
	})
	if len(result.Facilities) > 10 {
		result.Facilities = result.Facilities[:10]
	}
	data, err := json.Marshal(result)
	if err != nil {
		return false, fmt.Errorf("encode ECHO investigation screen: %w", err)
	}
	if err := queries.CreateResponseEvidence(ctx, db.CreateResponseEvidenceParams{
		ID: uuid.New(), RunID: runID, Provider: "EPA_ECHO", Kind: "POTENTIAL_PFAS_SECTORS", Status: "AVAILABLE",
		Title: "Nearby regulated-facility screen", Summary: fmt.Sprintf("%d nearby ECHO facilities matched an EPA potential PFAS-handling sector code.", len(result.Facilities)),
		Data: data, SourceUrl: echoSourceURL, FetchedAt: pgTime(result.FetchedAt),
		Caveat: "This is a geographic investigation screen. A sector match does not prove PFAS use, discharge, sewer connection, or causation.",
	}); err != nil {
		return false, fmt.Errorf("store ECHO investigation screen: %w", err)
	}
	for index, facility := range result.Facilities {
		codes, err := json.Marshal(facility.NAICS)
		if err != nil {
			return false, fmt.Errorf("encode investigation lead codes: %w", err)
		}
		if err := queries.CreateInvestigationLead(ctx, db.CreateInvestigationLeadParams{
			RunID: runID, Position: int32(index + 1), RegistryID: facility.RegistryID,
			FacilityName: facility.Name, City: optional(facility.City), State: optional(facility.State),
			NaicsCodes: codes, EvidenceTier: 3, EvidenceLabel: "Potential PFAS-handling sector",
			Rationale: "EPA ECHO lists this regulated facility within five miles of the treatment plant under a potential PFAS-handling industry code.",
			Caveat:    "Verify current operations and sewer connectivity before requesting records or planning sampling.",
			SourceUrl: echoReportURL + facility.RegistryID,
		}); err != nil {
			return false, fmt.Errorf("store investigation lead: %w", err)
		}
	}
	return true, nil
}

func excludeTreatmentPlant(facilities []evidence.ECHOFacility, treatmentPlantName string) []evidence.ECHOFacility {
	plantName := canonicalFacilityName(treatmentPlantName)
	result := make([]evidence.ECHOFacility, 0, len(facilities))
	for _, facility := range facilities {
		if canonicalFacilityName(facility.Name) != plantName {
			result = append(result, facility)
		}
	}
	return result
}

func canonicalFacilityName(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "WASTEWATER TREATMENT PLANT", "WWTP")
	value = strings.ReplaceAll(value, "WATER RESOURCE RECOVERY FACILITY", "WRRF")
	return strings.Join(strings.FieldsFunc(value, func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	}), " ")
}

func (s *Service) collectAlternatives(ctx context.Context, queries *db.Queries, runID uuid.UUID, latitude, longitude float64) (bool, error) {
	result, err := s.landfills.FetchActiveTypeII(ctx, latitude, longitude, 5)
	if err != nil {
		return false, s.addGap(ctx, queries, runID, "LANDFILL_INVENTORY_UNAVAILABLE",
			"Michigan's active Type II landfill inventory could not be retrieved.",
			"Retry before preparing alternative-management inquiries.", true)
	}
	if len(result.Facilities) == 0 {
		return false, s.addGap(ctx, queries, runID, "NO_VERIFIED_LANDFILL_CANDIDATES",
			"No active, accepting Michigan Type II landfill with complete coordinates was found.",
			"Confirm alternative treatment or disposal options directly with EGLE.", true)
	}
	datasetData, err := json.Marshal(result.Facilities)
	if err != nil {
		return false, fmt.Errorf("encode landfill inventory: %w", err)
	}
	if err := queries.CreateResponseEvidence(ctx, db.CreateResponseEvidenceParams{
		ID: uuid.New(), RunID: runID, Provider: "MICHIGAN_EGLE", Kind: "ACTIVE_TYPE_II_LANDFILLS", Status: "AVAILABLE",
		Title: "Active Michigan Type II landfill shortlist", Summary: "The five nearest active, accepting Type II facilities were shortlisted for route comparison.",
		Data: datasetData, SourceUrl: result.SourceURL, SourceVintage: optional(result.SourceVintage),
		ResponseHash: optional(digest(result.Raw)), FetchedAt: pgTime(result.FetchedAt),
		Caveat: "Active and accepting describes the facility's Part 115 status. It does not confirm acceptance of this biosolids batch or PFAS-impacted material.",
	}); err != nil {
		return false, fmt.Errorf("store landfill inventory: %w", err)
	}
	destinations := make([]string, len(result.Facilities))
	for index, facility := range result.Facilities {
		destinations[index] = coordinate(facility.Latitude, facility.Longitude)
	}
	routes, routeErr := s.mireye.Distance(ctx, mireye.DistanceRequest{
		Origins: []string{coordinate(latitude, longitude)}, Destinations: destinations, MaxCredits: len(destinations) * 12,
	})
	if routeErr != nil {
		if err := s.addGap(ctx, queries, runID, "DRIVING_ROUTES_UNAVAILABLE",
			"Mireye could not calculate driving routes to the verified landfill shortlist.",
			"Retry routing before comparing alternative-management inquiries.", true); err != nil {
			return false, err
		}
	} else {
		if err := queries.CreateResponseEvidence(ctx, db.CreateResponseEvidenceParams{
			ID: uuid.New(), RunID: runID, Provider: "MIREYE", Kind: "ALTERNATIVE_ROUTES", Status: "AVAILABLE",
			Title: "Typical driving routes", Summary: "Mireye compared the confirmed treatment plant coordinate with the verified landfill shortlist.",
			Data: routes.Raw, SourceUrl: routes.SourceURL, RequestHash: optional(routes.RequestHash), ResponseHash: optional(routes.ResponseHash),
			RequestID: optional(routes.RequestID), FetchedAt: pgTime(routes.FetchedAt),
			Caveat: "Durations reflect typical traffic, not real-time conditions. Unreachable, snapped, and dropped destinations are not treated as usable routes.",
		}); err != nil {
			return false, fmt.Errorf("store alternative routes: %w", err)
		}
	}
	candidates := routeCandidates(result.Facilities, routes, routeErr)
	for index, candidate := range candidates {
		if err := queries.CreateAlternativeCandidate(ctx, db.CreateAlternativeCandidateParams{
			RunID: runID, Position: int32(index + 1), WdsID: candidate.WDSID,
			FacilityName: candidate.Name, FacilityType: candidate.FacilityType,
			Address: candidate.Address, City: candidate.City, County: candidate.County,
			Latitude: candidate.Latitude, Longitude: candidate.Longitude,
			DisposalAreaStatus: candidate.DisposalAreaStatus, StraightlineDistanceKm: candidate.StraightlineKM,
			RouteStatus: candidate.RouteStatus, DrivingDistanceKm: candidate.DrivingDistanceKM,
			DurationMinutes: candidate.DurationMinutes, RouteNote: optional(candidate.RouteNote), SourceUrl: candidate.SourceURL,
		}); err != nil {
			return false, fmt.Errorf("store alternative candidate: %w", err)
		}
	}
	return routeErr == nil, nil
}

type routedLandfill struct {
	Landfill
	RouteStatus       string
	DrivingDistanceKM *float64
	DurationMinutes   *float64
	RouteNote         string
}

func routeCandidates(facilities []Landfill, routes mireye.DistanceResult, routeErr error) []routedLandfill {
	result := make([]routedLandfill, len(facilities))
	legs := make(map[int]mireye.DistanceLeg, len(routes.Legs))
	for _, leg := range routes.Legs {
		legs[leg.DestinationIndex] = leg
	}
	for index, facility := range facilities {
		candidate := routedLandfill{Landfill: facility, RouteStatus: "NOT_ROUTED"}
		if routeErr == nil {
			if index < len(routes.ResolvedDestinations) && routes.ResolvedDestinations[index].Error != nil {
				candidate.RouteStatus = "DROPPED"
				candidate.RouteNote = *routes.ResolvedDestinations[index].Error
			} else if leg, ok := legs[index]; ok {
				if leg.Flag != nil && *leg.Flag == "unreachable_or_snapped" {
					candidate.RouteStatus = "UNREACHABLE"
					candidate.RouteNote = "Mireye found no usable road route."
				} else if leg.DurationMinutes != nil {
					candidate.RouteStatus = "ROUTED"
					candidate.DrivingDistanceKM = &leg.DistanceKM
					candidate.DurationMinutes = leg.DurationMinutes
				} else {
					candidate.RouteStatus = "DROPPED"
					candidate.RouteNote = "Mireye returned no usable duration."
				}
			} else {
				candidate.RouteStatus = "DROPPED"
				candidate.RouteNote = "Mireye omitted this route."
			}
		}
		result[index] = candidate
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.RouteStatus == "ROUTED" && right.RouteStatus == "ROUTED" {
			return *left.DurationMinutes < *right.DurationMinutes
		}
		if left.RouteStatus != right.RouteStatus {
			return left.RouteStatus == "ROUTED"
		}
		return left.StraightlineKM < right.StraightlineKM
	})
	return result
}

func (s *Service) addGap(ctx context.Context, queries *db.Queries, runID uuid.UUID, code, detail, resolution string, critical bool) error {
	if err := queries.CreateResponseDataGap(ctx, db.CreateResponseDataGapParams{
		RunID: runID, Code: code, Detail: detail, Resolution: resolution, Critical: critical,
	}); err != nil {
		return fmt.Errorf("store response data gap: %w", err)
	}
	return nil
}

type runRecord struct {
	ID                 uuid.UUID
	DecisionID         uuid.UUID
	FacilityLocationID uuid.UUID
	FacilityName       string
	BatchIdentifier    string
	Tier               string
	Status             string
	PolicySourceURL    string
	PolicyVersion      string
	ResolvedAddress    *string
	Latitude           *float64
	Longitude          *float64
	LocationConfidence *float64
	LocationSourceURL  string
	LocationFetchedAt  pgtype.Timestamptz
	FailureCode        *string
	FailureDetail      *string
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

func runRecordFromGet(record db.GetResponseRunRow) runRecord {
	return runRecord{
		ID: record.ID, DecisionID: record.DecisionID, FacilityLocationID: record.FacilityLocationID,
		FacilityName: record.FacilityName, BatchIdentifier: record.BatchIdentifier,
		Tier: record.Tier, Status: record.Status, PolicySourceURL: record.PolicySourceUrl,
		PolicyVersion: record.PolicyVersion, ResolvedAddress: record.ResolvedAddress,
		Latitude: record.Latitude, Longitude: record.Longitude, LocationConfidence: record.LocationConfidence,
		LocationSourceURL: record.LocationSourceUrl, LocationFetchedAt: record.LocationFetchedAt,
		FailureCode: record.FailureCode, FailureDetail: record.FailureDetail,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func runRecordFromLatest(record db.GetLatestResponseRunRow) runRecord {
	return runRecord{
		ID: record.ID, DecisionID: record.DecisionID, FacilityLocationID: record.FacilityLocationID,
		FacilityName: record.FacilityName, BatchIdentifier: record.BatchIdentifier,
		Tier: record.Tier, Status: record.Status, PolicySourceURL: record.PolicySourceUrl,
		PolicyVersion: record.PolicyVersion, ResolvedAddress: record.ResolvedAddress,
		Latitude: record.Latitude, Longitude: record.Longitude, LocationConfidence: record.LocationConfidence,
		LocationSourceURL: record.LocationSourceUrl, LocationFetchedAt: record.LocationFetchedAt,
		FailureCode: record.FailureCode, FailureDetail: record.FailureDetail,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func (s *Service) build(ctx context.Context, record runRecord) (ResponseRun, error) {
	queries := db.New(s.pool)
	tasks, err := queries.ListResponseTasks(ctx, record.ID)
	if err != nil {
		return ResponseRun{}, fmt.Errorf("load response tasks: %w", err)
	}
	evidenceRows, err := queries.ListResponseEvidence(ctx, record.ID)
	if err != nil {
		return ResponseRun{}, fmt.Errorf("load response evidence: %w", err)
	}
	leads, err := queries.ListInvestigationLeads(ctx, record.ID)
	if err != nil {
		return ResponseRun{}, fmt.Errorf("load investigation leads: %w", err)
	}
	alternatives, err := queries.ListAlternativeCandidates(ctx, record.ID)
	if err != nil {
		return ResponseRun{}, fmt.Errorf("load alternative candidates: %w", err)
	}
	gaps, err := queries.ListResponseDataGaps(ctx, record.ID)
	if err != nil {
		return ResponseRun{}, fmt.Errorf("load response data gaps: %w", err)
	}

	result := ResponseRun{
		ID: record.ID.String(), DecisionID: record.DecisionID.String(), FacilityLocationID: record.FacilityLocationID.String(),
		FacilityName: record.FacilityName, BatchIdentifier: record.BatchIdentifier,
		Tier: record.Tier, Status: record.Status, PolicySourceURL: record.PolicySourceURL,
		PolicyVersion: record.PolicyVersion, Tasks: make([]ResponseTask, 0, len(tasks)), Evidence: make([]ResponseEvidence, 0, len(evidenceRows)),
		InvestigationLeads: make([]InvestigationLead, 0, len(leads)), Alternatives: make([]AlternativeCandidate, 0, len(alternatives)),
		DataGaps: make([]ResponseDataGap, 0, len(gaps)), CreatedAt: record.CreatedAt.Time, UpdatedAt: record.UpdatedAt.Time,
		Location: FacilityLocation{
			ID: record.FacilityLocationID.String(), Latitude: record.Latitude, Longitude: record.Longitude,
			Confidence: record.LocationConfidence, SourceURL: record.LocationSourceURL,
			FetchedAt: record.LocationFetchedAt.Time, Confirmed: true, Candidates: []LocationCandidate{},
		},
	}
	if record.ResolvedAddress != nil {
		result.Location.ResolvedAddress = *record.ResolvedAddress
	}
	if record.FailureCode != nil {
		result.FailureCode = *record.FailureCode
	}
	if record.FailureDetail != nil {
		result.FailureDetail = *record.FailureDetail
	}
	for _, row := range tasks {
		result.Tasks = append(result.Tasks, ResponseTask{Position: int(row.Position), Code: row.Code, Category: row.Category, Title: row.Title, Detail: row.Detail, Timing: row.Timing, State: row.State})
	}
	for _, row := range evidenceRows {
		item := ResponseEvidence{Provider: row.Provider, Kind: row.Kind, Status: row.Status, Title: row.Title, Summary: row.Summary, Data: append(json.RawMessage(nil), row.Data...), SourceURL: row.SourceUrl, FetchedAt: row.FetchedAt.Time, Caveat: row.Caveat}
		if row.SourceVintage != nil {
			item.SourceVintage = *row.SourceVintage
		}
		result.Evidence = append(result.Evidence, item)
	}
	for _, row := range leads {
		var codes []string
		_ = json.Unmarshal(row.NaicsCodes, &codes)
		item := InvestigationLead{Position: int(row.Position), RegistryID: row.RegistryID, FacilityName: row.FacilityName, NAICSCodes: codes, EvidenceTier: int(row.EvidenceTier), EvidenceLabel: row.EvidenceLabel, Rationale: row.Rationale, Caveat: row.Caveat, SourceURL: row.SourceUrl}
		if row.City != nil {
			item.City = *row.City
		}
		if row.State != nil {
			item.State = *row.State
		}
		result.InvestigationLeads = append(result.InvestigationLeads, item)
	}
	for _, row := range alternatives {
		item := AlternativeCandidate{Position: int(row.Position), WDSID: row.WdsID, FacilityName: row.FacilityName, FacilityType: row.FacilityType, Address: row.Address, City: row.City, County: row.County, Latitude: row.Latitude, Longitude: row.Longitude, DisposalAreaStatus: row.DisposalAreaStatus, StraightlineDistanceKM: row.StraightlineDistanceKm, RouteStatus: row.RouteStatus, DrivingDistanceKM: row.DrivingDistanceKm, DurationMinutes: row.DurationMinutes, AcceptanceStatus: row.AcceptanceStatus, Executable: row.Executable, SourceURL: row.SourceUrl}
		if row.RouteNote != nil {
			item.RouteNote = *row.RouteNote
		}
		result.Alternatives = append(result.Alternatives, item)
	}
	for _, row := range gaps {
		result.DataGaps = append(result.DataGaps, ResponseDataGap{Code: row.Code, Detail: row.Detail, Resolution: row.Resolution, Critical: row.Critical})
	}
	return result, nil
}

func taskDefinitions(tier string) []ResponseTask {
	common := []ResponseTask{
		{Code: "SOURCE_EFFLUENT_SAMPLE", Category: "SAMPLING", Title: "Collect a source-effluent sample", Detail: "Collect and analyze a representative source-effluent sample using an accepted PFAS method.", Timing: "Within 30 days", State: "REQUIRED"},
		{Code: "VERIFY_COLLECTION_SYSTEM", Category: "INVESTIGATION", Title: "Verify collection-system connections", Detail: "Compare the industrial user inventory and sewer map with the geographic leads before treating any facility as connected.", Timing: "Before targeted sampling", State: "DRAFT"},
		{Code: "REVIEW_UPSTREAM_RECORDS", Category: "INVESTIGATION", Title: "Review upstream records", Detail: "Check current operations, permits, chemical inventories, and prior sampling for verified connected users.", Timing: "During source investigation", State: "DRAFT"},
		{Code: "PLAN_TARGETED_SAMPLING", Category: "INVESTIGATION", Title: "Plan targeted upstream sampling", Detail: "Prioritize sampling only after connectivity and current PFAS-handling activity are verified.", Timing: "After records review", State: "DRAFT"},
	}
	if tier == "PROHIBITED" {
		return append([]ResponseTask{
			{Code: "BLOCK_LAND_APPLICATION", Category: "CONTROL", Title: "Keep this batch out of land application", Detail: "The control plane has blocked land-application allocation for this batch.", Timing: "Effective immediately", State: "ENFORCED"},
			{Code: "NOTIFY_EGLE", Category: "REGULATORY", Title: "Notify EGLE through MiEnviro", Detail: "Prepare the industrially impacted biosolids notification with the confirmed laboratory evidence.", Timing: "Promptly", State: "REQUIRED"},
			{Code: "ARRANGE_ALTERNATIVE_MANAGEMENT", Category: "ALTERNATIVE_MANAGEMENT", Title: "Request acceptance and quotes", Detail: "Contact verified candidate facilities to confirm material acceptance, analytical requirements, capacity, price, and scheduling.", Timing: "Before transport", State: "DRAFT"},
		}, common...)
	}
	return append([]ResponseTask{{Code: "ENFORCE_RATE_CAP", Category: "CONTROL", Title: "Keep the PFAS rate ceiling in the placement plan", Detail: "The placement calculator limits the batch to 1.5 dry tons per acre unless EGLE approves an alternative strategy.", Timing: "Before land application", State: "ENFORCED"}}, common...)
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

func locationFromRecord(record db.PfasFacilityLocationLookup) FacilityLocation {
	var candidates []mireye.LookupCandidate
	_ = json.Unmarshal(record.Candidates, &candidates)
	result := FacilityLocation{
		ID: record.ID.String(), FacilityID: record.FacilityID.String(), Input: record.Input,
		Kind: record.InputKind, Disposition: record.Disposition, Latitude: record.Latitude,
		Longitude: record.Longitude, Confidence: record.Confidence, SourceURL: record.SourceUrl,
		FetchedAt: record.FetchedAt.Time, Confirmed: record.ConfirmedAt.Valid,
		Candidates: make([]LocationCandidate, 0, len(candidates)),
	}
	if record.ResolvedAddress != nil {
		result.ResolvedAddress = *record.ResolvedAddress
	}
	if record.State != nil {
		result.State = *record.State
	}
	if record.County != nil {
		result.County = *record.County
	}
	if record.Reason != nil {
		result.Reason = *record.Reason
	}
	if record.Hint != nil {
		result.Hint = *record.Hint
	}
	for _, candidate := range candidates {
		result.Candidates = append(result.Candidates, LocationCandidate{
			Label: candidate.Label, ResolvedAddress: candidate.ResolvedAddress,
			Latitude: candidate.Latitude, Longitude: candidate.Longitude, Confidence: candidate.Confidence,
		})
	}
	return result
}

func optional(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func pgTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func coordinate(latitude, longitude float64) string {
	return fmt.Sprintf("%.7f,%.7f", latitude, longitude)
}
