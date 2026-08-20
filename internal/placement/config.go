package placement

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	ConfigVersion = "mi-fieldproof-placement-2026.08.5"

	DemoProgramPolicyVersion   = "DEMO-MI-LAND-APPLICATION-2026.08.1"
	PolygonSamplingAlgorithmV1 = "POLYGON_STRATIFIED_SCREEN_V1"
	DemoBoundaryIssuerRole     = "DEMO_FIELD_BOUNDARY_ISSUER"
	DemoPlacementReviewerRole  = "DEMO_PLACEMENT_REVIEWER"

	michiganBiosolidsURL = "https://www.michigan.gov/egle/faqs/water-quality-protection/land-application-of-biosolids"
	epaDraftGuidanceURL  = "https://www.epa.gov/system/files/documents/2026-06/draft-guidance-reducing-risk-pfoa-pfos-biosolids.pdf"
)

var ConfigChecksum = func() string {
	digest := sha256.Sum256([]byte(ConfigVersion + "|water-direct-overlap-review|wetland-100m|groundwater-0.762m-3m|surface-slope-6pct-review-unless-parent-bound-and-sampled-screen-resolution-v3|epa-2026-crop-guidance|uncertainty-v1|rank-all-fields-distinct|review-before-capacity|" + DemoProgramPolicyVersion + "|" + PolygonSamplingAlgorithmV1))
	return hex.EncodeToString(digest[:])
}()

type SamplingPolicy struct {
	AlgorithmVersion           string  `json:"algorithmVersion"`
	TargetSampleCount          int     `json:"targetSampleCount"`
	MaxRequestSamples          int     `json:"maxRequestSamples"`
	MinimumSpacingMeters       float64 `json:"minimumSpacingMeters"`
	BoundaryNearDistanceMeters float64 `json:"boundaryNearDistanceMeters"`
}

type ResolutionPolicy struct {
	Version                 string         `json:"version"`
	DemonstrationRoles      bool           `json:"demonstrationRoles"`
	AuthorizedIssuerRoles   []string       `json:"authorizedIssuerRoles"`
	AuthorizedReviewerRoles []string       `json:"authorizedReviewerRoles"`
	Sampling                SamplingPolicy `json:"sampling"`
}

func CurrentResolutionPolicy() ResolutionPolicy {
	return ResolutionPolicy{
		Version:                 DemoProgramPolicyVersion,
		DemonstrationRoles:      true,
		AuthorizedIssuerRoles:   []string{DemoBoundaryIssuerRole},
		AuthorizedReviewerRoles: []string{DemoPlacementReviewerRole},
		Sampling: SamplingPolicy{
			AlgorithmVersion:           PolygonSamplingAlgorithmV1,
			TargetSampleCount:          5,
			MaxRequestSamples:          8,
			MinimumSpacingMeters:       75,
			BoundaryNearDistanceMeters: 120,
		},
	}
}
