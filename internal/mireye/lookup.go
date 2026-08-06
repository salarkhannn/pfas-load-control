package mireye

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const lookupPath = "/v1/lookup"

type LookupKind string

const (
	LookupAddress    LookupKind = "address"
	LookupCoordinate LookupKind = "coord"
	LookupAPN        LookupKind = "apn"
)

type LookupRequest struct {
	Input string     `json:"input"`
	Kind  LookupKind `json:"kind"`
}

type LookupCandidate struct {
	Label           string   `json:"label,omitempty"`
	ResolvedAddress string   `json:"resolved_address,omitempty"`
	Latitude        *float64 `json:"lat,omitempty"`
	Longitude       *float64 `json:"lng,omitempty"`
	Confidence      *float64 `json:"confidence,omitempty"`
	MatchMethod     string   `json:"match_method,omitempty"`
}

type LookupParcel struct {
	ID             string          `json:"id,omitempty"`
	APN            string          `json:"apn,omitempty"`
	Geometry       json.RawMessage `json:"geometry,omitempty"`
	MatchType      string          `json:"match_type,omitempty"`
	MatchDistanceM *float64        `json:"match_distance_m,omitempty"`
	Source         string          `json:"source,omitempty"`
}

type LookupResponse struct {
	Disposition       string            `json:"disposition"`
	Latitude          *float64          `json:"lat,omitempty"`
	Longitude         *float64          `json:"lng,omitempty"`
	ResolvedAddress   string            `json:"resolved_address,omitempty"`
	State             string            `json:"state,omitempty"`
	County            string            `json:"county,omitempty"`
	FIPS              string            `json:"fips,omitempty"`
	Confidence        *float64          `json:"confidence,omitempty"`
	MatchMethod       string            `json:"match_method,omitempty"`
	Parcel            *LookupParcel     `json:"parcel,omitempty"`
	ParcelUnavailable bool              `json:"parcel_unavailable,omitempty"`
	Candidates        []LookupCandidate `json:"candidates,omitempty"`
	Reason            string            `json:"reason,omitempty"`
	Hint              string            `json:"hint,omitempty"`
}

type LookupResult struct {
	RequestHash  string
	ResponseHash string
	RequestID    string
	SourceURL    string
	FetchedAt    time.Time
	Response     LookupResponse
	Evidence     json.RawMessage
}

func (c *Client) Lookup(ctx context.Context, request LookupRequest) (LookupResult, error) {
	payload, requestHash, err := prepareLookupRequest(request)
	if err != nil {
		return LookupResult{}, err
	}

	startedAt := c.now()
	result := LookupResult{
		RequestHash: requestHash,
		SourceURL:   c.baseURL + lookupPath,
		FetchedAt:   startedAt.UTC(),
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, result.SourceURL, bytes.NewReader(payload))
	if err != nil {
		return result, fmt.Errorf("build Mireye lookup request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+c.token)
	httpRequest.Header.Set("User-Agent", "pfas-load-control/0.1")

	response, err := c.http.Do(httpRequest)
	if err != nil {
		code := "MIREYE_NETWORK_ERROR"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = "MIREYE_TIMEOUT"
		}
		return result, &CallError{Code: code, Detail: "Mireye could not resolve the field location", Retryable: true}
	}
	defer response.Body.Close()
	result.RequestID = response.Header.Get("X-Request-ID")
	result.FetchedAt = c.now().UTC()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		callErr := classifyStatus(response, Result{})
		if typed, ok := callErr.(*CallError); ok {
			typed.Detail = "Mireye could not resolve the field location"
		}
		return result, callErr
	}
	if mediaType := strings.ToLower(response.Header.Get("Content-Type")); !strings.Contains(mediaType, "application/json") {
		return result, &CallError{Code: "MIREYE_INVALID_CONTENT_TYPE", Detail: "Mireye returned a non-JSON lookup response", Status: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return result, &CallError{Code: "MIREYE_RESPONSE_READ_ERROR", Detail: "Mireye lookup response could not be read", Status: response.StatusCode, Retryable: true}
	}
	if len(body) > maxResponseBytes {
		return result, &CallError{Code: "MIREYE_RESPONSE_TOO_LARGE", Detail: "Mireye lookup response exceeded the allowed size", Status: response.StatusCode}
	}

	var parsed lookupWireResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye returned invalid lookup JSON", Status: response.StatusCode}
	}
	lookup, err := parsed.validated()
	if err != nil {
		return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: err.Error(), Status: response.StatusCode}
	}
	evidence, err := json.Marshal(lookup)
	if err != nil {
		return result, fmt.Errorf("encode sanitized Mireye lookup evidence: %w", err)
	}
	result.ResponseHash = hash(body)
	result.Response = lookup
	result.Evidence = evidence
	return result, nil
}

func LookupRequestHash(request LookupRequest) (string, error) {
	_, requestHash, err := prepareLookupRequest(request)
	return requestHash, err
}

func prepareLookupRequest(request LookupRequest) ([]byte, string, error) {
	request.Input = strings.TrimSpace(request.Input)
	if request.Input == "" || len(request.Input) > 256 {
		return nil, "", errors.New("mireye lookup input must contain 1 to 256 characters")
	}
	if request.Kind != LookupAddress && request.Kind != LookupCoordinate && request.Kind != LookupAPN {
		return nil, "", fmt.Errorf("unsupported Mireye lookup kind %q", request.Kind)
	}

	payload, err := json.Marshal(struct {
		Input         string     `json:"input"`
		Kind          LookupKind `json:"kind"`
		IncludeParcel bool       `json:"include_parcel"`
	}{Input: request.Input, Kind: request.Kind, IncludeParcel: true})
	if err != nil {
		return nil, "", fmt.Errorf("encode Mireye lookup request: %w", err)
	}
	requestHash := hash(append([]byte(http.MethodPost+" "+lookupPath+"\n"), payload...))
	return payload, requestHash, nil
}

type lookupWireResponse struct {
	Disposition       string            `json:"disposition"`
	Latitude          *float64          `json:"lat"`
	Longitude         *float64          `json:"lng"`
	ResolvedAddress   string            `json:"resolved_address"`
	State             string            `json:"state"`
	County            string            `json:"county"`
	FIPS              string            `json:"fips"`
	Confidence        *float64          `json:"confidence"`
	MatchMethod       string            `json:"match_method"`
	Parcel            *lookupWireParcel `json:"parcel"`
	ParcelUnavailable bool              `json:"parcel_unavailable"`
	Candidates        []LookupCandidate `json:"candidates"`
	Reason            string            `json:"reason"`
	Hint              string            `json:"hint"`
}

type lookupWireParcel struct {
	ID             string          `json:"id"`
	ParcelID       string          `json:"parcel_id"`
	APN            string          `json:"apn"`
	Geometry       json.RawMessage `json:"geometry"`
	MatchType      string          `json:"match_type"`
	MatchDistanceM *float64        `json:"match_distance_m"`
	Source         string          `json:"source"`
}

func (wire lookupWireResponse) validated() (LookupResponse, error) {
	if wire.Disposition != "resolved" && wire.Disposition != "clarify" && wire.Disposition != "no_match" {
		return LookupResponse{}, fmt.Errorf("mireye returned unsupported lookup disposition %q", wire.Disposition)
	}
	if wire.Confidence != nil && (*wire.Confidence < 0 || *wire.Confidence > 1) {
		return LookupResponse{}, errors.New("mireye returned lookup confidence outside 0 through 1")
	}
	if wire.Disposition == "resolved" {
		if wire.Latitude == nil || wire.Longitude == nil || wire.Confidence == nil {
			return LookupResponse{}, errors.New("mireye resolved a location without coordinates and confidence")
		}
		if *wire.Latitude < -90 || *wire.Latitude > 90 || *wire.Longitude < -180 || *wire.Longitude > 180 {
			return LookupResponse{}, errors.New("mireye resolved coordinates outside valid bounds")
		}
	}
	if wire.Disposition == "clarify" && (len(wire.Candidates) == 0 || len(wire.Candidates) > 3) {
		return LookupResponse{}, errors.New("mireye clarification must contain one to three candidates")
	}
	if wire.Disposition == "no_match" && strings.TrimSpace(wire.Reason) == "" {
		return LookupResponse{}, errors.New("mireye no-match response omitted its reason")
	}

	candidates := wire.Candidates
	if candidates == nil {
		candidates = []LookupCandidate{}
	}
	result := LookupResponse{
		Disposition:       wire.Disposition,
		Latitude:          wire.Latitude,
		Longitude:         wire.Longitude,
		ResolvedAddress:   strings.TrimSpace(wire.ResolvedAddress),
		State:             strings.TrimSpace(wire.State),
		County:            strings.TrimSpace(wire.County),
		FIPS:              strings.TrimSpace(wire.FIPS),
		Confidence:        wire.Confidence,
		MatchMethod:       strings.TrimSpace(wire.MatchMethod),
		ParcelUnavailable: wire.ParcelUnavailable,
		Candidates:        candidates,
		Reason:            strings.TrimSpace(wire.Reason),
		Hint:              strings.TrimSpace(wire.Hint),
	}
	if wire.Parcel != nil {
		geometry, err := normalizeParcelGeometry(wire.Parcel.Geometry)
		if err != nil {
			return LookupResponse{}, err
		}
		parcelID := strings.TrimSpace(wire.Parcel.ParcelID)
		if parcelID == "" {
			parcelID = strings.TrimSpace(wire.Parcel.ID)
		}
		result.Parcel = &LookupParcel{
			ID:             parcelID,
			APN:            strings.TrimSpace(wire.Parcel.APN),
			Geometry:       geometry,
			MatchType:      strings.TrimSpace(wire.Parcel.MatchType),
			MatchDistanceM: wire.Parcel.MatchDistanceM,
			Source:         strings.TrimSpace(wire.Parcel.Source),
		}
	}
	return result, nil
}

func normalizeParcelGeometry(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var encoded string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return nil, errors.New("mireye parcel geometry was not valid JSON")
		}
		raw = json.RawMessage(encoded)
	}
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, errors.New("mireye parcel geometry was not valid GeoJSON")
	}
	if header.Type != "Polygon" && header.Type != "MultiPolygon" {
		return nil, fmt.Errorf("mireye parcel geometry has unsupported type %q", header.Type)
	}
	return append(json.RawMessage(nil), raw...), nil
}
