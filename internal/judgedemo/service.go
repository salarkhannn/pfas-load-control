package judgedemo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/decisionpackage"
	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/lab"
	"github.com/salarkhannn/pfas-load-control/internal/mireye"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
	"github.com/salarkhannn/pfas-load-control/internal/policy"
)

const (
	caseID         = "MI-2026-014"
	fixtureVersion = "judge-demo-v2"
	batchDryTons   = "52"
	wetMassKg      = "235868.0324"
	percentSolids  = "20"
	policySource   = "https://www.michigan.gov/egle/about/organization/water-resources/biosolids/pfas-related/interim-strategy"
	modeUnresolved = "UNRESOLVED"
	modeReviewed   = "REVIEWED_EVIDENCE"
)

//go:embed testdata/*.json
var fixtureFiles embed.FS

var (
	ErrNotFound  = errors.New("judge-demo run not found")
	ErrInvalid   = errors.New("invalid judge-demo request")
	ErrIntegrity = errors.New("judge-demo package integrity verification failed")
)

type Service struct {
	startMu                        sync.Mutex
	store                          runStore
	now                            func() time.Time
	labFixture                     []byte
	mireyeCaptureFixture           []byte
	mireyeRequestFixture           []byte
	mireyeResponseFixture          []byte
	operatorAdjustmentFixture      []byte
	resolutionCaptureFixture       []byte
	resolutionArtifactFixture      []byte
	resolutionAuthorizationFixture []byte
	parentBoundaryFixture          []byte
	revisedScreeningFixture        []byte
}

func NewService(pool *pgxpool.Pool) *Service {
	var store runStore = newMemoryStore()
	if pool != nil {
		store = postgresStore{pool: pool}
	}
	return newService(store)
}

func newService(store runStore) *Service {
	labFixture, _ := fixtureFiles.ReadFile("testdata/lab-report-v1.json")
	mireyeCaptureFixture, _ := fixtureFiles.ReadFile("testdata/mireye-fetch-batch-capture-v1.json")
	mireyeRequestFixture, _ := fixtureFiles.ReadFile("testdata/mireye-fetch-batch-request-v1.json")
	mireyeResponseFixture, _ := fixtureFiles.ReadFile("testdata/mireye-fetch-batch-response-v1.json")
	operatorAdjustmentFixture, _ := fixtureFiles.ReadFile("testdata/operator-boundary-adjustment-v1.json")
	resolutionCaptureFixture, _ := fixtureFiles.ReadFile("testdata/slope-resolution-capture-v1.json")
	resolutionArtifactFixture, _ := fixtureFiles.ReadFile("testdata/slope-resolution-boundary-v1.json")
	resolutionAuthorizationFixture, _ := fixtureFiles.ReadFile("testdata/slope-resolution-review-authorization-v1.json")
	parentBoundaryFixture, _ := fixtureFiles.ReadFile("testdata/confirmed-parent-boundary-v3.json")
	revisedScreeningFixture, _ := fixtureFiles.ReadFile("testdata/mireye-revised-boundary-screening-v1.json")
	return &Service{store: store, now: time.Now, labFixture: labFixture, mireyeCaptureFixture: mireyeCaptureFixture, mireyeRequestFixture: mireyeRequestFixture, mireyeResponseFixture: mireyeResponseFixture, operatorAdjustmentFixture: operatorAdjustmentFixture, resolutionCaptureFixture: resolutionCaptureFixture, resolutionArtifactFixture: resolutionArtifactFixture, resolutionAuthorizationFixture: resolutionAuthorizationFixture, parentBoundaryFixture: parentBoundaryFixture, revisedScreeningFixture: revisedScreeningFixture}
}

func (s *Service) Start(ctx context.Context, idempotencyKey string) (DemoRun, error) {
	return s.start(ctx, idempotencyKey, modeUnresolved, "")
}

func (s *Service) StartReviewed(ctx context.Context, idempotencyKey, parentRunID string) (DemoRun, error) {
	parent, err := s.Get(ctx, parentRunID)
	if err != nil {
		return DemoRun{}, err
	}
	if parent.Mode != modeUnresolved || parent.CaseID != caseID || parent.RunStatus != "SUCCEEDED" || parent.CalculationStatus != string(placement.StatusReviewRequired) {
		return DemoRun{}, fmt.Errorf("%w: reviewed evidence requires the unresolved prepared run", ErrInvalid)
	}
	return s.start(ctx, idempotencyKey, modeReviewed, parent.ID)
}

func (s *Service) start(ctx context.Context, idempotencyKey, mode, parentRunID string) (DemoRun, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 200 {
		return DemoRun{}, fmt.Errorf("%w: idempotency key is required", ErrInvalid)
	}
	s.startMu.Lock()
	defer s.startMu.Unlock()
	releaseIdempotency, err := s.store.AcquireIdempotency(ctx, idempotencyKey)
	if err != nil {
		return DemoRun{}, err
	}
	defer releaseIdempotency()
	if existing, err := s.store.GetByIdempotencyKey(ctx, idempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return DemoRun{}, err
	}

	recorder := &toolRecorder{now: s.now}
	labInput := labToolInput{MediaType: "application/json", PercentSolids: percentSolids, Report: append(json.RawMessage(nil), s.labFixture...)}
	extraction, err := executeTool(recorder, "lab.report.extract", "#lab-input", "fixture_lab_report_v1", labInput, func() (lab.Extraction, error) {
		solids := percentSolids
		return lab.NewParser(nil).Parse(ctx, labInput.MediaType, labInput.Report, &solids)
	}, func(result lab.Extraction) string {
		return fmt.Sprintf("Parsed %d analytes from the frozen laboratory report, including PFOS and PFOA on page 4.", len(result.Draft.Analytes))
	})
	if err != nil {
		return DemoRun{}, fmt.Errorf("extract frozen laboratory report: %w", err)
	}

	classificationInput := policyInput(extraction)
	classification, err := executeTool(recorder, "policy.classify", policySource, "fixture_policy_2024_4", classificationInput, func() (policyResult, error) {
		packs, loadErr := policy.LoadCatalog()
		if loadErr != nil {
			return policyResult{}, loadErr
		}
		for _, pack := range packs {
			if pack.Code == "MI-PFAS-BIOSOLIDS" && pack.Version == "2024.4" {
				return policyResult{RulePack: pack, Evaluation: policy.Evaluate(pack, classificationInput)}, nil
			}
		}
		return policyResult{}, errors.New("reviewed rule pack 2024.4 for Michigan is unavailable")
	}, func(result policyResult) string {
		return fmt.Sprintf("Applied Michigan rule pack %s; classification: %s.", result.RulePack.Version, result.Evaluation.Tier)
	})
	if err != nil {
		return DemoRun{}, fmt.Errorf("classify frozen laboratory report: %w", err)
	}
	if classification.Evaluation.Tier != policy.TierStandard {
		return DemoRun{}, fmt.Errorf("prepared case unexpectedly classified as %s", classification.Evaluation.Tier)
	}

	mireyeInput := mireyeReplayInput{Capture: json.RawMessage(s.mireyeCaptureFixture), Request: json.RawMessage(s.mireyeRequestFixture), Response: json.RawMessage(s.mireyeResponseFixture)}
	mireyeResult, err := executeTool(recorder, "mireye.fetch.batch", "https://api.mireye.com/v1/fetch/batch", "req_088804c2fcb6", mireyeInput, func() (capturedMireyeResult, error) {
		return executeCapturedMireye(mireyeInput)
	}, func(result capturedMireyeResult) string {
		return "Replayed the captured Mireye batch response and found a sampled maximum slope above Michigan's default six-percent surface-application limit."
	})
	if err != nil {
		return DemoRun{}, fmt.Errorf("investigate frozen Mireye field: %w", err)
	}
	adjustment, err := executeTool(recorder, "field.boundary.adjust", "#operator-boundary-input", "fixture_operator_boundary_v1", json.RawMessage(s.operatorAdjustmentFixture), func() (DemoAcreageAdjustment, error) {
		return executeOperatorAdjustment(s.operatorAdjustmentFixture)
	}, func(result DemoAcreageAdjustment) string {
		return fmt.Sprintf("Applied the seeded operator boundary adjustment: %s recorded acres − %s excluded acres = %s effective acres.", result.RecordedBoundaryAcres, result.ExcludedAcres, result.EffectiveAcres)
	})
	if err != nil {
		return DemoRun{}, fmt.Errorf("apply operator acreage adjustment: %w", err)
	}

	evidenceAsOf := time.Date(2026, time.August, 20, 10, 5, 0, 0, time.UTC)
	beforeInput := placement.Input{Tier: string(classification.Evaluation.Tier), WetMassKg: wetMassKg, PercentSolids: percentSolids, EvidenceAsOf: evidenceAsOf, Fields: []placement.FieldInput{fieldA(adjustment.RecordedBoundaryAcres, preScreenFacts(), adjustment.BoundaryVersion, nil), fieldB()}}
	beforeResult, err := executeTool(recorder, "placement.evaluate", "#allocation", "engine_before_"+mireyeResult.Capture.RequestID, beforeInput, func() (placementResult, error) {
		plan, inputHash, evaluateErr := evaluate(ctx, beforeInput, nil)
		return placementResult{Plan: plan, InputHash: inputHash}, evaluateErr
	}, func(result placementResult) string {
		return fmt.Sprintf("Calculated the pre-screen plan with Field A capacity of %s dry tons.", capacity(result.Plan, "Riverbend East"))
	})
	if err != nil {
		return DemoRun{}, fmt.Errorf("evaluate pre-screen placement: %w", err)
	}

	var resolutionEvidence *DemoResolutionEvidence
	var resolutionStore placement.SlopeEvidenceStore
	var resolutionReference *placement.SlopeResolutionReference
	boundaryVersion := adjustment.BoundaryVersion
	if mode == modeReviewed {
		prepared, store, reference, prepareErr := s.prepareReviewedEvidence(mireyeResult)
		if prepareErr != nil {
			return DemoRun{}, fmt.Errorf("prepare reviewed slope evidence: %w", prepareErr)
		}
		verificationField := fieldA(adjustment.EffectiveAcres, placementFacts(mireyeResult.Facts), reference.BoundaryVersion, reference)
		_, screeningErr := executeTool(recorder, "mireye.revised-boundary.screen", "https://api.mireye.com/v1/fetch/batch", reference.RevisedScreeningEvidenceRecordID, reference, func() (placement.RevisedScreeningVerification, error) {
			return placement.VerifyRevisedSlopeScreening(ctx, verificationField, store, evidenceAsOf)
		}, func(result placement.RevisedScreeningVerification) string {
			return fmt.Sprintf("Received %d of %d bounded Mireye sample results for revised boundary v%d; maximum sampled slope %s° (%s%% grade).", result.ReturnedSampleCount, result.RequestedSampleCount, reference.BoundaryVersion, result.MaximumSlopeDegrees, result.MaximumSlopeGradePercent)
		})
		if screeningErr != nil {
			return DemoRun{}, fmt.Errorf("verify revised-boundary Mireye screening: %w", screeningErr)
		}
		verification, verifyErr := executeTool(recorder, "slope-resolution.verify", "#reviewed-evidence", reference.EvidenceRecordID, reference, func() (placement.ResolutionVerification, error) {
			return placement.VerifySlopeResolution(ctx, verificationField, store, evidenceAsOf)
		}, func(result placement.ResolutionVerification) string {
			return fmt.Sprintf("Verified parent boundary %s, revised boundary v%d, configured demonstration roles, and bounded sampled-terrain evidence.", result.ParentBoundaryEvidenceRecordID, result.BoundaryVersion)
		})
		if verifyErr != nil {
			return DemoRun{}, fmt.Errorf("verify reviewed slope evidence: %w", verifyErr)
		}
		prepared.Verification = verification
		resolutionEvidence, resolutionStore, resolutionReference = &prepared, store, reference
		boundaryVersion = reference.BoundaryVersion
	}

	afterInput := beforeInput
	afterInput.Fields = []placement.FieldInput{fieldA(adjustment.EffectiveAcres, placementFacts(mireyeResult.Facts), boundaryVersion, resolutionReference), fieldB()}
	afterResult, err := executeTool(recorder, "placement.evaluate", "#allocation", "engine_after_"+mireyeResult.Capture.RequestID, afterInput, func() (placementResult, error) {
		plan, inputHash, evaluateErr := evaluate(ctx, afterInput, resolutionStore)
		if evaluateErr == nil {
			evaluateErr = validatePlan(plan)
		}
		return placementResult{Plan: plan, InputHash: inputHash}, evaluateErr
	}, func(result placementResult) string {
		if mode == modeReviewed {
			return fmt.Sprintf("Recalculated the verified boundary and allocated %s dry tons to Field B plus %s dry tons to Field A.", allocation(result.Plan, "North Forty"), allocation(result.Plan, "Riverbend East"))
		}
		return fmt.Sprintf("Allocated %s dry tons to Field B, placed Field A into slope review, and left %s dry tons unallocated.", allocation(result.Plan, "North Forty"), result.Plan.UnallocatedDryTons)
	})
	if err != nil {
		return DemoRun{}, fmt.Errorf("evaluate post-screen placement: %w", err)
	}

	evidenceRecordedAt := s.now().UTC()
	citations := buildCitations(evidenceRecordedAt, extraction, classification, mireyeResult, adjustment, resolutionEvidence, afterResult.Plan)
	reviewQuestion := "Provide an immutable confirmed-boundary record that proves the high-slope sample falls outside Riverbend East before calculating an allocation. This demo does not accept acreage or free-form approval text as a resolution."
	if mode == modeReviewed {
		reviewQuestion = "The boundary evidence cleared the slope gate for calculation. A responsible person must still authorize any real application."
	}
	runID, packageID := uuid.NewString(), uuid.NewString()
	decisionCalls := append([]DemoToolCall(nil), recorder.calls...)
	packageInput := packagePayload{SchemaVersion: fixtureVersion, Mode: mode, ParentRunID: parentRunID, RunStatus: "SUCCEEDED", CalculationStatus: string(afterResult.Plan.Status), AuthorizationStatus: "REQUIRED", AuthorizationRequired: true, CaseID: caseID, Lab: extraction.Draft, Policy: classification, PhysicalEvidence: mireyeResult, AcreageAdjustment: adjustment, ResolutionEvidence: resolutionEvidence, Before: beforeResult.Plan, After: afterResult.Plan, Citations: citations, ReviewQuestion: reviewQuestion, ExecutedCalls: decisionCalls}
	freezeStarted := s.now().UTC()
	payloadArtifact, payloadHash, err := decisionpackage.FreezeRecord(packageInput)
	if err != nil {
		return DemoRun{}, fmt.Errorf("freeze decision payload: %w", err)
	}
	freezeCompleted := s.now().UTC()
	freezeReceipt := DemoFreezeReceipt{Position: len(decisionCalls) + 1, ToolName: "decisionpackage.freeze", Status: "SUCCEEDED", ArtifactID: packageID, ArtifactHash: payloadHash, StartedAt: freezeStarted, CompletedAt: freezeCompleted}
	envelope := packageEnvelope{SchemaVersion: fixtureVersion + "-envelope", DecisionPayload: payloadArtifact, FreezeReceipt: freezeReceipt}
	frozenArtifact, envelopeHash, err := decisionpackage.FreezeRecord(envelope)
	if err != nil {
		return DemoRun{}, fmt.Errorf("freeze package envelope: %w", err)
	}

	completed := s.now().UTC()
	reviewRequired := afterResult.Plan.Status == placement.StatusReviewRequired
	run := DemoRun{ID: runID, FixtureVersion: fixtureVersion, Mode: mode, ParentRunID: parentRunID, Kind: "LAND_APPLICATION_READINESS_DECISION", RunStatus: "SUCCEEDED", CalculationStatus: string(afterResult.Plan.Status), AuthorizationStatus: "REQUIRED", AuthorizationRequired: true, CaseID: caseID, BatchDryTons: batchDryTons, Before: beforeResult.Plan, After: afterResult.Plan, ExcludedAcres: adjustment.ExcludedAcres, ReviewRequired: reviewRequired, ReviewQuestion: reviewQuestion, PhysicalEvidence: mireyeResult.Facts, MireyeCapture: mireyeResult.Capture, AcreageAdjustment: adjustment, ResolutionEvidence: resolutionEvidence, ToolCalls: decisionCalls, FreezeReceipt: freezeReceipt, Citations: citations, Package: DemoPackage{ID: packageID, Status: "FROZEN", InputHash: afterResult.InputHash, DecisionHash: envelopeHash, PayloadHash: payloadHash, Artifact: frozenArtifact, DownloadURL: "/api/v1/judge-demo/runs/" + runID + "/package", CreatedAt: completed}, CreatedAt: recorder.startedAt(), CompletedAt: completed}
	stored, err := s.store.Save(ctx, idempotencyKey, run)
	if err != nil {
		return DemoRun{}, fmt.Errorf("persist judge-demo run: %w", err)
	}
	return stored, nil
}

func (s *Service) Get(ctx context.Context, id string) (DemoRun, error) { return s.store.Get(ctx, id) }

func (s *Service) PackageArtifact(ctx context.Context, id string) ([]byte, error) {
	run, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), run.Package.Artifact...), nil
}

type labToolInput struct {
	MediaType     string          `json:"mediaType"`
	PercentSolids string          `json:"percentSolids"`
	Report        json.RawMessage `json:"report"`
}

type policyResult struct {
	RulePack   policy.RulePack   `json:"rulePack"`
	Evaluation policy.Evaluation `json:"evaluation"`
}

type placementResult struct {
	Plan      placement.PlacementPlan `json:"plan"`
	InputHash string                  `json:"inputHash"`
}

type packagePayload struct {
	SchemaVersion         string                  `json:"schemaVersion"`
	Mode                  string                  `json:"mode"`
	ParentRunID           string                  `json:"parentRunId,omitempty"`
	RunStatus             string                  `json:"runStatus"`
	CalculationStatus     string                  `json:"calculationStatus"`
	AuthorizationStatus   string                  `json:"authorizationStatus"`
	AuthorizationRequired bool                    `json:"authorizationRequired"`
	CaseID                string                  `json:"caseId"`
	Lab                   lab.Draft               `json:"lab"`
	Policy                policyResult            `json:"policy"`
	PhysicalEvidence      capturedMireyeResult    `json:"physicalEvidence"`
	AcreageAdjustment     DemoAcreageAdjustment   `json:"acreageAdjustment"`
	ResolutionEvidence    *DemoResolutionEvidence `json:"resolutionEvidence,omitempty"`
	Before                placement.PlacementPlan `json:"before"`
	After                 placement.PlacementPlan `json:"after"`
	Citations             []DemoCitation          `json:"citations"`
	ReviewQuestion        string                  `json:"reviewQuestion"`
	ExecutedCalls         []DemoToolCall          `json:"executedCalls"`
}

type packageEnvelope struct {
	SchemaVersion   string            `json:"schemaVersion"`
	DecisionPayload json.RawMessage   `json:"decisionPayload"`
	FreezeReceipt   DemoFreezeReceipt `json:"freezeReceipt"`
}

type resolutionCapture struct {
	FixtureVersion                   string `json:"fixtureVersion"`
	ProgramPolicyVersion             string `json:"programPolicyVersion"`
	EvidenceRecordID                 string `json:"evidenceRecordId"`
	EvidenceType                     string `json:"evidenceType"`
	ArtifactHash                     string `json:"artifactHash"`
	BoundaryVersion                  int    `json:"boundaryVersion"`
	ParentBoundaryEvidenceRecordID   string `json:"parentBoundaryEvidenceRecordId"`
	ParentBoundaryArtifactHash       string `json:"parentBoundaryArtifactHash"`
	ParentBoundaryVersion            int    `json:"parentBoundaryVersion"`
	SourceEvidenceRecordID           string `json:"sourceEvidenceRecordId"`
	SourceArtifactHash               string `json:"sourceArtifactHash"`
	RevisedScreeningEvidenceRecordID string `json:"revisedScreeningEvidenceRecordId"`
	RevisedScreeningArtifactHash     string `json:"revisedScreeningArtifactHash"`
	ReviewerAuthorizationRecordID    string `json:"reviewerAuthorizationRecordId"`
	ReviewerAuthorizationHash        string `json:"reviewerAuthorizationHash"`
	Label                            string `json:"label"`
}

type demoEvidenceStore map[string]placement.EvidenceArtifact

func (s demoEvidenceStore) LoadSlopeEvidence(_ context.Context, id string) (placement.EvidenceArtifact, error) {
	record, ok := s[id]
	if !ok {
		return placement.EvidenceArtifact{}, errors.New("slope evidence record not found")
	}
	return record, nil
}

func (s *Service) prepareReviewedEvidence(captured capturedMireyeResult) (DemoResolutionEvidence, demoEvidenceStore, *placement.SlopeResolutionReference, error) {
	var capture resolutionCapture
	if err := json.Unmarshal(s.resolutionCaptureFixture, &capture); err != nil {
		return DemoResolutionEvidence{}, nil, nil, err
	}
	artifact, artifactHash, err := placement.CanonicalizeSlopeResolutionArtifact(s.resolutionArtifactFixture)
	if err != nil {
		return DemoResolutionEvidence{}, nil, nil, err
	}
	if artifactHash != capture.ArtifactHash {
		return DemoResolutionEvidence{}, nil, nil, fmt.Errorf("reviewed boundary artifact hash does not match its immutable capture record: got %s want %s", artifactHash, capture.ArtifactHash)
	}
	authorizationArtifact, authorizationHash, err := placement.CanonicalizeReviewerAuthorizationArtifact(s.resolutionAuthorizationFixture)
	if err != nil {
		return DemoResolutionEvidence{}, nil, nil, err
	}
	if authorizationHash != capture.ReviewerAuthorizationHash {
		return DemoResolutionEvidence{}, nil, nil, fmt.Errorf("reviewer authorization artifact hash does not match its immutable capture record: got %s want %s", authorizationHash, capture.ReviewerAuthorizationHash)
	}
	parentArtifact, parentHash, err := placement.CanonicalizeParentBoundaryArtifact(s.parentBoundaryFixture)
	if err != nil {
		return DemoResolutionEvidence{}, nil, nil, err
	}
	if parentHash != capture.ParentBoundaryArtifactHash {
		return DemoResolutionEvidence{}, nil, nil, errors.New("parent boundary artifact hash does not match its immutable capture record")
	}
	screeningArtifact, screeningHash, err := placement.CanonicalizeRevisedScreeningArtifact(s.revisedScreeningFixture)
	if err != nil {
		return DemoResolutionEvidence{}, nil, nil, err
	}
	if screeningHash != capture.RevisedScreeningArtifactHash {
		return DemoResolutionEvidence{}, nil, nil, fmt.Errorf("revised screening artifact hash does not match its immutable capture record: got %s want %s", screeningHash, capture.RevisedScreeningArtifactHash)
	}
	source := append([]byte(nil), captured.Capture.Response...)
	if hashBytes(source) != capture.SourceArtifactHash || captured.Capture.ResponseHash != capture.SourceArtifactHash {
		return DemoResolutionEvidence{}, nil, nil, errors.New("reviewed boundary source does not match the captured Mireye response")
	}
	store := demoEvidenceStore{
		capture.EvidenceRecordID:                 {ID: capture.EvidenceRecordID, ArtifactHash: capture.ArtifactHash, Artifact: artifact},
		capture.SourceEvidenceRecordID:           {ID: capture.SourceEvidenceRecordID, ArtifactHash: capture.SourceArtifactHash, Artifact: source},
		capture.ReviewerAuthorizationRecordID:    {ID: capture.ReviewerAuthorizationRecordID, ArtifactHash: capture.ReviewerAuthorizationHash, Artifact: authorizationArtifact},
		capture.ParentBoundaryEvidenceRecordID:   {ID: capture.ParentBoundaryEvidenceRecordID, ArtifactHash: capture.ParentBoundaryArtifactHash, Artifact: parentArtifact},
		capture.RevisedScreeningEvidenceRecordID: {ID: capture.RevisedScreeningEvidenceRecordID, ArtifactHash: capture.RevisedScreeningArtifactHash, Artifact: screeningArtifact},
	}
	reference := &placement.SlopeResolutionReference{
		ProgramPolicyVersion: capture.ProgramPolicyVersion,
		EvidenceRecordID:     capture.EvidenceRecordID, EvidenceType: capture.EvidenceType,
		ArtifactHash: capture.ArtifactHash, BoundaryVersion: capture.BoundaryVersion,
		ParentBoundaryEvidenceRecordID: capture.ParentBoundaryEvidenceRecordID, ParentBoundaryArtifactHash: capture.ParentBoundaryArtifactHash, ParentBoundaryVersion: capture.ParentBoundaryVersion,
		SourceEvidenceRecordID: capture.SourceEvidenceRecordID, SourceArtifactHash: capture.SourceArtifactHash,
		RevisedScreeningEvidenceRecordID: capture.RevisedScreeningEvidenceRecordID, RevisedScreeningArtifactHash: capture.RevisedScreeningArtifactHash,
	}
	return DemoResolutionEvidence{FixtureVersion: capture.FixtureVersion, Label: capture.Label, RecordID: capture.EvidenceRecordID, EvidenceType: capture.EvidenceType, ArtifactHash: capture.ArtifactHash, Artifact: artifact, ReviewerAuthorizationRecordID: capture.ReviewerAuthorizationRecordID, ReviewerAuthorizationHash: capture.ReviewerAuthorizationHash, ReviewerAuthorizationArtifact: authorizationArtifact, ParentBoundaryRecordID: capture.ParentBoundaryEvidenceRecordID, ParentBoundaryArtifactHash: capture.ParentBoundaryArtifactHash, ParentBoundaryArtifact: parentArtifact, RevisedScreeningRecordID: capture.RevisedScreeningEvidenceRecordID, RevisedScreeningArtifactHash: capture.RevisedScreeningArtifactHash, RevisedScreeningArtifact: screeningArtifact}, store, reference, nil
}

type mireyeReplayInput struct {
	Capture  json.RawMessage `json:"capture"`
	Request  json.RawMessage `json:"request"`
	Response json.RawMessage `json:"response"`
}

type mireyeCaptureFixture struct {
	FixtureVersion string    `json:"fixtureVersion"`
	Endpoint       string    `json:"endpoint"`
	RequestID      string    `json:"requestId"`
	HTTPStatus     int       `json:"httpStatus"`
	RetrievedAt    time.Time `json:"retrievedAt"`
	RequestHash    string    `json:"requestHash"`
	ResponseHash   string    `json:"responseHash"`
}

type capturedMireyeResult struct {
	Capture DemoMireyeCapture    `json:"capture"`
	Facts   []evidence.FieldFact `json:"facts"`
}

func executeCapturedMireye(input mireyeReplayInput) (capturedMireyeResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(input.Capture))
	decoder.DisallowUnknownFields()
	var capture mireyeCaptureFixture
	if err := decoder.Decode(&capture); err != nil {
		return capturedMireyeResult{}, fmt.Errorf("decode Mireye capture metadata: %w", err)
	}
	if capture.FixtureVersion == "" || capture.RequestID == "" || capture.Endpoint == "" || capture.RetrievedAt.IsZero() || capture.HTTPStatus != 200 {
		return capturedMireyeResult{}, errors.New("captured Mireye response is missing provenance")
	}
	batch, err := mireye.ReplayFetchBatch(mireye.FetchBatchCapture{SourceURL: capture.Endpoint, RequestID: capture.RequestID, HTTPStatus: capture.HTTPStatus, FetchedAt: capture.RetrievedAt, ExpectedRequestHash: capture.RequestHash, ExpectedResponseHash: capture.ResponseHash, Request: input.Request, Response: input.Response})
	if err != nil {
		return capturedMireyeResult{}, err
	}
	facts := make([]evidence.FieldFact, 0, 10)
	for _, name := range []string{"within_floodplain_polygon", "intersects_wetland", "intersects_nhd_area", "nearest_wetland_distance_m", "wetlands_within_100m_count", "wetlands_within_500m_count", "nearest_groundwater_well_depth_to_water_m", "slope_degrees", "housing_units_within_1km", "nearest_road_distance_m"} {
		fact, aggregateErr := evidence.AggregateFetchBatchFact(batch, name)
		if aggregateErr != nil {
			return capturedMireyeResult{}, fmt.Errorf("aggregate captured Mireye field %s: %w", name, aggregateErr)
		}
		facts = append(facts, fact)
	}
	return capturedMireyeResult{Capture: DemoMireyeCapture{FixtureVersion: capture.FixtureVersion, Endpoint: capture.Endpoint, RequestID: capture.RequestID, HTTPStatus: capture.HTTPStatus, RetrievedAt: capture.RetrievedAt, RequestHash: batch.RequestHash, ResponseHash: batch.ResponseHash, Request: batch.Request, Response: batch.Raw}, Facts: facts}, nil
}

type operatorAdjustmentFixture struct {
	FixtureVersion        string    `json:"fixtureVersion"`
	InputType             string    `json:"inputType"`
	BoundaryVersion       int       `json:"boundaryVersion"`
	RecordedBoundaryAcres float64   `json:"recordedBoundaryAcres"`
	ExcludedAcres         float64   `json:"excludedAcres"`
	EffectiveAcres        float64   `json:"effectiveAcres"`
	RecordedAt            time.Time `json:"recordedAt"`
	Source                string    `json:"source"`
	Reason                string    `json:"reason"`
}

func executeOperatorAdjustment(raw []byte) (DemoAcreageAdjustment, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input operatorAdjustmentFixture
	if err := decoder.Decode(&input); err != nil {
		return DemoAcreageAdjustment{}, fmt.Errorf("decode operator boundary adjustment: %w", err)
	}
	boundary := new(big.Rat).SetFloat64(input.RecordedBoundaryAcres)
	excluded := new(big.Rat).SetFloat64(input.ExcludedAcres)
	effective := new(big.Rat).SetFloat64(input.EffectiveAcres)
	if input.InputType != "OPERATOR_SUPPLIED" || input.FixtureVersion == "" || input.BoundaryVersion < 1 || input.RecordedAt.IsZero() || boundary == nil || excluded == nil || effective == nil || boundary.Sign() <= 0 || excluded.Sign() <= 0 || effective.Sign() <= 0 {
		return DemoAcreageAdjustment{}, errors.New("operator boundary adjustment is incomplete")
	}
	if new(big.Rat).Sub(boundary, excluded).Cmp(effective) != 0 {
		return DemoAcreageAdjustment{}, errors.New("operator acreage adjustment does not reconcile")
	}
	var canonical bytes.Buffer
	if err := json.Compact(&canonical, raw); err != nil {
		return DemoAcreageAdjustment{}, err
	}
	return DemoAcreageAdjustment{FixtureVersion: input.FixtureVersion, InputType: input.InputType, BoundaryVersion: input.BoundaryVersion, RecordedBoundaryAcres: decimalString(boundary), ExcludedAcres: decimalString(excluded), EffectiveAcres: decimalString(effective), RecordedAt: input.RecordedAt, Source: input.Source, Reason: input.Reason, InputHash: hashBytes(canonical.Bytes()), RawFixture: append(json.RawMessage(nil), raw...)}, nil
}

type toolRecorder struct {
	now   func() time.Time
	calls []DemoToolCall
}

func (r *toolRecorder) startedAt() time.Time {
	if len(r.calls) == 0 {
		return r.now().UTC()
	}
	return r.calls[0].StartedAt
}

func executeTool[T any](recorder *toolRecorder, name, sourceURL, requestID string, input any, run func() (T, error), summarize func(T) string) (T, error) {
	started := recorder.now().UTC()
	inputJSON, inputErr := json.Marshal(input)
	if inputErr != nil {
		var zero T
		return zero, inputErr
	}
	result, err := run()
	completed := recorder.now().UTC()
	outputJSON, outputErr := json.Marshal(result)
	if err == nil && outputErr != nil {
		err = outputErr
	}
	call := DemoToolCall{Position: len(recorder.calls) + 1, ToolName: name, Status: "SUCCEEDED", SourceURL: sourceURL, RequestID: requestID, InputHash: hashBytes(inputJSON), OutputHash: hashBytes(outputJSON), Input: inputJSON, Output: outputJSON, StartedAt: started, CompletedAt: completed}
	if err != nil {
		call.Status = "FAILED"
		call.Error = err.Error()
		call.Summary = name + " failed: " + err.Error()
	} else {
		call.Summary = summarize(result)
	}
	recorder.calls = append(recorder.calls, call)
	return result, err
}

func policyInput(extraction lab.Extraction) policy.ClassificationInput {
	input := policy.ClassificationInput{Jurisdiction: "MI", Matrix: extraction.Draft.Matrix, Method: extraction.Draft.Method, Basis: extraction.Draft.Basis, Analytes: make([]policy.AnalyteEvidence, 0, len(extraction.Draft.Analytes))}
	for _, analyte := range extraction.Draft.Analytes {
		var upperBound *string
		if analyte.IsNonDetect {
			upperBound = analyte.NormalizedReportingLimitUGKGDry
			if upperBound == nil {
				upperBound = analyte.NormalizedDetectionLimitUGKGDry
			}
		}
		input.Analytes = append(input.Analytes, policy.AnalyteEvidence{CanonicalAnalyte: analyte.CanonicalAnalyte, ResultText: analyte.ResultText, IsNonDetect: analyte.IsNonDetect, NormalizedValueUGKGDry: analyte.NormalizedValueUGKGDry, UpperBoundUGKGDry: upperBound, SourcePage: analyte.SourcePage})
	}
	return input
}

func evaluate(ctx context.Context, input placement.Input, evidenceStore placement.SlopeEvidenceStore) (placement.PlacementPlan, string, error) {
	inputHash := hashValue(input)
	input.DecisionInputHash = inputHash
	plan, err := placement.EvaluateWithEvidence(ctx, input, evidenceStore)
	if err != nil {
		return placement.PlacementPlan{}, "", err
	}
	plan.InputHash = inputHash
	return plan, inputHash, nil
}

func validatePlan(plan placement.PlacementPlan) error {
	type fieldState struct {
		capacity    string
		disposition placement.Disposition
		rank        int
	}
	fields := make(map[string]fieldState, len(plan.Fields))
	ranks := make(map[int]struct{}, len(plan.Fields))
	for _, field := range plan.Fields {
		if field.Rank == nil || *field.Rank < 1 {
			return fmt.Errorf("field %s has no valid rank", field.FieldName)
		}
		if _, duplicate := ranks[*field.Rank]; duplicate {
			return fmt.Errorf("field rank %d is duplicated", *field.Rank)
		}
		ranks[*field.Rank] = struct{}{}
		fields[field.FieldID] = fieldState{capacity: field.AvailableCapacity, disposition: field.Disposition, rank: *field.Rank}
	}
	for _, item := range plan.Allocations {
		field, ok := fields[item.FieldID]
		if !ok || field.disposition != placement.DispositionEligible || field.capacity == "" {
			return fmt.Errorf("allocation references field %s without eligible calculated capacity", item.FieldName)
		}
		if item.Position != field.rank {
			return fmt.Errorf("allocation position %d disagrees with field rank %d", item.Position, field.rank)
		}
		if greaterDecimal(item.DryTons, field.capacity) {
			return fmt.Errorf("allocation %s exceeds field capacity %s", item.DryTons, field.capacity)
		}
	}
	if !equalDecimalSum(plan.AllocatedDryTons, plan.UnallocatedDryTons, plan.BatchDryTons) {
		return errors.New("allocation mass balance does not equal batch total")
	}
	return nil
}

func fieldA(usableAcres string, facts []placement.FactInput, boundaryVersion int, resolution *placement.SlopeResolutionReference) placement.FieldInput {
	field := eligibleField("0f153cf7-0d2e-4bba-9aee-9388ae2379a1", "Riverbend East", usableAcres, "1.2", "8", facts)
	field.BoundaryVersion = boundaryVersion
	field.SlopeResolution = resolution
	return field
}

func fieldB() placement.FieldInput {
	return eligibleField("5c3ee8c7-8e62-4429-8cbb-2261947715ae", "North Forty", "25", "1.2", "2", seededFieldBFacts())
}

func eligibleField(id, name, acres, rate, prior string, facts []placement.FactInput) placement.FieldInput {
	return placement.FieldInput{ID: id, Name: name, Status: "READY", RMPApproved: true, UsableAcres: acres, AgronomicRate: rate, PriorLoadingDryTons: prior, CropOrUse: "corn", PhysicalEvaluationID: "seeded-evidence-" + id[:8], PhysicalStatus: "SUCCEEDED", SupplementalAvailable: true, Facts: facts}
}

func buildCitations(retrieved time.Time, extraction lab.Extraction, classification policyResult, captured capturedMireyeResult, adjustment DemoAcreageAdjustment, resolution *DemoResolutionEvidence, after placement.PlacementPlan) []DemoCitation {
	values := map[string]string{}
	for _, analyte := range extraction.Draft.Analytes {
		if analyte.NormalizedValueUGKGDry != nil {
			values[analyte.CanonicalAnalyte] = *analyte.NormalizedValueUGKGDry
		}
	}
	slope := physicalFact(captured.Facts, "slope_degrees")
	conversion := placement.SlopeConversionForDegrees(rangeMaxFloat(slope.Value))
	citations := []DemoCitation{{ID: "slope", Finding: "Original sampled slope", Value: conversion.OriginalDegrees + "° = " + conversion.DerivedGradePercent + "% grade", Source: "USGS 3DEP through captured Mireye batch", SourceURL: slope.SourceURL, RetrievedAt: valueTime(slope.FetchedAt, captured.Capture.RetrievedAt), Effect: "Exceeded Michigan's 6% grade (" + conversion.ThresholdDegrees + "°) surface threshold, placed Field A into review, and left " + after.UnallocatedDryTons + " dry tons unallocated."}, {ID: "acreage", Finding: "Boundary adjustment", Value: adjustment.ExcludedAcres + " acres excluded", Source: adjustment.Source, SourceURL: "#operator-boundary-input", RetrievedAt: adjustment.RecordedAt, Effect: "Records " + adjustment.EffectiveAcres + " effective acres but does not prove that the high-slope sample was excluded."}, {ID: "rate", Finding: "Field A agronomic rate", Value: "1.2 dry tons/acre", Source: "Approved RMP agronomic record", SourceURL: "#rmp-source", RetrievedAt: retrieved, Effect: "Will control capacity only after the slope-review gate is resolved."}, {ID: "prior-loading", Finding: "Field A prior loading", Value: "8 dry tons", Source: "Operator loading ledger", SourceURL: "#loading-source", RetrievedAt: retrieved, Effect: "Will reduce Field A's capacity when supported slope evidence permits recalculation."}, {ID: "policy", Finding: "PFOS/PFOA classification", Value: values["PFOS"] + " / " + values["PFOA"] + " µg/kg dry", Source: "Michigan rule pack " + classification.RulePack.Version, SourceURL: classification.RulePack.SourceURL, RetrievedAt: classification.RulePack.RetrievedAt, Effect: "PFAS is one batch input; it does not establish field readiness."}}
	if resolution != nil {
		citations[0].Effect = "The verified boundary geometry excludes the high-slope sample, so the engine reconsidered Field A."
		citations[1].Effect = "Superseded for calculation by verified boundary version " + strconv.Itoa(resolution.Verification.BoundaryVersion) + "."
		citations[2].Effect = "Controls the verified Field A capacity calculation."
		citations[3].Effect = "Reduces verified Field A capacity to " + capacity(after, "Riverbend East") + " dry tons."
		citations = append(citations,
			DemoCitation{ID: "resolution", Finding: "Parent-bound revised geometry", Value: resolution.Verification.DerivedUsableAcres + " geometry-derived acres", Source: resolution.Label, SourceURL: "#reviewed-evidence", RetrievedAt: resolution.Verification.RecordedAt, Effect: "Matched parent boundary v" + strconv.Itoa(resolution.Verification.ParentBoundaryVersion) + ", remained inside it, and preserved immutable reviewer authorization."},
			DemoCitation{ID: "revised-screening", Finding: "Sampled terrain screen", Value: resolution.Verification.RevisedScreening.MaximumSlopeDegrees + "° = " + resolution.Verification.RevisedScreening.MaximumSlopeGradePercent + "% grade sampled maximum", Source: "USGS 3DEP through captured Mireye batch", SourceURL: resolution.Verification.RevisedScreening.Endpoint, RetrievedAt: resolution.Verification.RevisedScreening.RetrievedAt, Effect: strconv.Itoa(resolution.Verification.RevisedScreening.ReturnedSampleCount) + " sampled locations returned slopes below the screening threshold. Unsampled terrain may contain different conditions; this does not establish whole-field slope suitability."},
		)
	}
	return citations
}

func preScreenFacts() []placement.FactInput {
	return supportingFacts("Pre-screen counterfactual, not physical evidence", 150, 0.4, 1.8, 0.9)
}

func seededFieldBFacts() []placement.FactInput {
	return supportingFacts("Seeded Field B evidence record", 145, 0.4, 1.8, 0.9)
}

func supportingFacts(source string, road, minSlope, maxSlope, medianSlope float64) []placement.FactInput {
	return []placement.FactInput{booleanFact("within_floodplain_polygon", "Floodplain intersects application boundary", false, source), booleanFact("intersects_wetland", "Wetland intersects application boundary", false, source), booleanFact("intersects_nhd_area", "Mapped surface water intersects application boundary", false, source), numberFact("nearest_wetland_distance_m", "Nearest wetland", 800, source), numberFact("wetlands_within_100m_count", "Wetlands within 100 m", 0, source), numberFact("wetlands_within_500m_count", "Wetlands within 500 m", 0, source), numberFact("nearest_groundwater_well_depth_to_water_m", "Nearest measured depth to groundwater", 4.2, source), rangeFact("slope_degrees", "Slope", minSlope, maxSlope, medianSlope, source), numberFact("housing_units_within_1km", "Homes within 1 km", 3, source), numberFact("nearest_road_distance_m", "Nearest road", road, source)}
}

func placementFacts(facts []evidence.FieldFact) []placement.FactInput {
	result := make([]placement.FactInput, 0, len(facts))
	for _, fact := range facts {
		result = append(result, placement.FactInput{Name: fact.Name, Label: fact.Label, State: fact.State, Value: append(json.RawMessage(nil), fact.Value...), Unit: fact.Unit, Source: fact.Source, SourceURL: fact.SourceURL})
	}
	return result
}

func booleanFact(name, label string, value bool, source string) placement.FactInput {
	encoded, _ := json.Marshal(value)
	return placement.FactInput{Name: name, Label: label, State: "COMPLETE", Value: encoded, Source: source}
}

func numberFact(name, label string, value float64, source string) placement.FactInput {
	encoded, _ := json.Marshal(value)
	return placement.FactInput{Name: name, Label: label, State: "COMPLETE", Value: encoded, Source: source}
}

func rangeFact(name, label string, min, max, median float64, source string) placement.FactInput {
	encoded, _ := json.Marshal(map[string]float64{"min": min, "max": max, "median": median})
	return placement.FactInput{Name: name, Label: label, State: "COMPLETE", Value: encoded, Source: source}
}

func physicalFact(facts []evidence.FieldFact, name string) evidence.FieldFact {
	for _, fact := range facts {
		if fact.Name == name {
			return fact
		}
	}
	return evidence.FieldFact{}
}

func rangeMaxFloat(value json.RawMessage) float64 {
	var result struct {
		Max float64 `json:"max"`
	}
	_ = json.Unmarshal(value, &result)
	return result.Max
}

func valueTime(value *time.Time, fallback time.Time) time.Time {
	if value != nil {
		return *value
	}
	return fallback
}

func capacity(plan placement.PlacementPlan, fieldName string) string {
	for _, field := range plan.Fields {
		if field.FieldName == fieldName {
			return field.AvailableCapacity
		}
	}
	return "0"
}

func allocation(plan placement.PlacementPlan, fieldName string) string {
	for _, item := range plan.Allocations {
		if item.FieldName == fieldName {
			return item.DryTons
		}
	}
	return "0"
}

func hashValue(value any) string {
	encoded, _ := json.Marshal(value)
	return hashBytes(encoded)
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func decimalString(value *big.Rat) string {
	return strings.TrimRight(strings.TrimRight(value.FloatString(6), "0"), ".")
}

func greaterDecimal(left, right string) bool {
	l, lok := new(big.Rat).SetString(left)
	r, rok := new(big.Rat).SetString(right)
	return !lok || !rok || l.Cmp(r) > 0
}

func equalDecimalSum(left, right, total string) bool {
	l, lok := new(big.Rat).SetString(left)
	r, rok := new(big.Rat).SetString(right)
	t, tok := new(big.Rat).SetString(total)
	return lok && rok && tok && new(big.Rat).Add(l, r).Cmp(t) == 0
}
