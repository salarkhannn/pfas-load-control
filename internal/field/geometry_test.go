package field

import (
	"encoding/json"
	"testing"
)

func TestCanonicalGeometryNormalizesPolygon(t *testing.T) {
	t.Parallel()
	geometry, digest, err := canonicalGeometry(`{
		"type":"Feature","properties":{"name":"North 40"},
		"geometry":{"type":"Polygon","coordinates":[[[-84.56,42.73],[-84.55,42.73],[-84.55,42.74],[-84.56,42.73]]]}
	}`)
	if err != nil {
		t.Fatalf("canonicalGeometry() error = %v", err)
	}
	var result geoJSONGeometry
	if err := json.Unmarshal(geometry, &result); err != nil {
		t.Fatalf("canonical geometry is invalid JSON: %v", err)
	}
	if result.Type != "MultiPolygon" || len(result.Coordinates) != 1 || len(digest) != 64 {
		t.Fatalf("canonicalGeometry() = %s, %q", geometry, digest)
	}
}

func TestCanonicalGeometryRejectsOpenRing(t *testing.T) {
	t.Parallel()
	_, _, err := canonicalGeometry(`{"type":"Polygon","coordinates":[[[-84.56,42.73],[-84.55,42.73],[-84.55,42.74],[-84.56,42.74]]]}`)
	if err == nil {
		t.Fatal("canonicalGeometry() accepted an open ring")
	}
}
