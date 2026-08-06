package mireye

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDistancePreservesDroppedAndUnreachableDestinations(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != proximityPath {
			t.Fatalf("path = %s", request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-Request-ID", "req_route")
		_, _ = response.Write([]byte(`{
          "op":"distance",
          "legs":[
            {"origin_index":0,"destination_index":0,"distance_miles":10,"distance_km":16.09,"duration_seconds":1200,"duration_minutes":20,"flag":null},
            {"origin_index":0,"destination_index":1,"distance_miles":8,"distance_km":12.87,"duration_seconds":null,"duration_minutes":null,"flag":"unreachable_or_snapped"}
          ],
          "resolved_origins":[{"query":"42,-84","lat":42,"lng":-84}],
          "resolved_destinations":[
            {"query":"43,-85","lat":43,"lng":-85},
            {"query":"44,-86","lat":44,"lng":-86},
            {"query":"45,-87","lat":null,"lng":null,"error":"unresolvable_input"}
          ],
          "paid_driving_calcs":3,
          "notes":["1 destination failed to resolve","durations reflect typical traffic, not real-time"]
        }`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", server.Client())
	result, err := client.Distance(context.Background(), DistanceRequest{
		Origins: []string{"42,-84"}, Destinations: []string{"43,-85", "44,-86", "45,-87"}, MaxCredits: 36,
	})
	if err != nil {
		t.Fatalf("Distance() error = %v", err)
	}
	if len(result.Legs) != 2 || len(result.ResolvedDestinations) != 3 || result.PaidDrivingCalcs != 3 {
		t.Fatalf("unexpected distance result: %+v", result)
	}
	if result.Legs[1].Flag == nil || *result.Legs[1].Flag != "unreachable_or_snapped" {
		t.Fatalf("unreachable flag = %v", result.Legs[1].Flag)
	}
}

func TestDistanceRejectsSpendAboveModuleLimit(t *testing.T) {
	t.Parallel()
	client := NewClient("https://api.mireye.com", "token", http.DefaultClient)
	_, err := client.Distance(context.Background(), DistanceRequest{
		Origins: []string{"42,-84"}, Destinations: []string{"43,-85"}, MaxCredits: 61,
	})
	if err == nil {
		t.Fatal("Distance() accepted a credit cap above the Module 6 limit")
	}
}
