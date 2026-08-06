package placement

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusReady                  Status = "READY"
	StatusReviewRequired         Status = "REVIEW_REQUIRED"
	StatusInsufficientCapacity   Status = "INSUFFICIENT_CAPACITY"
	StatusLandApplicationBlocked Status = "LAND_APPLICATION_BLOCKED"
)

type Disposition string

const (
	DispositionEligible       Disposition = "ELIGIBLE"
	DispositionReviewRequired Disposition = "REVIEW_REQUIRED"
	DispositionIneligible     Disposition = "INELIGIBLE"
)

type Band string

const (
	BandLow      Band = "LOW"
	BandModerate Band = "MODERATE"
	BandHigh     Band = "HIGH"
	BandUnknown  Band = "UNKNOWN"
)

type FactInput struct {
	Name      string          `json:"name"`
	Label     string          `json:"label"`
	State     string          `json:"state"`
	Value     json.RawMessage `json:"value,omitempty"`
	Unit      string          `json:"unit,omitempty"`
	Source    string          `json:"source,omitempty"`
	SourceURL string          `json:"sourceUrl,omitempty"`
}

type FieldInput struct {
	ID                    string      `json:"id"`
	Name                  string      `json:"name"`
	Status                string      `json:"status"`
	RMPApproved           bool        `json:"rmpApproved"`
	UsableAcres           string      `json:"usableAcres,omitempty"`
	AgronomicRate         string      `json:"agronomicRate,omitempty"`
	PriorLoadingDryTons   string      `json:"priorLoadingDryTons,omitempty"`
	CropOrUse             string      `json:"cropOrUse,omitempty"`
	PhysicalEvaluationID  string      `json:"physicalEvaluationId,omitempty"`
	PhysicalStatus        string      `json:"physicalStatus,omitempty"`
	PhysicalCriticalGaps  int         `json:"physicalCriticalGaps"`
	PhysicalOtherGaps     int         `json:"physicalOtherGaps"`
	SupplementalAvailable bool        `json:"supplementalAvailable"`
	Facts                 []FactInput `json:"facts"`
}

type Input struct {
	Tier              string       `json:"tier"`
	PolicyRate        string       `json:"policyRate,omitempty"`
	WetMassKg         string       `json:"wetMassKg,omitempty"`
	PercentSolids     string       `json:"percentSolids,omitempty"`
	Fields            []FieldInput `json:"fields"`
	DecisionInputHash string       `json:"decisionInputHash"`
}

type PlacementComponent struct {
	FactName  string          `json:"factName"`
	Label     string          `json:"label"`
	State     string          `json:"state"`
	Value     json.RawMessage `json:"value,omitempty"`
	Unit      string          `json:"unit,omitempty"`
	Source    string          `json:"source,omitempty"`
	SourceURL string          `json:"sourceUrl,omitempty"`
}

type VulnerabilityCategory struct {
	Key           string               `json:"key"`
	Label         string               `json:"label"`
	Band          Band                 `json:"band"`
	Explanation   string               `json:"explanation"`
	Components    []PlacementComponent `json:"components"`
	AuthorityType string               `json:"authorityType"`
	SourceTitle   string               `json:"sourceTitle"`
	SourceURL     string               `json:"sourceUrl"`
	ConfigVersion string               `json:"configVersion"`
}

type PlacementField struct {
	FieldID              string                  `json:"fieldId"`
	FieldName            string                  `json:"fieldName"`
	Disposition          Disposition             `json:"disposition"`
	Rank                 *int                    `json:"rank,omitempty"`
	Explanation          string                  `json:"explanation"`
	Counterfactual       string                  `json:"counterfactual,omitempty"`
	HighConcernCount     int                     `json:"highConcernCount"`
	ModerateConcernCount int                     `json:"moderateConcernCount"`
	DataGapCount         int                     `json:"dataGapCount"`
	AllowedRate          string                  `json:"allowedRateDryTonsPerAcre,omitempty"`
	AvailableCapacity    string                  `json:"availableCapacityDryTons,omitempty"`
	RoadAccessDistanceM  *float64                `json:"roadAccessDistanceM,omitempty"`
	PhysicalEvaluationID string                  `json:"physicalEvaluationId,omitempty"`
	Reasons              []string                `json:"reasons"`
	Categories           []VulnerabilityCategory `json:"categories"`
}

type PlacementAllocation struct {
	Position  int    `json:"position"`
	FieldID   string `json:"fieldId"`
	FieldName string `json:"fieldName"`
	DryTons   string `json:"dryTons"`
	Acres     string `json:"acres"`
	Rate      string `json:"rateDryTonsPerAcre"`
}

type PlacementGap struct {
	Code       string `json:"code"`
	Detail     string `json:"detail"`
	Resolution string `json:"resolution"`
}

type PlacementPlan struct {
	ID                 string                `json:"id"`
	DecisionID         string                `json:"decisionId"`
	Status             Status                `json:"status"`
	Tier               string                `json:"tier"`
	ConfigVersion      string                `json:"configVersion"`
	ConfigChecksum     string                `json:"configChecksum"`
	InputHash          string                `json:"inputHash"`
	WetMassKg          string                `json:"wetMassKg,omitempty"`
	PercentSolids      string                `json:"percentSolids,omitempty"`
	BatchDryTons       string                `json:"batchDryTons,omitempty"`
	AllocatedDryTons   string                `json:"allocatedDryTons,omitempty"`
	UnallocatedDryTons string                `json:"unallocatedDryTons,omitempty"`
	Fields             []PlacementField      `json:"fields"`
	Allocations        []PlacementAllocation `json:"allocations"`
	Gaps               []PlacementGap        `json:"gaps"`
	CreatedAt          time.Time             `json:"createdAt"`
}

type PlanInput struct {
	WetMassKg     *string `json:"wetMassKg,omitempty" maxLength:"40"`
	PercentSolids *string `json:"percentSolids,omitempty" maxLength:"40"`
}
