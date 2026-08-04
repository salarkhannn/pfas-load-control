package mireye

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientValidatesFieldsAndCapturesProvenance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("request omitted bearer authentication")
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-Request-ID", "req_test")
		_, _ = response.Write([]byte(`{"fields":[{"name":"slope","layer":"terrain","source":"USGS"}],"presets":{"site":{}},"us_envelope":{},"version":"0.14.0"}`))
	}))
	defer server.Close()

	result, err := NewClient(server.URL, "test-token", server.Client()).Call(context.Background(), ToolFields)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.RequestID != "req_test" || len(result.ResponseHash) != 64 || result.HTTPStatus != http.StatusOK {
		t.Fatalf("Call() provenance = %+v", result)
	}
	summary, ok := result.Summary.(FieldsSummary)
	if !ok || summary.FieldCount != 1 || summary.CatalogVersion != "0.14.0" {
		t.Fatalf("Call() summary = %#v", result.Summary)
	}
}

func TestClientRejectsMalformedSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"fields":[]}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "test-token", server.Client()).Call(context.Background(), ToolFields)
	var callErr *CallError
	if !errors.As(err, &callErr) || callErr.Code != "MIREYE_SCHEMA_MISMATCH" || callErr.Retryable {
		t.Fatalf("Call() error = %#v", err)
	}
}

func TestClientClassifiesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Retry-After", "12")
		response.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "test-token", server.Client()).Call(context.Background(), ToolUsage)
	var callErr *CallError
	if !errors.As(err, &callErr) || callErr.Code != "MIREYE_RATE_LIMITED" || !callErr.Retryable || callErr.RetryAfter != 12*time.Second {
		t.Fatalf("Call() error = %#v", err)
	}
}
