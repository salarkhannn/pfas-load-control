package field

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

type geoJSONEnvelope struct {
	Type        string          `json:"type"`
	Geometry    json.RawMessage `json:"geometry"`
	Coordinates json.RawMessage `json:"coordinates"`
}

type geoJSONGeometry struct {
	Type        string          `json:"type"`
	Coordinates [][][][]float64 `json:"coordinates"`
}

func canonicalGeometry(input string) (json.RawMessage, string, error) {
	if len(input) == 0 || len(input) > MaxCSVBytes {
		return nil, "", errors.New("field boundary must be valid GeoJSON no larger than 1 MiB")
	}
	var envelope geoJSONEnvelope
	if err := json.Unmarshal([]byte(input), &envelope); err != nil {
		return nil, "", errors.New("field boundary must be valid GeoJSON")
	}
	rawGeometry := []byte(input)
	if envelope.Type == "Feature" {
		if len(envelope.Geometry) == 0 || string(envelope.Geometry) == "null" {
			return nil, "", errors.New("GeoJSON feature must contain a geometry")
		}
		rawGeometry = envelope.Geometry
	}
	var geometryEnvelope geoJSONEnvelope
	if err := json.Unmarshal(rawGeometry, &geometryEnvelope); err != nil {
		return nil, "", errors.New("field boundary must contain valid GeoJSON geometry")
	}

	var coordinates [][][][]float64
	switch geometryEnvelope.Type {
	case "Polygon":
		var polygon [][][]float64
		if err := json.Unmarshal(geometryEnvelope.Coordinates, &polygon); err != nil {
			return nil, "", errors.New("polygon coordinates are invalid")
		}
		coordinates = [][][][]float64{polygon}
	case "MultiPolygon":
		if err := json.Unmarshal(geometryEnvelope.Coordinates, &coordinates); err != nil {
			return nil, "", errors.New("multipolygon coordinates are invalid")
		}
	default:
		return nil, "", fmt.Errorf("field boundary must be a Polygon or MultiPolygon, not %q", geometryEnvelope.Type)
	}
	if err := validateCoordinates(coordinates); err != nil {
		return nil, "", err
	}
	canonical, err := json.Marshal(geoJSONGeometry{Type: "MultiPolygon", Coordinates: coordinates})
	if err != nil {
		return nil, "", fmt.Errorf("encode canonical field boundary: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(digest[:]), nil
}

func validateCoordinates(polygons [][][][]float64) error {
	if len(polygons) == 0 {
		return errors.New("field boundary must contain at least one polygon")
	}
	for _, polygon := range polygons {
		if len(polygon) == 0 {
			return errors.New("each field polygon must contain an exterior ring")
		}
		for _, ring := range polygon {
			if len(ring) < 4 {
				return errors.New("each field boundary ring must contain at least four positions")
			}
			for _, position := range ring {
				if len(position) < 2 || math.IsNaN(position[0]) || math.IsNaN(position[1]) || math.IsInf(position[0], 0) || math.IsInf(position[1], 0) {
					return errors.New("each field boundary position must contain finite longitude and latitude")
				}
				if position[0] < -180 || position[0] > 180 || position[1] < -90 || position[1] > 90 {
					return errors.New("field boundary coordinates are outside longitude and latitude bounds")
				}
			}
			first, last := ring[0], ring[len(ring)-1]
			if first[0] != last[0] || first[1] != last[1] {
				return errors.New("each field boundary ring must be closed")
			}
		}
	}
	return nil
}
