package placement

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	ConfigVersion = "mi-pfas-field-comparison-2026.08.1"

	michiganBiosolidsURL = "https://www.michigan.gov/egle/faqs/water-quality-protection/land-application-of-biosolids"
	epaDraftGuidanceURL  = "https://www.epa.gov/system/files/documents/2026-06/draft-guidance-reducing-risk-pfoa-pfos-biosolids.pdf"
)

var ConfigChecksum = func() string {
	digest := sha256.Sum256([]byte(ConfigVersion + "|water-direct-overlap-review|wetland-100m|groundwater-0.762m-3m|surface-slope-6pct|epa-2026-crop-guidance|uncertainty-v1|rank-high-moderate-gaps-capacity-access|review-before-capacity"))
	return hex.EncodeToString(digest[:])
}()
