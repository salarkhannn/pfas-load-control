package decisionpackage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/lab"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
	"github.com/salarkhannn/pfas-load-control/internal/policy"
	"github.com/salarkhannn/pfas-load-control/internal/responseplan"
)

func TestPackageStatusPreservesReviewGaps(t *testing.T) {
	snapshot := testSnapshot()
	snapshot.Gaps = []PackageGap{{Source: "PHYSICAL_EVIDENCE", Code: "SOIL_DRAINAGE", Detail: "Missing", Resolution: "Review", Critical: true}}
	if got := packageStatus(snapshot); got != "REVIEW_REQUIRED" {
		t.Fatalf("packageStatus() = %q, want REVIEW_REQUIRED", got)
	}
}

func TestProposedActionsAreNonExecutableAndDeduplicated(t *testing.T) {
	snapshot := testSnapshot()
	snapshot.Decision.Requirements = []policy.Requirement{{ID: "SUBMIT", RuleID: "standard", Title: "Submit results", Detail: "Submit confirmed results.", Timing: "Before application"}}
	actions := buildProposedActions(snapshot)
	if len(actions) != 2 {
		t.Fatalf("len(actions) = %d, want 2", len(actions))
	}
	for index, action := range actions {
		if action.Executable {
			t.Fatalf("action %q is executable", action.Code)
		}
		if action.Position != index+1 {
			t.Fatalf("action position = %d, want %d", action.Position, index+1)
		}
	}
}

func TestProposedActionsMergeEquivalentPolicyAndResponseWork(t *testing.T) {
	snapshot := testSnapshot()
	snapshot.Decision.Requirements = []policy.Requirement{{ID: "MI-PFAS-ELEVATED-RATE", RuleID: "elevated", Title: "Limit the application rate", Detail: "Use no more than 1.5 dry tons per acre.", Timing: "Before land application"}}
	snapshot.Response = &responseplan.ResponseRun{ID: "66666666-6666-6666-6666-666666666666", Tasks: []responseplan.ResponseTask{{Code: "ENFORCE_RATE_CAP", Category: "CONTROL", State: "ENFORCED", Title: "Keep the PFAS rate ceiling in the placement plan", Detail: "Enforce the rate ceiling.", Timing: "Before land application"}}}
	actions := buildProposedActions(snapshot)
	if len(actions) != 2 {
		t.Fatalf("len(actions) = %d, want one merged rate action plus placement review", len(actions))
	}
	if actions[0].Code != "ENFORCE_RATE_CAP" || actions[0].State != "ENFORCED" {
		t.Fatalf("merged action = %#v, want ENFORCE_RATE_CAP in ENFORCED state", actions[0])
	}
}

func TestInputHashIncludesFrozenPhysicalEvidence(t *testing.T) {
	first := testSnapshot()
	first.PhysicalEvidence = []evidence.Evaluation{{ID: "77777777-7777-7777-7777-777777777777", Facts: []evidence.FieldFact{{Name: "slope", Value: json.RawMessage(`1.2`)}}}}
	second := first
	second.PhysicalEvidence = append([]evidence.Evaluation(nil), first.PhysicalEvidence...)
	second.PhysicalEvidence[0].Facts = []evidence.FieldFact{{Name: "slope", Value: json.RawMessage(`2.4`)}}
	firstHash, err := hashInputs(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := hashInputs(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("physical evidence changed without changing the package input hash")
	}
}

func TestDetectedLabValueHasNoNonDetectUpperBound(t *testing.T) {
	value := "42"
	reportingLimit := "0.5"
	report := lab.Report{
		ID: "55555555-5555-5555-5555-555555555555", Status: lab.StatusConfirmed, OriginalFilename: "report.csv", MediaType: "text/csv", SHA256: strings.Repeat("b", 64),
		Draft: &lab.Draft{Version: 1, Analytes: []lab.Analyte{{CanonicalAnalyte: "PFOS", ResultText: value, NormalizedValueUGKGDry: &value, NormalizedReportingLimitUGKGDry: &reportingLimit, IsNonDetect: false}}},
	}
	snapshot := buildSnapshot(testSnapshot().Decision, report, nil, nil, nil)
	if snapshot.Lab.Analytes[0].UpperBound != nil {
		t.Fatalf("detected result upper bound = %q, want omitted", *snapshot.Lab.Analytes[0].UpperBound)
	}
}

func TestPhysicalEvidenceWithVisibleGapsIsPartial(t *testing.T) {
	snapshot := testSnapshot()
	snapshot.PhysicalEvidence = []evidence.Evaluation{{ID: "77777777-7777-7777-7777-777777777777", Status: evidence.StatusSucceeded, Gaps: []evidence.PhysicalDataGap{{Code: "NO_DATA", Detail: "Unavailable", Critical: false}}}}
	entries := buildEvidenceLedger(snapshot)
	for _, entry := range entries {
		if entry.Kind == "PHYSICAL_EVIDENCE" {
			if entry.Status != "PARTIAL" {
				t.Fatalf("physical evidence status = %q, want PARTIAL", entry.Status)
			}
			return
		}
	}
	t.Fatal("physical evidence ledger entry not found")
}

func TestExportsContainPackageBoundary(t *testing.T) {
	value := DecisionPackage{
		ID: "11111111-1111-1111-1111-111111111111", DecisionID: "22222222-2222-2222-2222-222222222222",
		SchemaVersion: SchemaVersion, Status: "READY", InputHash: strings.Repeat("a", 64), Snapshot: testSnapshot(),
		Evidence:        []EvidenceEntry{{Position: 1, Kind: "LAB_REPORT", Provider: "Laboratory report", Title: "Confirmed PFAS laboratory evidence", Status: "AVAILABLE", Detail: "Confirmed source."}},
		ProposedActions: []ProposedAction{{Position: 1, Code: "REVIEW", Category: "PLACEMENT", State: "DRAFT", Title: "Review allocation", Detail: "Review before scheduling.", Timing: "Before scheduling", Executable: false}},
		CreatedAt:       time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC),
	}
	html, err := renderHTML(value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "does not approve, submit, schedule, notify, contact, or execute") {
		t.Fatal("HTML export omits the human-review boundary")
	}
	pdf, err := renderPDF(value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("PDF export is not a PDF document")
	}
}

func testSnapshot() Snapshot {
	plan := &placement.PlacementPlan{
		ID: "33333333-3333-3333-3333-333333333333", DecisionID: "22222222-2222-2222-2222-222222222222",
		Status: placement.StatusReady, Tier: "STANDARD", AllocatedDryTons: "2.205", UnallocatedDryTons: "0",
		Allocations: []placement.PlacementAllocation{{Position: 1, FieldID: "44444444-4444-4444-4444-444444444444", FieldName: "South field", DryTons: "2.205", Acres: "1.1025", Rate: "2"}},
	}
	return Snapshot{
		Decision:  policy.Decision{ID: "22222222-2222-2222-2222-222222222222", Tier: policy.TierStandard, BatchIdentifier: "BATCH-01", FacilityName: "Example WWTP", Explanation: "Below PFAS action thresholds.", RulePack: policy.RulePack{Version: "2024.4"}},
		Lab:       LabSnapshot{ReportID: "55555555-5555-5555-5555-555555555555", ReportVersion: 1, OriginalFilename: "report.pdf", SHA256: strings.Repeat("b", 64), Analytes: []LabAnalyte{}, Gaps: []PackageGap{}},
		Placement: plan, Gaps: []PackageGap{},
	}
}
