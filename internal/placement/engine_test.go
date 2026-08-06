package placement

import (
	"encoding/json"
	"math/big"
	"math/rand/v2"
	"strconv"
	"testing"
)

func TestEvaluateElevatedAllocationHonorsRateAndCapacity(t *testing.T) {
	input := Input{
		Tier: "ELEVATED", PolicyRate: "1.5", WetMassKg: "90718.474", PercentSolids: "10",
		Fields: []FieldInput{
			eligibleField("field-b", "Field B", "20", "2.5", "0", 300, "grain"),
			eligibleField("field-a", "Field A", "10", "3", "0", 120, "grain"),
		},
	}
	result, err := Evaluate(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusReady {
		t.Fatalf("status = %s", result.Status)
	}
	if result.BatchDryTons != "10" || result.AllocatedDryTons != "10" || result.UnallocatedDryTons != "0" {
		t.Fatalf("unexpected mass balance: batch=%s allocated=%s residual=%s", result.BatchDryTons, result.AllocatedDryTons, result.UnallocatedDryTons)
	}
	if len(result.Allocations) != 1 {
		t.Fatalf("allocations = %d", len(result.Allocations))
	}
	if result.Allocations[0].Rate != "1.5" {
		t.Fatalf("rate = %s", result.Allocations[0].Rate)
	}
	if result.Allocations[0].DryTons != "10" {
		t.Fatalf("dry tons = %s", result.Allocations[0].DryTons)
	}
}

func TestEvaluateExcludesReviewRequiredField(t *testing.T) {
	unsafe := eligibleField("water", "Water overlap", "100", "2", "0", 20, "grain")
	unsafe.Facts = append(unsafe.Facts, booleanFact("intersects_wetland", "Wetland intersects field", true))
	safe := eligibleField("safe", "Dry field", "5", "2", "0", 80, "grain")
	result, err := Evaluate(Input{Tier: "STANDARD", WetMassKg: "9071.8474", PercentSolids: "10", Fields: []FieldInput{unsafe, safe}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Allocations) != 1 || result.Allocations[0].FieldID != "safe" {
		t.Fatalf("unexpected allocations: %#v", result.Allocations)
	}
	for _, field := range result.Fields {
		if field.FieldID == "water" && field.Disposition != DispositionReviewRequired {
			t.Fatalf("water field disposition = %s", field.Disposition)
		}
	}
}

func TestEvaluateProhibitedProducesNoAllocation(t *testing.T) {
	result, err := Evaluate(Input{Tier: "PROHIBITED", WetMassKg: "9071.8474", PercentSolids: "10", Fields: []FieldInput{eligibleField("field", "Field", "100", "2", "0", 100, "grain")}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusLandApplicationBlocked || len(result.Allocations) != 0 || result.AllocatedDryTons != "0" || result.UnallocatedDryTons != "1" {
		t.Fatalf("unexpected prohibited result: %#v", result)
	}
}

func TestEvaluateReportsResidualWithoutOverrunningCapacity(t *testing.T) {
	result, err := Evaluate(Input{Tier: "STANDARD", WetMassKg: "90718.474", PercentSolids: "10", Fields: []FieldInput{eligibleField("field", "Field", "2", "2", "1", 100, "grain")}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusInsufficientCapacity || result.AllocatedDryTons != "3" || result.UnallocatedDryTons != "7" {
		t.Fatalf("unexpected residual: %#v", result)
	}
}

func TestEvaluateRequiresReviewWhenNoFieldIsCurrentlyEligible(t *testing.T) {
	field := eligibleField("field", "Floodplain field", "10", "2", "0", 100, "grain")
	field.PhysicalCriticalGaps = 1

	result, err := Evaluate(Input{Tier: "STANDARD", WetMassKg: "10000", PercentSolids: "20", Fields: []FieldInput{field}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusReviewRequired {
		t.Fatalf("status = %s", result.Status)
	}
	if result.AllocatedDryTons != "0" || result.UnallocatedDryTons != result.BatchDryTons {
		t.Fatalf("unexpected mass balance: %#v", result)
	}
	if len(result.Gaps) != 1 || result.Gaps[0].Code != "FIELD_REVIEW_REQUIRED" {
		t.Fatalf("unexpected gaps: %#v", result.Gaps)
	}
	if result.Gaps[0].Detail != "No field is currently eligible to receive this batch." {
		t.Fatalf("gap detail = %q", result.Gaps[0].Detail)
	}
}

func TestEvaluateDistinguishesCapacityShortageFromFieldReview(t *testing.T) {
	eligible := eligibleField("eligible", "Eligible field", "1", "1", "0", 100, "grain")
	review := eligibleField("review", "Review field", "20", "2", "0", 200, "grain")
	review.PhysicalCriticalGaps = 1

	result, err := Evaluate(Input{Tier: "STANDARD", WetMassKg: "9071.8474", PercentSolids: "100", Fields: []FieldInput{eligible, review}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusReviewRequired {
		t.Fatalf("status = %s", result.Status)
	}
	if result.AllocatedDryTons != "1" || result.UnallocatedDryTons != "9" {
		t.Fatalf("unexpected mass balance: %#v", result)
	}
	if len(result.Gaps) != 1 || result.Gaps[0].Code != "FIELD_REVIEW_REQUIRED" {
		t.Fatalf("unexpected gaps: %#v", result.Gaps)
	}
}

func TestAllocationMassAndCapacityProperties(t *testing.T) {
	for iteration := 0; iteration < 1000; iteration++ {
		fieldCount := 1 + rand.IntN(8)
		fields := make([]FieldInput, 0, fieldCount)
		for index := 0; index < fieldCount; index++ {
			acres := 1 + rand.IntN(100)
			rate := 1 + rand.IntN(5)
			fields = append(fields, eligibleField(string(rune('a'+index)), "Field", decimalInteger(acres), decimalInteger(rate), "0", float64(rand.IntN(500)), "grain"))
		}
		wetTons := 1 + rand.IntN(500)
		result, err := Evaluate(Input{Tier: "STANDARD", WetMassKg: decimalInteger(wetTons * 907), PercentSolids: "100", Fields: fields})
		if err != nil {
			t.Fatal(err)
		}
		batch, _ := decimal(result.BatchDryTons)
		allocated, _ := decimal(result.AllocatedDryTons)
		residual, _ := decimal(result.UnallocatedDryTons)
		if newRatSum(allocated, residual).Cmp(batch) != 0 {
			t.Fatalf("mass balance failed: %#v", result)
		}
		capacities := map[string]string{}
		for _, field := range result.Fields {
			capacities[field.FieldID] = field.AvailableCapacity
		}
		for _, allocation := range result.Allocations {
			amount, _ := decimal(allocation.DryTons)
			capacity, _ := decimal(capacities[allocation.FieldID])
			if amount.Cmp(capacity) > 0 {
				t.Fatalf("allocation %s exceeds capacity %s", allocation.DryTons, capacities[allocation.FieldID])
			}
		}
	}
}

func eligibleField(id, name, acres, rate, prior string, road float64, crop string) FieldInput {
	return FieldInput{
		ID: id, Name: name, Status: "READY", RMPApproved: true, UsableAcres: acres,
		AgronomicRate: rate, PriorLoadingDryTons: prior, CropOrUse: crop,
		PhysicalEvaluationID: "evaluation-" + id, PhysicalStatus: "SUCCEEDED", SupplementalAvailable: true,
		Facts: []FactInput{
			booleanFact("within_floodplain_polygon", "Floodplain intersects field", false),
			booleanFact("intersects_wetland", "Wetland intersects field", false),
			booleanFact("intersects_nhd_area", "Mapped surface water intersects field", false),
			numberFact("nearest_wetland_distance_m", "Nearest wetland", 800),
			numberFact("wetlands_within_100m_count", "Wetlands within 100 m", 0),
			numberFact("wetlands_within_500m_count", "Wetlands within 500 m", 0),
			numberFact("nearest_groundwater_well_depth_to_water_m", "Nearest measured depth to groundwater", 12),
			rangeFact("slope_degrees", "Slope", 0.5, 1.2, 0.8),
			numberFact("housing_units_within_1km", "Homes within 1 km", 0),
			numberFact("nearest_road_distance_m", "Nearest road", road),
		},
	}
}

func booleanFact(name, label string, value bool) FactInput {
	encoded, _ := json.Marshal(value)
	return FactInput{Name: name, Label: label, State: "COMPLETE", Value: encoded}
}
func numberFact(name, label string, value float64) FactInput {
	encoded, _ := json.Marshal(value)
	return FactInput{Name: name, Label: label, State: "COMPLETE", Value: encoded}
}
func rangeFact(name, label string, min, max, median float64) FactInput {
	encoded, _ := json.Marshal(map[string]float64{"min": min, "max": max, "median": median})
	return FactInput{Name: name, Label: label, State: "COMPLETE", Value: encoded}
}
func decimalInteger(value int) string { return strconv.Itoa(value) }

func newRatSum(left, right *big.Rat) *big.Rat { return new(big.Rat).Add(left, right) }
