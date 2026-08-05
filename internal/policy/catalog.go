package policy

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

//go:embed rules/*.json
var ruleFiles embed.FS

func LoadCatalog() ([]RulePack, error) {
	entries, err := ruleFiles.ReadDir("rules")
	if err != nil {
		return nil, fmt.Errorf("read embedded rule packs: %w", err)
	}
	packs := make([]RulePack, 0, len(entries))
	for _, entry := range entries {
		content, err := ruleFiles.ReadFile("rules/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read rule pack %s: %w", entry.Name(), err)
		}
		var pack RulePack
		decoder := json.NewDecoder(strings.NewReader(string(content)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&pack); err != nil {
			return nil, fmt.Errorf("decode rule pack %s: %w", entry.Name(), err)
		}
		if err := validateRulePack(pack); err != nil {
			return nil, fmt.Errorf("validate rule pack %s: %w", entry.Name(), err)
		}
		canonical, err := json.Marshal(pack)
		if err != nil {
			return nil, fmt.Errorf("canonicalize rule pack %s: %w", entry.Name(), err)
		}
		digest := sha256.Sum256(canonical)
		pack.Checksum = hex.EncodeToString(digest[:])
		packs = append(packs, pack)
	}
	return packs, nil
}

func validateRulePack(pack RulePack) error {
	if pack.SchemaVersion != 1 || pack.Code == "" || pack.Version == "" || pack.Jurisdiction == "" {
		return fmt.Errorf("identity and schema version are required")
	}
	if pack.ReviewStatus != "ACTIVE" || pack.ReviewedBy == "" || pack.ReviewedAt.IsZero() {
		return fmt.Errorf("active rule packs require an explicit review")
	}
	if pack.AuthorityType == "DRAFT_GUIDANCE" {
		return fmt.Errorf("draft guidance cannot be an active compliance rule pack")
	}
	if pack.RetrievedAt.IsZero() {
		return fmt.Errorf("retrievedAt is required")
	}
	parsedSource, err := url.Parse(pack.SourceURL)
	if err != nil || parsedSource.Scheme != "https" || parsedSource.Host == "" {
		return fmt.Errorf("sourceUrl must be an absolute HTTPS URL")
	}
	if _, err := time.Parse("2006-01-02", pack.EffectiveFrom); err != nil {
		return fmt.Errorf("effectiveFrom must be an ISO date")
	}
	elevated, err := decimal(pack.ElevatedThresholdUGKGDry)
	if err != nil {
		return fmt.Errorf("invalid elevated threshold: %w", err)
	}
	if elevated.Sign() < 0 {
		return fmt.Errorf("elevated threshold cannot be negative")
	}
	prohibited, err := decimal(pack.ProhibitedThresholdUGKGDry)
	if err != nil {
		return fmt.Errorf("invalid prohibited threshold: %w", err)
	}
	if prohibited.Sign() < 0 {
		return fmt.Errorf("prohibited threshold cannot be negative")
	}
	if elevated.Cmp(prohibited) >= 0 {
		return fmt.Errorf("elevated threshold must be below prohibited threshold")
	}
	if len(pack.AcceptedAnalyticalMethodTokens) == 0 {
		return fmt.Errorf("accepted analytical method tokens are required")
	}
	for _, token := range pack.AcceptedAnalyticalMethodTokens {
		if strings.TrimSpace(token) == "" {
			return fmt.Errorf("accepted analytical method tokens cannot be blank")
		}
	}
	if len(pack.Rules) != 3 || len(pack.Requirements) == 0 {
		return fmt.Errorf("classification rules and requirements are required")
	}
	expectedRules := map[string]struct {
		tier      Tier
		operator  string
		threshold string
	}{
		"MI-PFAS-PROHIBIT-100":      {TierProhibited, "ANY_GTE", pack.ProhibitedThresholdUGKGDry},
		"MI-PFAS-ELEVATED-20":       {TierElevated, "ANY_GTE", pack.ElevatedThresholdUGKGDry},
		"MI-PFAS-STANDARD-BELOW-20": {TierStandard, "ALL_LT", pack.ElevatedThresholdUGKGDry},
	}
	seenRules := make(map[string]bool, len(pack.Rules))
	for _, rule := range pack.Rules {
		expected, ok := expectedRules[rule.ID]
		if !ok || seenRules[rule.ID] {
			return fmt.Errorf("rule %q is unknown or duplicated", rule.ID)
		}
		seenRules[rule.ID] = true
		if rule.Tier != expected.tier || rule.Operator != expected.operator || rule.ThresholdUGKGDry != expected.threshold || strings.TrimSpace(rule.Explanation) == "" {
			return fmt.Errorf("rule %q does not match the deterministic classifier contract", rule.ID)
		}
		if rule.Tier == TierElevated {
			if rule.MaximumApplicationRateDryTonsPerAcre == nil {
				return fmt.Errorf("elevated rule requires a maximum application rate")
			}
			rate, rateErr := decimal(*rule.MaximumApplicationRateDryTonsPerAcre)
			if rateErr != nil || rate.Sign() <= 0 {
				return fmt.Errorf("elevated rule contains an invalid maximum application rate")
			}
		}
		if rule.Tier == TierProhibited && !containsString(rule.ProhibitedActions, "LAND_APPLICATION") {
			return fmt.Errorf("prohibited rule must explicitly prohibit land application")
		}
	}
	seenRequirements := make(map[string]bool, len(pack.Requirements))
	for _, requirement := range pack.Requirements {
		if strings.TrimSpace(requirement.ID) == "" || seenRequirements[requirement.ID] || len(requirement.Tiers) == 0 || strings.TrimSpace(requirement.Title) == "" || strings.TrimSpace(requirement.Detail) == "" || strings.TrimSpace(requirement.Timing) == "" {
			return fmt.Errorf("requirement %q is incomplete or duplicated", requirement.ID)
		}
		seenRequirements[requirement.ID] = true
		for _, tier := range requirement.Tiers {
			if tier != TierStandard && tier != TierElevated && tier != TierProhibited {
				return fmt.Errorf("requirement %q has an unsupported tier", requirement.ID)
			}
		}
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
