package policy

import "testing"

func TestMichiganThresholdBoundaries(t *testing.T) {
	t.Parallel()
	pack := mustPack(t)
	tests := []struct {
		name string
		pfos string
		pfoa string
		want Tier
	}{
		{"both below 20", "19.99", "19.99", TierStandard},
		{"PFOS at 20", "20.00", "0", TierElevated},
		{"PFOA at 20", "0", "20.00", TierElevated},
		{"both below 100", "99.99", "99.99", TierElevated},
		{"PFOS at 100", "100.00", "0", TierProhibited},
		{"PFOA at 100", "0", "100.00", TierProhibited},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			matrix, method, basis := "BIOSOLIDS", "EPA 1633", "DRY"
			got := Evaluate(pack, ClassificationInput{Jurisdiction: "MI", Matrix: &matrix, Method: &method, Basis: &basis, Analytes: []AnalyteEvidence{detected("PFOS", test.pfos), detected("PFOA", test.pfoa)}})
			if got.Tier != test.want {
				t.Fatalf("Evaluate() tier = %s, want %s", got.Tier, test.want)
			}
		})
	}
}

func TestNonDetectIsNeverTreatedAsZero(t *testing.T) {
	t.Parallel()
	pack := mustPack(t)
	matrix, method, basis := "BIOSOLIDS", "EPA 1633", "DRY"
	limit := "25"
	result := Evaluate(pack, ClassificationInput{Jurisdiction: "MI", Matrix: &matrix, Method: &method, Basis: &basis, Analytes: []AnalyteEvidence{
		{CanonicalAnalyte: "PFOS", ResultText: "ND", IsNonDetect: true, UpperBoundUGKGDry: &limit},
		detected("PFOA", "1"),
	}})
	if result.Tier != TierUndetermined || result.BlockingReason == nil {
		t.Fatalf("Evaluate() = %#v, want blocked non-detect", result)
	}
}

func TestNonDetectAtThresholdResolvesBelowThreshold(t *testing.T) {
	t.Parallel()
	pack := mustPack(t)
	matrix, method, basis := "BIOSOLIDS", "EPA 1633", "DRY"
	limit := "20.00"
	result := Evaluate(pack, ClassificationInput{Jurisdiction: "MI", Matrix: &matrix, Method: &method, Basis: &basis, Analytes: []AnalyteEvidence{
		{CanonicalAnalyte: "PFOS", ResultText: "ND", IsNonDetect: true, UpperBoundUGKGDry: &limit},
		detected("PFOA", "19.99"),
	}})
	if result.Tier != TierStandard {
		t.Fatalf("Evaluate() tier = %s, want %s", result.Tier, TierStandard)
	}
}

func TestTierEffectsAreMachineReadableAndCited(t *testing.T) {
	t.Parallel()
	pack := mustPack(t)
	matrix, method, basis := "BIOSOLIDS", "EPA 1633", "DRY"

	elevated := Evaluate(pack, ClassificationInput{Jurisdiction: "MI", Matrix: &matrix, Method: &method, Basis: &basis, Analytes: []AnalyteEvidence{detected("PFOS", "20"), detected("PFOA", "1")}})
	if elevated.MaximumApplicationRateDryTonsPerAcre == nil || *elevated.MaximumApplicationRateDryTonsPerAcre != "1.5" {
		t.Fatalf("elevated maximum rate = %v, want 1.5", elevated.MaximumApplicationRateDryTonsPerAcre)
	}
	for _, requirement := range elevated.Requirements {
		if requirement.RuleID != elevated.MatchedRuleID || requirement.SourceURL != pack.SourceURL {
			t.Fatalf("requirement is not tied to the matched rule and source: %#v", requirement)
		}
	}

	prohibited := Evaluate(pack, ClassificationInput{Jurisdiction: "MI", Matrix: &matrix, Method: &method, Basis: &basis, Analytes: []AnalyteEvidence{detected("PFOS", "100"), detected("PFOA", "1")}})
	if !containsString(prohibited.ProhibitedActions, "LAND_APPLICATION") {
		t.Fatalf("prohibited actions = %v, want LAND_APPLICATION", prohibited.ProhibitedActions)
	}
	if len(prohibited.Requirements) == 0 || prohibited.Requirements[0].ID != "MI-PFAS-WRD-NOTIFICATION" || prohibited.Requirements[0].Timing != "Immediate" {
		t.Fatalf("prohibited notification = %#v, want immediate WRD notification", prohibited.Requirements)
	}
}

func TestUnsupportedAnalyticalMethodBlocksClassification(t *testing.T) {
	t.Parallel()
	pack := mustPack(t)
	matrix, method, basis := "BIOSOLIDS", "EPA 200.8", "DRY"
	result := Evaluate(pack, ClassificationInput{Jurisdiction: "MI", Matrix: &matrix, Method: &method, Basis: &basis, Analytes: []AnalyteEvidence{detected("PFOS", "1"), detected("PFOA", "1")}})
	if result.Tier != TierUndetermined || result.BlockingReason == nil {
		t.Fatalf("Evaluate() = %#v, want unsupported method to block classification", result)
	}
}

func TestMichiganReportIsotopicDilutionMethodIsAccepted(t *testing.T) {
	t.Parallel()
	pack := mustPack(t)
	matrix := "BIOSOLIDS"
	method := "ASTM D7968-17M — ASTM Method D7968 - 17 Modified (Isotopic Dilution)"
	basis := "DRY"
	result := Evaluate(pack, ClassificationInput{Jurisdiction: "MI", Matrix: &matrix, Method: &method, Basis: &basis, Analytes: []AnalyteEvidence{detected("PFOS", "5.5"), detected("PFOA", "3")}})
	if result.Tier != TierStandard {
		t.Fatalf("Evaluate() tier = %s, want %s; blocked by %v", result.Tier, TierStandard, result.BlockingReason)
	}
}

func TestDraftGuidanceCannotBecomeActiveRulePack(t *testing.T) {
	t.Parallel()
	pack := mustPack(t)
	pack.AuthorityType = "DRAFT_GUIDANCE"
	if err := validateRulePack(pack); err == nil {
		t.Fatal("validateRulePack() accepted draft guidance as active compliance policy")
	}
}

func mustPack(t *testing.T) RulePack {
	t.Helper()
	packs, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 {
		t.Fatalf("LoadCatalog() returned %d packs", len(packs))
	}
	return packs[0]
}

func detected(name, value string) AnalyteEvidence {
	return AnalyteEvidence{CanonicalAnalyte: name, ResultText: value, NormalizedValueUGKGDry: &value}
}
