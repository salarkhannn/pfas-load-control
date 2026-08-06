package responseplan

import (
	"encoding/json"
	"time"

	"github.com/riverqueue/river"
)

const (
	Queue         = "response"
	configVersion = "module-06.1"
)

type BuildArgs struct {
	RunID string `json:"runId"`
}

func (BuildArgs) Kind() string { return "build_pfas_response" }

func (BuildArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{Queue: Queue} }

type LocationInput struct {
	Kind  string `json:"kind" enum:"address,coord"`
	Input string `json:"input" minLength:"1" maxLength:"256"`
}

type LocationCandidate struct {
	Label           string   `json:"label,omitempty"`
	ResolvedAddress string   `json:"resolvedAddress,omitempty"`
	Latitude        *float64 `json:"latitude,omitempty"`
	Longitude       *float64 `json:"longitude,omitempty"`
	Confidence      *float64 `json:"confidence,omitempty"`
}

type FacilityLocation struct {
	ID              string              `json:"id"`
	FacilityID      string              `json:"facilityId"`
	Input           string              `json:"input"`
	Kind            string              `json:"kind"`
	Disposition     string              `json:"disposition"`
	Latitude        *float64            `json:"latitude,omitempty"`
	Longitude       *float64            `json:"longitude,omitempty"`
	ResolvedAddress string              `json:"resolvedAddress,omitempty"`
	State           string              `json:"state,omitempty"`
	County          string              `json:"county,omitempty"`
	Confidence      *float64            `json:"confidence,omitempty"`
	Candidates      []LocationCandidate `json:"candidates"`
	Reason          string              `json:"reason,omitempty"`
	Hint            string              `json:"hint,omitempty"`
	SourceURL       string              `json:"sourceUrl"`
	FetchedAt       time.Time           `json:"fetchedAt"`
	Confirmed       bool                `json:"confirmed"`
}

type StartInput struct {
	FacilityLocationID string `json:"facilityLocationId" format:"uuid"`
}

type ResponseTask struct {
	Position int    `json:"position"`
	Code     string `json:"code"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Timing   string `json:"timing"`
	State    string `json:"state"`
}

type ResponseEvidence struct {
	Provider      string          `json:"provider"`
	Kind          string          `json:"kind"`
	Status        string          `json:"status"`
	Title         string          `json:"title"`
	Summary       string          `json:"summary"`
	Data          json.RawMessage `json:"data"`
	SourceURL     string          `json:"sourceUrl"`
	SourceVintage string          `json:"sourceVintage,omitempty"`
	FetchedAt     time.Time       `json:"fetchedAt"`
	Caveat        string          `json:"caveat"`
}

type InvestigationLead struct {
	Position      int      `json:"position"`
	RegistryID    string   `json:"registryId"`
	FacilityName  string   `json:"facilityName"`
	City          string   `json:"city,omitempty"`
	State         string   `json:"state,omitempty"`
	NAICSCodes    []string `json:"naicsCodes"`
	EvidenceTier  int      `json:"evidenceTier"`
	EvidenceLabel string   `json:"evidenceLabel"`
	Rationale     string   `json:"rationale"`
	Caveat        string   `json:"caveat"`
	SourceURL     string   `json:"sourceUrl"`
}

type AlternativeCandidate struct {
	Position               int      `json:"position"`
	WDSID                  string   `json:"wdsId"`
	FacilityName           string   `json:"facilityName"`
	FacilityType           string   `json:"facilityType"`
	Address                string   `json:"address"`
	City                   string   `json:"city"`
	County                 string   `json:"county"`
	Latitude               float64  `json:"latitude"`
	Longitude              float64  `json:"longitude"`
	DisposalAreaStatus     string   `json:"disposalAreaStatus"`
	StraightlineDistanceKM float64  `json:"straightlineDistanceKm"`
	RouteStatus            string   `json:"routeStatus"`
	DrivingDistanceKM      *float64 `json:"drivingDistanceKm,omitempty"`
	DurationMinutes        *float64 `json:"durationMinutes,omitempty"`
	RouteNote              string   `json:"routeNote,omitempty"`
	AcceptanceStatus       string   `json:"acceptanceStatus"`
	Executable             bool     `json:"executable"`
	SourceURL              string   `json:"sourceUrl"`
}

type ResponseDataGap struct {
	Code       string `json:"code"`
	Detail     string `json:"detail"`
	Resolution string `json:"resolution"`
	Critical   bool   `json:"critical"`
}

type ResponseRun struct {
	ID                 string                 `json:"id"`
	DecisionID         string                 `json:"decisionId"`
	FacilityLocationID string                 `json:"facilityLocationId"`
	FacilityName       string                 `json:"facilityName"`
	BatchIdentifier    string                 `json:"batchIdentifier"`
	Tier               string                 `json:"tier"`
	Status             string                 `json:"status"`
	PolicySourceURL    string                 `json:"policySourceUrl"`
	PolicyVersion      string                 `json:"policyVersion"`
	Location           FacilityLocation       `json:"location"`
	Tasks              []ResponseTask         `json:"tasks"`
	Evidence           []ResponseEvidence     `json:"evidence"`
	InvestigationLeads []InvestigationLead    `json:"investigationLeads"`
	Alternatives       []AlternativeCandidate `json:"alternatives"`
	DataGaps           []ResponseDataGap      `json:"dataGaps"`
	FailureCode        string                 `json:"failureCode,omitempty"`
	FailureDetail      string                 `json:"failureDetail,omitempty"`
	CreatedAt          time.Time              `json:"createdAt"`
	UpdatedAt          time.Time              `json:"updatedAt"`
}
