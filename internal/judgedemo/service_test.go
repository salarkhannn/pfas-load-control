package judgedemo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
)

func TestStartExecutesCapturedAdaptersAndPreservesAllocationInvariants(t *testing.T) {
	service := newService(newMemoryStore())
	fixed := time.Date(2026, time.August, 20, 10, 5, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }
	run, err := service.Start(context.Background(), "test-run-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == "" || run.Package.ID == "" || len(run.ToolCalls) != 6 || run.FreezeReceipt.Position != 7 {
		t.Fatalf("run was not fully recorded: %#v", run)
	}
	for _, call := range run.ToolCalls {
		if call.Status != "SUCCEEDED" || len(call.Input) == 0 || len(call.Output) == 0 || call.Error != "" {
			t.Fatalf("tool call did not come from execution: %#v", call)
		}
		if digest(call.Input) != call.InputHash || digest(call.Output) != call.OutputHash {
			t.Fatalf("tool call hashes do not match recorded I/O: %#v", call)
		}
	}
	if run.MireyeCapture.Endpoint != "https://api.mireye.com/v1/fetch/batch" || run.MireyeCapture.RequestID != "req_088804c2fcb6" || run.MireyeCapture.ResponseHash != "8a6cfc9863a09c337be4290b4da156296357ebe25463f801fcabc27835ec737a" {
		t.Fatalf("captured Mireye provenance mismatch: %#v", run.MireyeCapture)
	}
	if len(run.PhysicalEvidence) != 10 {
		t.Fatalf("expected ten captured physical facts, got %d", len(run.PhysicalEvidence))
	}
	for _, fact := range run.PhysicalEvidence {
		if len(fact.Samples) != 5 {
			t.Fatalf("fact %s did not preserve its five provider samples", fact.Name)
		}
		for _, sample := range fact.Samples {
			if sample.Source == "" || sample.SourceURL == "" {
				t.Fatalf("fact %s sample %d lost provider attribution: %#v", fact.Name, sample.Index, sample)
			}
		}
	}
	if run.AcreageAdjustment.InputType != "OPERATOR_SUPPLIED" || run.AcreageAdjustment.ExcludedAcres != "18.4" || run.AcreageAdjustment.EffectiveAcres != "31.6" {
		t.Fatalf("operator acreage attribution missing: %#v", run.AcreageAdjustment)
	}
	if allocation(run.Before, "Riverbend East") != "52" {
		t.Fatalf("unexpected pre-screen allocation: %#v", run.Before.Allocations)
	}
	if run.RunStatus != "SUCCEEDED" || run.CalculationStatus != string(placement.StatusReviewRequired) || run.AuthorizationStatus != "REQUIRED" || !run.AuthorizationRequired || run.After.Status != placement.StatusReviewRequired || !run.ReviewRequired {
		t.Fatalf("run, calculation, and authorization statuses disagree: %#v", run)
	}
	if allocation(run.After, "North Forty") != "28" || allocation(run.After, "Riverbend East") != "0" || run.After.AllocatedDryTons != "28" || run.After.UnallocatedDryTons != "24" {
		t.Fatalf("captured slope did not block Field A allocation: %#v", run.After)
	}
	fieldB := placementField(run.After, "North Forty")
	fieldA := placementField(run.After, "Riverbend East")
	if fieldB.Disposition != placement.DispositionEligible || fieldB.Rank == nil || *fieldB.Rank != 1 {
		t.Fatalf("Field B rank or disposition mismatch: %#v", fieldB)
	}
	if fieldA.Disposition != placement.DispositionReviewRequired || fieldA.Rank == nil || *fieldA.Rank != 2 || fieldA.AvailableCapacity != "" {
		t.Fatalf("Field A was not consistently blocked before capacity: %#v", fieldA)
	}
	if len(run.Package.Artifact) == 0 || digest(run.Package.Artifact) != run.Package.DecisionHash {
		t.Fatal("exact frozen package bytes were not preserved")
	}
	frozen, envelope := decodePackageEnvelope(t, run.Package.Artifact)
	if digest(envelope.DecisionPayload) != run.Package.PayloadHash || envelope.FreezeReceipt != run.FreezeReceipt || envelope.FreezeReceipt.ArtifactHash != run.Package.PayloadHash || len(frozen.ExecutedCalls) != len(run.ToolCalls) {
		t.Fatalf("freeze receipt or decision timeline mismatch: envelope=%#v run=%#v", envelope, run)
	}
	if frozen.After.Status != run.After.Status || frozen.After.UnallocatedDryTons != "24" || allocation(frozen.After, "Riverbend East") != "0" {
		t.Fatalf("frozen package disagrees with the run: %#v", frozen.After)
	}
	frozenA := placementField(frozen.After, "Riverbend East")
	frozenB := placementField(frozen.After, "North Forty")
	if frozenA.Rank == nil || *frozenA.Rank != 2 || frozenB.Rank == nil || *frozenB.Rank != 1 {
		t.Fatalf("frozen package ranks disagree with the run: %#v", frozen.After.Fields)
	}
	assertPlanInvariants(t, run.After)
}

func TestOperatorAcreageDoesNotResolveSlopeReview(t *testing.T) {
	service := newService(newMemoryStore())
	var fixture map[string]any
	if err := json.Unmarshal(service.operatorAdjustmentFixture, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture["excludedAcres"] = 25
	fixture["effectiveAcres"] = 25
	service.operatorAdjustmentFixture, _ = json.Marshal(fixture)
	run, err := service.Start(context.Background(), "changed-adjustment")
	if err != nil {
		t.Fatal(err)
	}
	if run.AcreageAdjustment.EffectiveAcres != "25" {
		t.Fatalf("changed operator acreage did not reach the engine: %#v", run.AcreageAdjustment)
	}
	fieldA := placementField(run.After, "Riverbend East")
	if fieldA.Disposition != placement.DispositionReviewRequired || fieldA.AvailableCapacity != "" || allocation(run.After, "Riverbend East") != "0" || run.After.UnallocatedDryTons != "24" {
		t.Fatalf("acreage alone incorrectly resolved the slope gate: %#v", run.After)
	}
}

func TestReviewedEvidenceRunVerifiesGeometryAndCompletesAllocation(t *testing.T) {
	service := newService(newMemoryStore())
	fixed := time.Date(2026, time.August, 20, 10, 5, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }
	initial, err := service.Start(context.Background(), "review-loop-initial")
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := service.StartReviewed(context.Background(), "review-loop-reviewed", initial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Mode != modeReviewed || reviewed.ParentRunID != initial.ID || reviewed.RunStatus != "SUCCEEDED" || reviewed.CalculationStatus != string(placement.StatusReady) || reviewed.AuthorizationStatus != "REQUIRED" || !reviewed.AuthorizationRequired || reviewed.ReviewRequired {
		t.Fatalf("reviewed run status is inconsistent: %#v", reviewed)
	}
	if len(reviewed.ToolCalls) != 8 || reviewed.ToolCalls[5].ToolName != "mireye.revised-boundary.screen" || reviewed.ToolCalls[6].ToolName != "slope-resolution.verify" || reviewed.ToolCalls[6].Status != "SUCCEEDED" || reviewed.FreezeReceipt.Position != 9 {
		t.Fatalf("reviewed evidence verification was not executed: %#v", reviewed.ToolCalls)
	}
	if allocation(reviewed.After, "North Forty") != "28" || allocation(reviewed.After, "Riverbend East") != "24" || reviewed.After.UnallocatedDryTons != "0" {
		t.Fatalf("reviewed allocation is wrong: %#v", reviewed.After)
	}
	fieldA := placementField(reviewed.After, "Riverbend East")
	fieldB := placementField(reviewed.After, "North Forty")
	if fieldB.Rank == nil || *fieldB.Rank != 1 || fieldA.Rank == nil || *fieldA.Rank != 2 || fieldA.Disposition != placement.DispositionEligible || fieldA.AvailableCapacity != "29.92" {
		t.Fatalf("reviewed ranks, disposition, or capacity are wrong: %#v", reviewed.After.Fields)
	}
	if fieldA.SlopeResolution == nil || fieldA.SlopeResolution.DerivedUsableAcres != "31.6" || fieldA.SlopeResolution.HighSlopeSamplesExcluded != 1 || fieldA.SlopeResolution.ParentBoundaryArtifactHash != "790c4933ec22fcf16ad4630c18ca6c8bd29877f786e8e059d8106cfefe85d7c3" || fieldA.SlopeResolution.RevisedScreening.ReturnedSampleCount != 5 || fieldA.SlopeResolution.RevisedScreening.Status != "SAMPLED_TERRAIN_SCREEN_PASSED" || fieldA.SlopeResolution.ReviewerAuthorizationHash != "5606f59ece4f37aff1e19f7ee20d95122b3b86197a55773a5944f5e43b640655" {
		t.Fatalf("verified resolution did not reach the placement record: %#v", fieldA.SlopeResolution)
	}
	if reviewed.ResolutionEvidence == nil || reviewed.ResolutionEvidence.ArtifactHash != "46190d7fc67eca328a845f62c1c41d0449505aeda5408bc49f35c0ec14cdc78e" || len(reviewed.ResolutionEvidence.Artifact) == 0 || reviewed.ResolutionEvidence.ReviewerAuthorizationHash != "5606f59ece4f37aff1e19f7ee20d95122b3b86197a55773a5944f5e43b640655" || len(reviewed.ResolutionEvidence.ReviewerAuthorizationArtifact) == 0 || reviewed.ResolutionEvidence.ParentBoundaryRecordID == "" || len(reviewed.ResolutionEvidence.ParentBoundaryArtifact) == 0 || reviewed.ResolutionEvidence.RevisedScreeningRecordID == "" || len(reviewed.ResolutionEvidence.RevisedScreeningArtifact) == 0 {
		t.Fatalf("immutable resolution artifact is missing: %#v", reviewed.ResolutionEvidence)
	}
	frozen, envelope := decodePackageEnvelope(t, reviewed.Package.Artifact)
	if frozen.Mode != modeReviewed || frozen.ParentRunID != initial.ID || frozen.ResolutionEvidence == nil || frozen.ResolutionEvidence.ArtifactHash != reviewed.ResolutionEvidence.ArtifactHash || frozen.After.UnallocatedDryTons != "0" {
		t.Fatalf("frozen reviewed package disagrees with the run: %#v", frozen)
	}
	if frozen.RunStatus != "SUCCEEDED" || frozen.CalculationStatus != string(placement.StatusReady) || frozen.AuthorizationStatus != "REQUIRED" || !frozen.AuthorizationRequired || len(frozen.ExecutedCalls) != len(reviewed.ToolCalls) || envelope.FreezeReceipt != reviewed.FreezeReceipt || digest(envelope.DecisionPayload) != reviewed.Package.PayloadHash {
		t.Fatalf("downloaded timeline, statuses, or freeze receipt disagree with the run: %#v", envelope)
	}
	for index, call := range frozen.ExecutedCalls {
		if call.ToolName != reviewed.ToolCalls[index].ToolName || call.Position != reviewed.ToolCalls[index].Position || call.ToolName == "decisionpackage.freeze" {
			t.Fatalf("downloaded decision call %d is missing or unexplained: %#v", index, call)
		}
	}
	record, err := json.Marshal(reviewed)
	if err != nil {
		t.Fatal(err)
	}
	hydrated, err := hydrateStoredRun(db.PfasJudgeDemoRun{
		ID:              uuid.MustParse(reviewed.ID),
		Status:          reviewed.RunStatus,
		FixtureVersion:  reviewed.FixtureVersion,
		Record:          record,
		DecisionHash:    reviewed.Package.DecisionHash,
		PackageArtifact: append([]byte(nil), reviewed.Package.Artifact...),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hydrated.ResolutionEvidence == nil || hydrated.ResolutionEvidence.ArtifactHash != reviewed.ResolutionEvidence.ArtifactHash || hydrated.ParentRunID != initial.ID || allocation(hydrated.After, "Riverbend East") != "24" {
		t.Fatalf("reviewed evidence did not survive database hydration: %#v", hydrated)
	}
	assertPlanInvariants(t, reviewed.After)
}

func TestReviewedEvidenceRunIsIdempotentPerAction(t *testing.T) {
	store := newMemoryStore()
	service := newService(store)
	initial, err := service.Start(context.Background(), "reviewed-idempotency-initial")
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.StartReviewed(context.Background(), "reviewed-idempotency-action", initial.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newService(store).StartReviewed(context.Background(), "reviewed-idempotency-action", initial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Package.DecisionHash != second.Package.DecisionHash || string(first.Package.Artifact) != string(second.Package.Artifact) {
		t.Fatal("repeated reviewed action did not return its original frozen run")
	}
	third, err := service.StartReviewed(context.Background(), "reviewed-idempotency-new-action", initial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if third.ID == first.ID {
		t.Fatal("a new reviewed action did not create a new run")
	}
}

func TestReviewedEvidenceArtifactTamperingFailsClosed(t *testing.T) {
	service := newService(newMemoryStore())
	initial, err := service.Start(context.Background(), "tampered-resolution-initial")
	if err != nil {
		t.Fatal(err)
	}
	service.resolutionArtifactFixture = append([]byte(nil), service.resolutionArtifactFixture...)
	service.resolutionArtifactFixture[len(service.resolutionArtifactFixture)-2] ^= 0x01
	if _, err := service.StartReviewed(context.Background(), "tampered-resolution-reviewed", initial.ID); err == nil {
		t.Fatal("tampered reviewed evidence was accepted")
	}
}

func TestReviewedAuthorizationArtifactTamperingFailsClosed(t *testing.T) {
	service := newService(newMemoryStore())
	initial, err := service.Start(context.Background(), "tampered-authorization-initial")
	if err != nil {
		t.Fatal(err)
	}
	service.resolutionAuthorizationFixture = append([]byte(nil), service.resolutionAuthorizationFixture...)
	service.resolutionAuthorizationFixture[len(service.resolutionAuthorizationFixture)-2] ^= 0x01
	if _, err := service.StartReviewed(context.Background(), "tampered-authorization-reviewed", initial.ID); err == nil {
		t.Fatal("tampered reviewer authorization was accepted")
	}
}

func TestDistinctRanksSurviveDatabaseHydration(t *testing.T) {
	service := newService(newMemoryStore())
	run, err := service.Start(context.Background(), "rank-database-roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	record, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	hydrated, err := hydrateStoredRun(db.PfasJudgeDemoRun{
		ID:              uuid.MustParse(run.ID),
		Status:          run.RunStatus,
		FixtureVersion:  run.FixtureVersion,
		Record:          record,
		DecisionHash:    run.Package.DecisionHash,
		PackageArtifact: append([]byte(nil), run.Package.Artifact...),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fieldB := placementField(hydrated.After, "North Forty")
	fieldA := placementField(hydrated.After, "Riverbend East")
	if fieldB.Rank == nil || *fieldB.Rank != 1 || fieldA.Rank == nil || *fieldA.Rank != 2 || fieldB.Rank == fieldA.Rank {
		t.Fatalf("distinct ranks did not survive the database retrieval path: %#v", hydrated.After.Fields)
	}
	if len(hydrated.After.Allocations) != 1 || hydrated.After.Allocations[0].Position != *fieldB.Rank {
		t.Fatalf("allocation positions disagree with field ranks: %#v", hydrated.After)
	}
}

func TestStoredPackageTamperingFailsIntegrityVerification(t *testing.T) {
	store := newMemoryStore()
	service := newService(store)
	run, err := service.Start(context.Background(), "tamper-test")
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	tampered := store.runs[run.ID]
	tampered.Package.Artifact = append([]byte(nil), tampered.Package.Artifact...)
	tampered.Package.Artifact[0] ^= 0xff
	store.runs[run.ID] = tampered
	store.mu.Unlock()
	if _, err := service.Get(context.Background(), run.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("tampered artifact was not rejected: %v", err)
	}
}

func TestIdempotencySurvivesServiceRestartWithoutReexecution(t *testing.T) {
	store := newMemoryStore()
	firstService := newService(store)
	first, err := firstService.Start(context.Background(), "same-page-visit")
	if err != nil {
		t.Fatal(err)
	}
	secondService := newService(store)
	second, err := secondService.Start(context.Background(), "same-page-visit")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Package.DecisionHash != second.Package.DecisionHash || string(first.Package.Artifact) != string(second.Package.Artifact) {
		t.Fatal("idempotent retrieval changed the frozen record")
	}
	third, err := secondService.Start(context.Background(), "explicit-rerun")
	if err != nil {
		t.Fatal(err)
	}
	if third.ID == first.ID {
		t.Fatal("a new idempotency key did not create a new run")
	}
}

func assertPlanInvariants(t *testing.T, plan placement.PlacementPlan) {
	t.Helper()
	capacities := make(map[string]*big.Rat, len(plan.Fields))
	for _, field := range plan.Fields {
		if field.AvailableCapacity == "" {
			continue
		}
		capacity, ok := new(big.Rat).SetString(field.AvailableCapacity)
		if !ok {
			t.Fatalf("invalid capacity %q", field.AvailableCapacity)
		}
		capacities[field.FieldID] = capacity
	}
	allocated := new(big.Rat)
	for _, item := range plan.Allocations {
		amount, ok := new(big.Rat).SetString(item.DryTons)
		if !ok {
			t.Fatalf("invalid allocation %q", item.DryTons)
		}
		capacity, ok := capacities[item.FieldID]
		if !ok {
			t.Fatalf("allocation references a field without calculated capacity: %#v", item)
		}
		if amount.Cmp(capacity) > 0 {
			t.Fatalf("allocation %s exceeds capacity %s", amount, capacities[item.FieldID])
		}
		allocated.Add(allocated, amount)
	}
	unallocated, _ := new(big.Rat).SetString(plan.UnallocatedDryTons)
	batch, _ := new(big.Rat).SetString(plan.BatchDryTons)
	if new(big.Rat).Add(allocated, unallocated).Cmp(batch) != 0 {
		t.Fatal("mass balance failed")
	}
}

func decodePackageEnvelope(t *testing.T, artifact []byte) (packagePayload, packageEnvelope) {
	t.Helper()
	var envelope packageEnvelope
	if err := json.Unmarshal(artifact, &envelope); err != nil {
		t.Fatal(err)
	}
	var payload packagePayload
	if err := json.Unmarshal(envelope.DecisionPayload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload, envelope
}

func placementField(plan placement.PlacementPlan, name string) placement.PlacementField {
	for _, field := range plan.Fields {
		if field.FieldName == name {
			return field
		}
	}
	return placement.PlacementField{}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
