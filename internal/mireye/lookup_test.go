package mireye

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupResolvedSanitizesParcel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != lookupPath || request.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Fatal("missing bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "lookup-1")
		_, _ = w.Write([]byte(`{
			"disposition":"resolved","lat":42.7,"lng":-84.5,
			"resolved_address":"100 Farm Rd","state":"Michigan","county":"Ingham County",
			"fips":"26065","confidence":0.8,"match_method":"point_in_parcel",
			"owner":"Private Person",
			"parcel":{"parcel_id":"P-1","geometry":"{\"type\":\"Polygon\",\"coordinates\":[[[-84.51,42.70],[-84.50,42.70],[-84.50,42.71],[-84.51,42.70]]]}","match_type":"exact_intersect","match_distance_m":0,"source":"regrid_paid","owner":"Private Person"}
		}`))
	}))
	defer server.Close()

	result, err := NewClient(server.URL, "token", server.Client()).Lookup(context.Background(), LookupRequest{Input: "42.7,-84.5", Kind: LookupCoordinate})
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if result.Response.Disposition != "resolved" || result.Response.Parcel == nil {
		t.Fatalf("unexpected lookup response %#v", result.Response)
	}
	if result.Response.Parcel.MatchType != "exact_intersect" || result.RequestID != "lookup-1" {
		t.Fatalf("missing parcel provenance %#v", result)
	}
	if string(result.Evidence) == "" || contains(string(result.Evidence), "Private Person") {
		t.Fatalf("evidence was not sanitized: %s", result.Evidence)
	}
}

func TestLookupRejectsMalformedClarification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"disposition":"clarify","candidates":[]}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "token", server.Client()).Lookup(context.Background(), LookupRequest{Input: "Main Street", Kind: LookupAddress})
	if err == nil {
		t.Fatal("Lookup() expected schema error")
	}
	callErr, ok := err.(*CallError)
	if !ok || callErr.Code != "MIREYE_SCHEMA_MISMATCH" {
		t.Fatalf("Lookup() error = %#v", err)
	}
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
