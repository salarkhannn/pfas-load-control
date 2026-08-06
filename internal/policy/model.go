package policy

import "time"

type Tier string

const (
	TierStandard     Tier = "STANDARD"
	TierElevated     Tier = "ELEVATED"
	TierProhibited   Tier = "PROHIBITED"
	TierUndetermined Tier = "UNDETERMINED"
)

type Rule struct {
	ID                                   string   `json:"id"`
	Tier                                 Tier     `json:"tier"`
	Operator                             string   `json:"operator"`
	ThresholdUGKGDry                     string   `json:"thresholdUgKgDry"`
	MaximumApplicationRateDryTonsPerAcre *string  `json:"maximumApplicationRateDryTonsPerAcre,omitempty"`
	ProhibitedActions                    []string `json:"prohibitedActions,omitempty"`
	Explanation                          string   `json:"explanation"`
}

type RequirementDefinition struct {
	ID     string `json:"id"`
	Tiers  []Tier `json:"tiers"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Timing string `json:"timing"`
}

type RulePack struct {
	SchemaVersion                  int                     `json:"schemaVersion"`
	Code                           string                  `json:"code"`
	Version                        string                  `json:"version"`
	Jurisdiction                   string                  `json:"jurisdiction"`
	AuthorityType                  string                  `json:"authorityType"`
	EffectiveFrom                  string                  `json:"effectiveFrom"`
	SourceURL                      string                  `json:"sourceUrl"`
	SourceTitle                    string                  `json:"sourceTitle"`
	RetrievedAt                    time.Time               `json:"retrievedAt"`
	ReviewedAt                     time.Time               `json:"reviewedAt"`
	ReviewedBy                     string                  `json:"reviewedBy"`
	ReviewStatus                   string                  `json:"reviewStatus"`
	Explanation                    string                  `json:"explanation"`
	ElevatedThresholdUGKGDry       string                  `json:"elevatedThresholdUgKgDry"`
	ProhibitedThresholdUGKGDry     string                  `json:"prohibitedThresholdUgKgDry"`
	AcceptedAnalyticalMethodTokens []string                `json:"acceptedAnalyticalMethodTokens"`
	Rules                          []Rule                  `json:"rules"`
	Requirements                   []RequirementDefinition `json:"requirements"`
	Checksum                       string                  `json:"checksum"`
}

type AnalyteEvidence struct {
	CanonicalAnalyte       string  `json:"canonicalAnalyte"`
	ResultText             string  `json:"resultText"`
	IsNonDetect            bool    `json:"isNonDetect"`
	NormalizedValueUGKGDry *string `json:"normalizedValueUgKgDry,omitempty"`
	UpperBoundUGKGDry      *string `json:"upperBoundUgKgDry,omitempty"`
	SourcePage             int     `json:"sourcePage"`
}

type ClassificationInput struct {
	Jurisdiction string
	Matrix       *string
	Method       *string
	Basis        *string
	Analytes     []AnalyteEvidence
}

type Requirement struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Detail        string `json:"detail"`
	Timing        string `json:"timing"`
	RuleID        string `json:"ruleId"`
	SourceURL     string `json:"sourceUrl"`
	SourceTitle   string `json:"sourceTitle"`
	AuthorityType string `json:"authorityType"`
}

type Evaluation struct {
	Tier                                 Tier
	MatchedRuleID                        string
	Explanation                          string
	Requirements                         []Requirement
	MaximumApplicationRateDryTonsPerAcre *string
	ProhibitedActions                    []string
	BlockingReason                       *string
}

type Decision struct {
	ID                                   string            `json:"id"`
	ReportID                             string            `json:"reportId"`
	BatchID                              string            `json:"batchId"`
	ReportVersion                        int               `json:"reportVersion"`
	FacilityName                         string            `json:"facilityName"`
	BatchIdentifier                      string            `json:"batchIdentifier"`
	WetMassKg                            string            `json:"wetMassKg,omitempty"`
	PercentSolids                        string            `json:"percentSolids,omitempty"`
	Jurisdiction                         string            `json:"jurisdiction"`
	Tier                                 Tier              `json:"tier"`
	Explanation                          string            `json:"explanation"`
	MatchedRuleID                        *string           `json:"matchedRuleId,omitempty"`
	BlockingReason                       *string           `json:"blockingReason,omitempty"`
	Analytes                             []AnalyteEvidence `json:"analytes"`
	Requirements                         []Requirement     `json:"requirements"`
	RulePack                             RulePack          `json:"rulePack"`
	MaximumApplicationRateDryTonsPerAcre *string           `json:"maximumApplicationRateDryTonsPerAcre,omitempty"`
	ProhibitedActions                    []string          `json:"prohibitedActions"`
	InputHash                            string            `json:"inputHash"`
	CreatedAt                            time.Time         `json:"createdAt"`
}
