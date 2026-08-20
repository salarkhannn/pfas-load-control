package lab

import "time"

const (
	MaxReportBytes = 10 * 1024 * 1024
	MaxPDFPages    = 15
)

type ReportStatus string

const (
	StatusUploaded       ReportStatus = "UPLOADED"
	StatusProcessing     ReportStatus = "PROCESSING"
	StatusNeedsReview    ReportStatus = "NEEDS_REVIEW"
	StatusReadyToConfirm ReportStatus = "READY_TO_CONFIRM"
	StatusConfirmed      ReportStatus = "CONFIRMED"
	StatusFailed         ReportStatus = "FAILED"
)

type Facility struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Jurisdiction string `json:"jurisdiction"`
}

type Batch struct {
	ID            string  `json:"id"`
	Identifier    string  `json:"identifier"`
	WetMassKG     *string `json:"wetMassKg,omitempty"`
	PercentSolids *string `json:"percentSolids,omitempty"`
	FacilityID    string  `json:"facilityId"`
	FacilityName  string  `json:"facilityName"`
	Jurisdiction  string  `json:"jurisdiction"`
}

type SourceBounds struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type Page struct {
	Number           int     `json:"number"`
	Text             string  `json:"text"`
	ExtractionMethod string  `json:"extractionMethod"`
	ReadError        string  `json:"readError,omitempty"`
	Width            *string `json:"width,omitempty"`
	Height           *string `json:"height,omitempty"`
}

type Analyte struct {
	ID                              string        `json:"id,omitempty"`
	CanonicalAnalyte                string        `json:"canonicalAnalyte"`
	ReportedAnalyte                 string        `json:"reportedAnalyte"`
	ResultText                      string        `json:"resultText"`
	Value                           *string       `json:"value,omitempty"`
	Unit                            *string       `json:"unit,omitempty"`
	Basis                           *string       `json:"basis,omitempty"`
	Qualifier                       *string       `json:"qualifier,omitempty"`
	IsNonDetect                     bool          `json:"isNonDetect"`
	ReportingLimit                  *string       `json:"reportingLimit,omitempty"`
	DetectionLimit                  *string       `json:"detectionLimit,omitempty"`
	NormalizedValueUGKGDry          *string       `json:"normalizedValueUgKgDry,omitempty"`
	NormalizedReportingLimitUGKGDry *string       `json:"normalizedReportingLimitUgKgDry,omitempty"`
	NormalizedDetectionLimitUGKGDry *string       `json:"normalizedDetectionLimitUgKgDry,omitempty"`
	SourcePage                      int           `json:"sourcePage"`
	SourceExcerpt                   string        `json:"sourceExcerpt"`
	SourceBounds                    *SourceBounds `json:"sourceBounds,omitempty"`
}

type Draft struct {
	ID               string     `json:"id,omitempty"`
	Version          int        `json:"version"`
	Status           string     `json:"status"`
	Source           string     `json:"source"`
	Laboratory       *string    `json:"laboratory,omitempty"`
	SampleIdentifier *string    `json:"sampleIdentifier,omitempty"`
	CollectionDate   *string    `json:"collectionDate,omitempty"`
	Matrix           *string    `json:"matrix,omitempty"`
	Method           *string    `json:"method,omitempty"`
	Basis            *string    `json:"basis,omitempty"`
	Analytes         []Analyte  `json:"analytes"`
	CreatedAt        time.Time  `json:"createdAt,omitempty"`
	ConfirmedAt      *time.Time `json:"confirmedAt,omitempty"`
}

type Gap struct {
	ID         string     `json:"id,omitempty"`
	Code       string     `json:"code"`
	FieldName  string     `json:"fieldName"`
	Detail     string     `json:"detail"`
	Resolution string     `json:"resolution"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt,omitempty"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

type Report struct {
	ID               string       `json:"id"`
	Status           ReportStatus `json:"status"`
	OriginalFilename string       `json:"originalFilename"`
	MediaType        string       `json:"mediaType"`
	SizeBytes        int          `json:"sizeBytes"`
	SHA256           string       `json:"sha256"`
	Facility         Facility     `json:"facility"`
	Batch            Batch        `json:"batch"`
	Draft            *Draft       `json:"draft,omitempty"`
	Pages            []Page       `json:"pages"`
	Gaps             []Gap        `json:"gaps"`
	FailureCode      *string      `json:"failureCode,omitempty"`
	CreatedAt        time.Time    `json:"createdAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
	ConfirmedAt      *time.Time   `json:"confirmedAt,omitempty"`
}

type Intake struct {
	FacilityName  string
	BatchID       string
	WetMassKG     *string
	PercentSolids *string
	Filename      string
	MediaType     string
	Content       []byte
}

type Extraction struct {
	Pages []Page
	Draft Draft
	Gaps  []Gap
}

type Correction struct {
	Laboratory       *string   `json:"laboratory,omitempty"`
	SampleIdentifier *string   `json:"sampleIdentifier,omitempty"`
	CollectionDate   *string   `json:"collectionDate,omitempty"`
	Matrix           *string   `json:"matrix,omitempty"`
	Method           *string   `json:"method,omitempty"`
	Basis            *string   `json:"basis,omitempty"`
	Analytes         []Analyte `json:"analytes" minItems:"2" maxItems:"2"`
}

type Context struct {
	Facilities []Facility `json:"facilities"`
	Batches    []Batch    `json:"batches"`
}
