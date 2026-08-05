package policy

import (
	"fmt"
	"math/big"
	"strings"
)

func Evaluate(pack RulePack, input ClassificationInput) Evaluation {
	if input.Jurisdiction != pack.Jurisdiction {
		return blocked("No active reviewed rule pack matches this facility's jurisdiction.")
	}
	if input.Matrix == nil || !isBiosolids(*input.Matrix) {
		return blocked("Confirm that the laboratory matrix is biosolids or sewage sludge.")
	}
	if input.Method == nil || !isAcceptedMethod(pack.AcceptedAnalyticalMethodTokens, *input.Method) {
		return blocked("Confirm an accepted biosolids PFAS analytical method: EPA 1633, ASTM D7979, or isotope dilution (Method 537 modified).")
	}
	if input.Basis == nil || strings.ToUpper(strings.TrimSpace(*input.Basis)) != "DRY" {
		return blocked("Confirmed dry-weight evidence is required for classification.")
	}

	elevated, _ := decimal(pack.ElevatedThresholdUGKGDry)
	prohibited, _ := decimal(pack.ProhibitedThresholdUGKGDry)
	values := make(map[string]*big.Rat, 2)
	resolved := make(map[string]bool, 2)
	for _, analyte := range input.Analytes {
		name := strings.ToUpper(strings.TrimSpace(analyte.CanonicalAnalyte))
		if name != "PFOS" && name != "PFOA" {
			continue
		}
		value := analyte.NormalizedValueUGKGDry
		if analyte.IsNonDetect {
			value = analyte.UpperBoundUGKGDry
		}
		if value == nil {
			return blocked(fmt.Sprintf("%s needs a dry-weight value or a usable non-detect reporting limit.", name))
		}
		parsed, err := decimal(*value)
		if err != nil || parsed.Sign() < 0 {
			return blocked(fmt.Sprintf("%s does not contain a valid non-negative dry-weight value.", name))
		}
		if analyte.IsNonDetect && parsed.Cmp(elevated) > 0 {
			return blocked(fmt.Sprintf("%s is non-detect, but its reporting limit is above the 20 µg/kg threshold.", name))
		}
		resolved[name] = true
		if !analyte.IsNonDetect {
			values[name] = parsed
		}
	}
	if !resolved["PFOS"] || !resolved["PFOA"] {
		return blocked("Confirmed PFOS and PFOA evidence are both required.")
	}

	if atOrAbove(values["PFOS"], prohibited) || atOrAbove(values["PFOA"], prohibited) {
		return matched(pack, TierProhibited, "MI-PFAS-PROHIBIT-100")
	}
	if atOrAbove(values["PFOS"], elevated) || atOrAbove(values["PFOA"], elevated) {
		return matched(pack, TierElevated, "MI-PFAS-ELEVATED-20")
	}
	return matched(pack, TierStandard, "MI-PFAS-STANDARD-BELOW-20")
}

func atOrAbove(value, threshold *big.Rat) bool {
	return value != nil && value.Cmp(threshold) >= 0
}

func matched(pack RulePack, tier Tier, ruleID string) Evaluation {
	result := Evaluation{Tier: tier, MatchedRuleID: ruleID, Requirements: requirementsFor(pack, tier, ruleID), ProhibitedActions: []string{}}
	for _, rule := range pack.Rules {
		if rule.ID == ruleID {
			result.Explanation = rule.Explanation
			result.MaximumApplicationRateDryTonsPerAcre = rule.MaximumApplicationRateDryTonsPerAcre
			result.ProhibitedActions = append(result.ProhibitedActions, rule.ProhibitedActions...)
			break
		}
	}
	return result
}

func requirementsFor(pack RulePack, tier Tier, ruleID string) []Requirement {
	result := make([]Requirement, 0)
	for _, definition := range pack.Requirements {
		if !containsTier(definition.Tiers, tier) {
			continue
		}
		result = append(result, Requirement{
			ID: definition.ID, Title: definition.Title, Detail: definition.Detail, Timing: definition.Timing,
			RuleID: ruleID, SourceURL: pack.SourceURL, SourceTitle: pack.SourceTitle, AuthorityType: pack.AuthorityType,
		})
	}
	return result
}

func blocked(reason string) Evaluation {
	return Evaluation{Tier: TierUndetermined, Explanation: "The evidence is not sufficient for deterministic classification.", BlockingReason: &reason, Requirements: []Requirement{}}
}

func containsTier(tiers []Tier, target Tier) bool {
	for _, tier := range tiers {
		if tier == target {
			return true
		}
	}
	return false
}

func isBiosolids(value string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	return normalized == "BIOSOLIDS" || normalized == "SEWAGE SLUDGE" || normalized == "SLUDGE"
}

func isAcceptedMethod(tokens []string, value string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	for _, token := range tokens {
		if strings.Contains(normalized, strings.ToUpper(token)) {
			return true
		}
	}
	return false
}

func decimal(value string) (*big.Rat, error) {
	parsed := new(big.Rat)
	if _, ok := parsed.SetString(strings.TrimSpace(value)); !ok {
		return nil, fmt.Errorf("invalid decimal %q", value)
	}
	return parsed, nil
}
