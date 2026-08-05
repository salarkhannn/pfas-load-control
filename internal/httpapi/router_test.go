package httpapi

import (
	"reflect"
	"testing"
)

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
