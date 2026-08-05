package mireye

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchBatchPreservesTriStateFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/fetch/batch" || request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("unexpected Mireye request: %s", request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-Request-ID", "req_batch")
		_, _ = response.Write([]byte(`{"fetched_at":"2026-08-05T00:00:00Z","results":[{"index":0,"ok":true,"lat":42.7,"lng":-84.5,"fields":{"slope_degrees":{"value":2.5,"unit":"degrees","source":"USGS_3DEP","source_url":"https://www.usgs.gov/3d-elevation-program","status":"ok"},"soil_drainage_class":{"value":null,"source":"USDA_SSURGO","status":"failed","error":"timeout","retryable":true}},"partial_failures":[{"field":"soil_drainage_class","source":"USDA_SSURGO","error":"timeout","retryable":true}],"resolved_location":{"lat":42.7,"lng":-84.5,"source":"coordinate"}}]}`))
	}))
	defer server.Close()

	result, err := NewClient(server.URL, "test-token", server.Client()).FetchBatch(context.Background(), FetchBatchRequest{
		Locations: []Coordinate{{Latitude: 42.7, Longitude: -84.5}},
		Fields:    []string{"slope_degrees", "soil_drainage_class"},
	})
	if err != nil {
		t.Fatalf("FetchBatch() error = %v", err)
	}
	if result.RequestID != "req_batch" || result.Results[0].Fields["soil_drainage_class"].Status != "failed" || !result.Results[0].Fields["soil_drainage_class"].Retryable {
		t.Fatalf("FetchBatch() result = %+v", result)
	}
}

func TestFetchBatchRejectsMisalignedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"results":[{"index":2,"ok":false,"error":{"error":"bad","message":"bad","retryable":false}}]}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "test-token", server.Client()).FetchBatch(context.Background(), FetchBatchRequest{
		Locations: []Coordinate{{Latitude: 42.7, Longitude: -84.5}}, Fields: []string{"slope_degrees"},
	})
	var callErr *CallError
	if !errors.As(err, &callErr) || callErr.Code != "MIREYE_SCHEMA_MISMATCH" {
		t.Fatalf("FetchBatch() error = %#v", err)
	}
}
