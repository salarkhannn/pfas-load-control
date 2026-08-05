package evidence

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusQueued         Status = "QUEUED"
	StatusRunning        Status = "RUNNING"
	StatusSucceeded      Status = "SUCCEEDED"
	StatusReviewRequired Status = "REVIEW_REQUIRED"
	StatusFailed         Status = "FAILED"
)

type Evaluation struct {
	ID                 string                 `json:"id"`
	FieldID            string                 `json:"fieldId"`
	GeometryVersion    int                    `json:"geometryVersion"`
	Status             Status                 `json:"status"`
	FieldSetVersion    string                 `json:"fieldSetVersion"`
	AggregationVersion string                 `json:"aggregationVersion"`
	CatalogVersion     string                 `json:"catalogVersion,omitempty"`
	SampleCount        int                    `json:"sampleCount"`
	ProjectedCredits   int                    `json:"projectedCredits"`
	FailureCode        string                 `json:"failureCode,omitempty"`
	FailureDetail      string                 `json:"failureDetail,omitempty"`
	Samples            []SamplePoint          `json:"samples"`
	Facts              []FieldFact            `json:"facts"`
	Supplemental       []SupplementalEvidence `json:"supplemental"`
	Gaps               []PhysicalDataGap      `json:"gaps"`
	StartedAt          *time.Time             `json:"startedAt,omitempty"`
	CompletedAt        *time.Time             `json:"completedAt,omitempty"`
	CreatedAt          time.Time              `json:"createdAt"`
	UpdatedAt          time.Time              `json:"updatedAt"`
}

type SamplePoint struct {
	Index     int     `json:"index"`
	Label     string  `json:"label"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type FieldFact struct {
	Name            string          `json:"name"`
	Label           string          `json:"label"`
	Category        string          `json:"category"`
	State           string          `json:"state"`
	AggregateMethod string          `json:"aggregateMethod"`
	Value           json.RawMessage `json:"value,omitempty"`
	Unit            string          `json:"unit,omitempty"`
	Source          string          `json:"source,omitempty"`
	SourceURL       string          `json:"sourceUrl,omitempty"`
	FetchedAt       *time.Time      `json:"fetchedAt,omitempty"`
	OKCount         int             `json:"okCount"`
	AbsentCount     int             `json:"absentCount"`
	FailedCount     int             `json:"failedCount"`
	Critical        bool            `json:"critical"`
	Samples         []SampleFact    `json:"samples"`
}

type SampleFact struct {
	Index          int             `json:"index"`
	Label          string          `json:"label"`
	Latitude       float64         `json:"latitude"`
	Longitude      float64         `json:"longitude"`
	Status         string          `json:"status"`
	Value          json.RawMessage `json:"value,omitempty"`
	Unit           string          `json:"unit,omitempty"`
	Source         string          `json:"source,omitempty"`
	SourceURL      string          `json:"sourceUrl,omitempty"`
	Confidence     string          `json:"confidence,omitempty"`
	FetchedAt      *time.Time      `json:"fetchedAt,omitempty"`
	DatasetVintage string          `json:"datasetVintage,omitempty"`
	Notes          string          `json:"notes,omitempty"`
	Error          string          `json:"error,omitempty"`
}

type SupplementalEvidence struct {
	Provider      string          `json:"provider"`
	Kind          string          `json:"kind"`
	Status        string          `json:"status"`
	Title         string          `json:"title"`
	Summary       string          `json:"summary"`
	Value         json.RawMessage `json:"value,omitempty"`
	SourceURL     string          `json:"sourceUrl"`
	SourceVintage string          `json:"sourceVintage,omitempty"`
	FetchedAt     time.Time       `json:"fetchedAt"`
	Caveat        string          `json:"caveat,omitempty"`
}

type PhysicalDataGap struct {
	Code     string `json:"code"`
	Source   string `json:"source"`
	Detail   string `json:"detail"`
	Critical bool   `json:"critical"`
}
