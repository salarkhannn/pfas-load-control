package evidence

import (
	"encoding/json"
	"testing"
)

func TestAggregateMinimumKeepsWetCorner(t *testing.T) {
	result, err := aggregateField(fieldSpec{Name: "nearest_wetland_distance_m", Method: aggregateMinimum}, []sampleObservation{
		{Index: 0, Status: "ok", Value: json.RawMessage(`450`)},
		{Index: 1, Status: "ok", Value: json.RawMessage(`8`)},
		{Index: 2, Status: "ok", Value: json.RawMessage(`320`)},
	}, 3)
	if err != nil {
		t.Fatalf("aggregateField() error = %v", err)
	}
	if string(result.Value) != "8" || result.State != "COMPLETE" {
		t.Fatalf("aggregateField() = %+v", result)
	}
}

func TestAggregateMarksPartialFailure(t *testing.T) {
	result, err := aggregateField(fieldSpec{Name: "intersects_wetland", Method: aggregateAnyTrue}, []sampleObservation{
		{Index: 0, Status: "ok", Value: json.RawMessage(`false`)},
		{Index: 1, Status: "failed"},
	}, 2)
	if err != nil {
		t.Fatalf("aggregateField() error = %v", err)
	}
	if result.State != "PARTIAL" || result.FailedCount != 1 || string(result.Value) != "false" {
		t.Fatalf("aggregateField() = %+v", result)
	}
}
