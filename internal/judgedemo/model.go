package judgedemo

import (
	"encoding/json"
	"time"

	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
)

type DemoRun struct {
	ID                    string                  `json:"id"`
	FixtureVersion        string                  `json:"fixtureVersion"`
	Mode                  string                  `json:"mode"`
	ParentRunID           string                  `json:"parentRunId,omitempty"`
	Kind                  string                  `json:"kind"`
	RunStatus             string                  `json:"runStatus"`
	CalculationStatus     string                  `json:"calculationStatus"`
	AuthorizationStatus   string                  `json:"authorizationStatus"`
	AuthorizationRequired bool                    `json:"authorizationRequired"`
	CaseID                string                  `json:"caseId"`
	BatchDryTons          string                  `json:"batchDryTons"`
	Before                placement.PlacementPlan `json:"before"`
	After                 placement.PlacementPlan `json:"after"`
	ExcludedAcres         string                  `json:"excludedAcres"`
	ReviewRequired        bool                    `json:"reviewRequired"`
	ReviewQuestion        string                  `json:"reviewQuestion"`
	PhysicalEvidence      []evidence.FieldFact    `json:"physicalEvidence"`
	MireyeCapture         DemoMireyeCapture       `json:"mireyeCapture"`
	AcreageAdjustment     DemoAcreageAdjustment   `json:"acreageAdjustment"`
	ResolutionEvidence    *DemoResolutionEvidence `json:"resolutionEvidence,omitempty"`
	ToolCalls             []DemoToolCall          `json:"toolCalls"`
	FreezeReceipt         DemoFreezeReceipt       `json:"freezeReceipt"`
	Citations             []DemoCitation          `json:"citations"`
	Package               DemoPackage             `json:"package"`
	CreatedAt             time.Time               `json:"createdAt"`
	CompletedAt           time.Time               `json:"completedAt"`
}

type DemoResolutionEvidence struct {
	FixtureVersion                string                           `json:"fixtureVersion"`
	Label                         string                           `json:"label"`
	RecordID                      string                           `json:"recordId"`
	EvidenceType                  string                           `json:"evidenceType"`
	ArtifactHash                  string                           `json:"artifactHash"`
	Artifact                      json.RawMessage                  `json:"artifact"`
	ReviewerAuthorizationRecordID string                           `json:"reviewerAuthorizationRecordId"`
	ReviewerAuthorizationHash     string                           `json:"reviewerAuthorizationHash"`
	ReviewerAuthorizationArtifact json.RawMessage                  `json:"reviewerAuthorizationArtifact"`
	ParentBoundaryRecordID        string                           `json:"parentBoundaryRecordId"`
	ParentBoundaryArtifactHash    string                           `json:"parentBoundaryArtifactHash"`
	ParentBoundaryArtifact        json.RawMessage                  `json:"parentBoundaryArtifact"`
	RevisedScreeningRecordID      string                           `json:"revisedScreeningRecordId"`
	RevisedScreeningArtifactHash  string                           `json:"revisedScreeningArtifactHash"`
	RevisedScreeningArtifact      json.RawMessage                  `json:"revisedScreeningArtifact"`
	Verification                  placement.ResolutionVerification `json:"verification"`
}

type DemoFreezeReceipt struct {
	Position     int       `json:"position"`
	ToolName     string    `json:"toolName"`
	Status       string    `json:"status"`
	ArtifactID   string    `json:"artifactId"`
	ArtifactHash string    `json:"artifactHash"`
	StartedAt    time.Time `json:"startedAt"`
	CompletedAt  time.Time `json:"completedAt"`
}

type DemoToolCall struct {
	Position    int             `json:"position"`
	ToolName    string          `json:"toolName"`
	Status      string          `json:"status"`
	Summary     string          `json:"summary"`
	SourceURL   string          `json:"sourceUrl"`
	RequestID   string          `json:"requestId"`
	InputHash   string          `json:"inputHash"`
	OutputHash  string          `json:"outputHash"`
	Input       json.RawMessage `json:"input"`
	Output      json.RawMessage `json:"output,omitempty"`
	Error       string          `json:"error,omitempty"`
	StartedAt   time.Time       `json:"startedAt"`
	CompletedAt time.Time       `json:"completedAt"`
}

type DemoMireyeCapture struct {
	FixtureVersion string          `json:"fixtureVersion"`
	Endpoint       string          `json:"endpoint"`
	RequestID      string          `json:"requestId"`
	HTTPStatus     int             `json:"httpStatus"`
	RetrievedAt    time.Time       `json:"retrievedAt"`
	RequestHash    string          `json:"requestHash"`
	ResponseHash   string          `json:"responseHash"`
	Request        json.RawMessage `json:"request"`
	Response       json.RawMessage `json:"response"`
}

type DemoAcreageAdjustment struct {
	FixtureVersion        string          `json:"fixtureVersion"`
	InputType             string          `json:"inputType"`
	BoundaryVersion       int             `json:"boundaryVersion"`
	RecordedBoundaryAcres string          `json:"recordedBoundaryAcres"`
	ExcludedAcres         string          `json:"excludedAcres"`
	EffectiveAcres        string          `json:"effectiveAcres"`
	RecordedAt            time.Time       `json:"recordedAt"`
	Source                string          `json:"source"`
	Reason                string          `json:"reason"`
	InputHash             string          `json:"inputHash"`
	RawFixture            json.RawMessage `json:"rawFixture"`
}

type DemoCitation struct {
	ID          string    `json:"id"`
	Finding     string    `json:"finding"`
	Value       string    `json:"value"`
	Source      string    `json:"source"`
	SourceURL   string    `json:"sourceUrl"`
	RetrievedAt time.Time `json:"retrievedAt"`
	Effect      string    `json:"effect"`
}

type DemoPackage struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	InputHash    string    `json:"inputHash"`
	DecisionHash string    `json:"decisionHash"`
	PayloadHash  string    `json:"payloadHash"`
	Artifact     []byte    `json:"-"`
	DownloadURL  string    `json:"downloadUrl"`
	CreatedAt    time.Time `json:"createdAt"`
}
