package placement

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestPolygonSamplingPlannerSupportsIrregularAndConcaveFields(t *testing.T) {
	policy := CurrentResolutionPolicy().Sampling
	tests := []struct {
		name string
		ring [][]float64
	}{
		{
			name: "irregular",
			ring: [][]float64{{-84.560, 42.731}, {-84.554, 42.7305}, {-84.552, 42.734}, {-84.555, 42.737}, {-84.559, 42.735}, {-84.560, 42.731}},
		},
		{
			name: "concave",
			ring: [][]float64{{-84.560, 42.731}, {-84.552, 42.731}, {-84.552, 42.739}, {-84.555, 42.739}, {-84.555, 42.734}, {-84.557, 42.734}, {-84.557, 42.739}, {-84.560, 42.739}, {-84.560, 42.731}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateSimpleRing(test.ring); err != nil {
				t.Fatal(err)
			}
			first, err := planPolygonScreeningSamples(test.ring, policy)
			if err != nil {
				t.Fatal(err)
			}
			second, err := planPolygonScreeningSamples(test.ring, policy)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("same geometry produced different sample plans:\n%#v\n%#v", first, second)
			}
			if len(first.Locations) != policy.TargetSampleCount || first.BoundaryNearSamples < 2 || first.InteriorSamples < 1 {
				t.Fatalf("sampling plan lacks requested boundary-near and interior locations: %#v", first)
			}
			for _, point := range first.Locations {
				if !pointStrictlyInside(point, test.ring) {
					t.Fatalf("planner placed a sample outside the polygon: %#v", point)
				}
			}
		})
	}
}

func TestSamplingRequestLimitLeavesFieldInReview(t *testing.T) {
	field, store, asOf := validResolutionFixture(t, eligibleField("slope", "Slope field", "50", "1", "0", 20, "grain"))
	field.Facts = append(field.Facts, rangeFact("slope_degrees", "Slope", 1, 9.42, 4))
	policy := CurrentResolutionPolicy()
	policy.Sampling.TargetSampleCount = policy.Sampling.MaxRequestSamples + 1
	plan, err := EvaluateWithEvidenceAndPolicy(context.Background(), Input{
		Tier: "STANDARD", WetMassKg: "9071.8474", PercentSolids: "100", EvidenceAsOf: asOf, Fields: []FieldInput{field},
	}, store, policy)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != StatusReviewRequired || len(plan.Fields) != 1 || plan.Fields[0].Disposition != DispositionReviewRequired || len(plan.Allocations) != 0 {
		t.Fatalf("sampling limit did not fail closed into review: %#v", plan)
	}
	joined := strings.Join(plan.Fields[0].Reasons, " ")
	if !strings.Contains(joined, "SAMPLING_LIMIT_REQUIRES_REVIEW") {
		t.Fatalf("sampling-limit reason was not preserved: %s", joined)
	}
}
