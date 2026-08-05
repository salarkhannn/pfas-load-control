package field

import (
	"encoding/json"
	"time"
)

const (
	MaxImportRows = 25
	MaxCSVBytes   = 1 << 20
)

type Status string

const (
	StatusNeedsLocation Status = "NEEDS_LOCATION"
	StatusNeedsGeometry Status = "NEEDS_GEOMETRY"
	StatusNeedsDetails  Status = "NEEDS_DETAILS"
	StatusReady         Status = "READY"
)

type LocatorKind string

const (
	LocatorAddress    LocatorKind = "ADDRESS"
	LocatorCoordinate LocatorKind = "COORDINATE"
	LocatorAPN        LocatorKind = "APN"
	LocatorGeoJSON    LocatorKind = "GEOJSON"
)

type FieldFacility struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Jurisdiction string `json:"jurisdiction"`
}

type Candidate struct {
	Label           string   `json:"label,omitempty"`
	ResolvedAddress string   `json:"resolvedAddress,omitempty"`
	Latitude        *float64 `json:"latitude,omitempty"`
	Longitude       *float64 `json:"longitude,omitempty"`
	Confidence      *float64 `json:"confidence,omitempty"`
	MatchMethod     string   `json:"matchMethod,omitempty"`
}

type Parcel struct {
	ID             string          `json:"id,omitempty"`
	APN            string          `json:"apn,omitempty"`
	Geometry       json.RawMessage `json:"geometry,omitempty"`
	MatchType      string          `json:"matchType,omitempty"`
	MatchDistanceM *string         `json:"matchDistanceM,omitempty"`
	Source         string          `json:"source,omitempty"`
}

type Location struct {
	ID                string      `json:"id"`
	Disposition       string      `json:"disposition"`
	Latitude          *string     `json:"latitude,omitempty"`
	Longitude         *string     `json:"longitude,omitempty"`
	ResolvedAddress   string      `json:"resolvedAddress,omitempty"`
	State             string      `json:"state,omitempty"`
	County            string      `json:"county,omitempty"`
	FIPS              string      `json:"fips,omitempty"`
	Confidence        *string     `json:"confidence,omitempty"`
	MatchMethod       string      `json:"matchMethod,omitempty"`
	Parcel            *Parcel     `json:"parcel,omitempty"`
	ParcelUnavailable bool        `json:"parcelUnavailable"`
	Candidates        []Candidate `json:"candidates"`
	Reason            string      `json:"reason,omitempty"`
	Hint              string      `json:"hint,omitempty"`
	RequestID         string      `json:"requestId,omitempty"`
	SourceURL         string      `json:"sourceUrl"`
	ResponseHash      string      `json:"responseHash"`
	FetchedAt         time.Time   `json:"fetchedAt"`
}

type Geometry struct {
	Version     int             `json:"version"`
	Source      string          `json:"source"`
	GeoJSON     json.RawMessage `json:"geojson"`
	AreaAcres   string          `json:"areaAcres"`
	Hash        string          `json:"hash"`
	ConfirmedAt time.Time       `json:"confirmedAt"`
}

type FieldGap struct {
	ID         string    `json:"id"`
	Code       string    `json:"code"`
	Detail     string    `json:"detail"`
	Resolution string    `json:"resolution"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Details struct {
	MiEnviroSiteID              *string `json:"miEnviroSiteId,omitempty"`
	RMPApproved                 *bool   `json:"rmpApproved,omitempty"`
	RMPDocumentReference        *string `json:"rmpDocumentReference,omitempty"`
	UsableAcres                 *string `json:"usableAcres,omitempty"`
	CropOrUse                   *string `json:"cropOrUse,omitempty"`
	AgronomicRateDryTonsPerAcre *string `json:"agronomicRateDryTonsPerAcre,omitempty"`
	PriorLoadingDryTons         *string `json:"priorLoadingDryTons,omitempty"`
	KnownConstraints            *string `json:"knownConstraints,omitempty"`
	AccessConstraints           *string `json:"accessConstraints,omitempty"`
}

type Field struct {
	ID           string        `json:"id"`
	Facility     FieldFacility `json:"facility"`
	Name         string        `json:"name"`
	LocatorKind  LocatorKind   `json:"locatorKind"`
	LocatorInput string        `json:"locatorInput"`
	Status       Status        `json:"status"`
	Details      Details       `json:"details"`
	Location     *Location     `json:"location,omitempty"`
	Geometry     *Geometry     `json:"geometry,omitempty"`
	Gaps         []FieldGap    `json:"gaps"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
}

type FieldContext struct {
	Facilities []FieldFacility `json:"facilities"`
	Fields     []Field         `json:"fields"`
}

type CreateInput struct {
	Name        string      `json:"name" minLength:"1" maxLength:"160"`
	LocatorKind LocatorKind `json:"locatorKind" enum:"ADDRESS,COORDINATE,APN,GEOJSON"`
	Locator     string      `json:"locator,omitempty" maxLength:"256"`
	County      string      `json:"county,omitempty" maxLength:"120"`
	GeoJSON     string      `json:"geojson,omitempty" maxLength:"1048576"`
}

type DetailsInput struct {
	MiEnviroSiteID              *string `json:"miEnviroSiteId,omitempty" maxLength:"120"`
	RMPApproved                 *bool   `json:"rmpApproved,omitempty"`
	RMPDocumentReference        *string `json:"rmpDocumentReference,omitempty" maxLength:"500"`
	UsableAcres                 *string `json:"usableAcres,omitempty" maxLength:"40"`
	CropOrUse                   *string `json:"cropOrUse,omitempty" maxLength:"240"`
	AgronomicRateDryTonsPerAcre *string `json:"agronomicRateDryTonsPerAcre,omitempty" maxLength:"40"`
	PriorLoadingDryTons         *string `json:"priorLoadingDryTons,omitempty" maxLength:"40"`
	KnownConstraints            *string `json:"knownConstraints,omitempty" maxLength:"2000"`
	AccessConstraints           *string `json:"accessConstraints,omitempty" maxLength:"2000"`
}

type CSVResult struct {
	Row     int    `json:"row"`
	Field   *Field `json:"field,omitempty"`
	Problem string `json:"problem,omitempty"`
}

type Import struct {
	Results []CSVResult `json:"results"`
}
