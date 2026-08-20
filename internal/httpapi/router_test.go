package httpapi

import (
	"io"
	"log/slog"
	"reflect"
	"testing"
)

func TestRouterRegistersAllSchemasWithoutNameCollisions(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if router := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger, "http://localhost:5174"); router == nil {
		t.Fatal("router is nil")
	}
}

func TestAllowedWebOriginsAddsEquivalentLoopbackOrigin(t *testing.T) {
	t.Parallel()
	want := []string{"http://localhost:5174", "http://127.0.0.1:5174"}
	if got := allowedWebOrigins("http://localhost:5174"); !reflect.DeepEqual(got, want) {
		t.Fatalf("allowedWebOrigins() = %v, want %v", got, want)
	}
}

func TestAllowedWebOriginsKeepsProductionOriginRestricted(t *testing.T) {
	t.Parallel()
	want := []string{"https://pfas.example.com"}
	if got := allowedWebOrigins("https://pfas.example.com"); !reflect.DeepEqual(got, want) {
		t.Fatalf("allowedWebOrigins() = %v, want %v", got, want)
	}
}
