package decisionpackage

import (
	"encoding/json"
	"time"

	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
	"github.com/salarkhannn/pfas-load-control/internal/policy"
	"github.com/salarkhannn/pfas-load-control/internal/responseplan"
)

const SchemaVersion = "module-07.1"

type LabSnapshot struct {
	ReportID         string       `json:"reportId"`
	ReportVersion    int          `json:"reportVersion"`
	OriginalFilename string       `json:"originalFilename"`
	MediaType        string       `json:"mediaType"`
	SHA256           string       `json:"sha256"`
	Laboratory       *string      `json:"laboratory,omitempty"`
	SampleIdentifier *string      `json:"sampleIdentifier,omitempty"`
	CollectionDate   *string      `json:"collectionDate,omitempty"`
	Matrix           *string      `json:"matrix,omitempty"`
	Method           *string      `json:"method,omitempty"`
	Basis            *string      `json:"basis,omitempty"`
	Analytes         []LabAnalyte `json:"analytes"`
	Gaps             []PackageGap `json:"gaps"`
	ConfirmedAt      *time.Time   `json:"confirmedAt,omitempty"`
}

type LabAnalyte struct {
	Name          string  `json:"name"`
	ResultText    string  `json:"resultText"`
	ValueUGKGDry  *string `json:"valueUgKgDry,omitempty"`
	UpperBound    *string `json:"upperBoundUgKgDry,omitempty"`
	IsNonDetect   bool    `json:"isNonDetect"`
	SourcePage    int     `json:"sourcePage"`
	SourceExcerpt string  `json:"sourceExcerpt"`
}

type Snapshot struct {
	Decision         policy.Decision           `json:"decision"`
	Lab              LabSnapshot               `json:"lab"`
	Placement        *placement.PlacementPlan  `json:"placement,omitempty"`
	PhysicalEvidence []evidence.Evaluation     `json:"physicalEvidence"`
	Response         *responseplan.ResponseRun `json:"response,omitempty"`
	Gaps             []PackageGap              `json:"gaps"`
}

type PackageGap struct {
	Source     string `json:"source"`
	Code       string `json:"code"`
	Detail     string `json:"detail"`
	Resolution string `json:"resolution"`
	Critical   bool   `json:"critical"`
}

type EvidenceEntry struct {
	Position      int             `json:"position"`
	Kind          string          `json:"kind"`
	Provider      string          `json:"provider"`
	Title         string          `json:"title"`
	Status        string          `json:"status"`
	RecordID      string          `json:"recordId,omitempty"`
	SourceURL     string          `json:"sourceUrl,omitempty"`
	SourceVersion string          `json:"sourceVersion,omitempty"`
	SourceHash    string          `json:"sourceHash,omitempty"`
	RetrievedAt   *time.Time      `json:"retrievedAt,omitempty"`
	Detail        string          `json:"detail"`
	Caveat        string          `json:"caveat,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
}

type ProposedAction struct {
	Position   int    `json:"position"`
	Code       string `json:"code"`
	Category   string `json:"category"`
	State      string `json:"state"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Timing     string `json:"timing"`
	SourceID   string `json:"sourceId"`
	Executable bool   `json:"executable"`
}

type Artifact struct {
	Format    string `json:"format"`
	MediaType string `json:"mediaType"`
	SHA256    string `json:"sha256"`
	SizeBytes int    `json:"sizeBytes"`
	URL       string `json:"url"`
}

type DecisionPackage struct {
	ID              string           `json:"id"`
	DecisionID      string           `json:"decisionId"`
	SchemaVersion   string           `json:"schemaVersion"`
	Status          string           `json:"status"`
	InputHash       string           `json:"inputHash"`
	Snapshot        Snapshot         `json:"snapshot"`
	Evidence        []EvidenceEntry  `json:"evidence"`
	ProposedActions []ProposedAction `json:"proposedActions"`
	Artifacts       []Artifact       `json:"artifacts"`
	CreatedAt       time.Time        `json:"createdAt"`
}

type ArtifactContent struct {
	Filename  string
	MediaType string
	Content   []byte
}
