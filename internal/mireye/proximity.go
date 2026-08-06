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

const proximityPath = "/v1/proximity"

type DistanceRequest struct {
	Origins      []string `json:"origins"`
	Destinations []string `json:"destinations"`
	MaxCredits   int      `json:"max_credits"`
}

type DistanceLeg struct {
	OriginIndex      int      `json:"origin_index"`
	DestinationIndex int      `json:"destination_index"`
	DistanceMiles    float64  `json:"distance_miles"`
	DistanceKM       float64  `json:"distance_km"`
	DurationSeconds  *float64 `json:"duration_seconds"`
	DurationMinutes  *float64 `json:"duration_minutes"`
	Flag             *string  `json:"flag"`
}

type ResolvedPoint struct {
	Query            string   `json:"query"`
	Latitude         *float64 `json:"lat"`
	Longitude        *float64 `json:"lng"`
	FormattedAddress *string  `json:"formatted_address"`
	AccuracyType     *string  `json:"accuracy_type"`
	Accuracy         *float64 `json:"accuracy"`
	Error            *string  `json:"error"`
}

type DistanceResult struct {
	SourceURL            string
	RequestHash          string
	ResponseHash         string
	RequestID            string
	HTTPStatus           int
	FetchedAt            time.Time
	Request              json.RawMessage
	Raw                  json.RawMessage
	Legs                 []DistanceLeg
	ResolvedOrigins      []ResolvedPoint
	ResolvedDestinations []ResolvedPoint
	PaidDrivingCalcs     int
	Notes                []string
}

func (c *Client) Distance(ctx context.Context, input DistanceRequest) (DistanceResult, error) {
	result := DistanceResult{SourceURL: c.baseURL + proximityPath}
	if len(input.Origins) != 1 {
		return result, errors.New("Mireye distance routing requires exactly one origin") //lint:ignore ST1005 provider name is a proper noun
	}
	if len(input.Destinations) < 1 || len(input.Destinations) > 5 {
		return result, errors.New("Mireye distance routing requires one to five destinations") //lint:ignore ST1005 provider name is a proper noun
	}
	if input.MaxCredits < 1 || input.MaxCredits > 60 {
		return result, errors.New("Mireye distance routing credit cap must be between 1 and 60") //lint:ignore ST1005 provider name is a proper noun
	}
	for _, locator := range append(append([]string{}, input.Origins...), input.Destinations...) {
		if strings.TrimSpace(locator) == "" || len(locator) > 256 {
			return result, errors.New("Mireye distance locator must contain one to 256 characters") //lint:ignore ST1005 provider name is a proper noun
		}
	}
	payload, err := json.Marshal(struct {
		Op           string   `json:"op"`
		Origins      []string `json:"origins"`
		Destinations []string `json:"destinations"`
		Mode         string   `json:"mode"`
		Units        string   `json:"units"`
		MaxCredits   int      `json:"max_credits"`
	}{"distance", input.Origins, input.Destinations, "driving", "miles", input.MaxCredits})
	if err != nil {
		return result, fmt.Errorf("encode Mireye distance request: %w", err)
	}
	result.Request = payload
	result.RequestHash = hash(append([]byte(http.MethodPost+" "+proximityPath+"\n"), payload...))

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, result.SourceURL, bytes.NewReader(payload))
	if err != nil {
		return result, fmt.Errorf("build Mireye distance request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("User-Agent", "pfas-load-control/0.1")
	response, err := c.http.Do(request)
	if err != nil {
		code := "MIREYE_NETWORK_ERROR"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = "MIREYE_TIMEOUT"
		}
		return result, &CallError{Code: code, Detail: "Mireye driving routes could not be reached", Retryable: true}
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	result.RequestID = response.Header.Get("X-Request-ID")
	result.FetchedAt = c.now().UTC()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return result, classifyStatus(response, Result{HTTPStatus: response.StatusCode, RequestID: result.RequestID, SourceURL: result.SourceURL})
	}
	if mediaType := strings.ToLower(response.Header.Get("Content-Type")); !strings.Contains(mediaType, "application/json") {
		return result, &CallError{Code: "MIREYE_INVALID_CONTENT_TYPE", Detail: "Mireye returned a non-JSON distance response", Status: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return result, &CallError{Code: "MIREYE_RESPONSE_READ_ERROR", Detail: "Mireye distance response could not be read", Status: response.StatusCode, Retryable: true}
	}
	if len(body) > maxResponseBytes {
		return result, &CallError{Code: "MIREYE_RESPONSE_TOO_LARGE", Detail: "Mireye distance response exceeded the allowed size", Status: response.StatusCode}
	}
	var wire struct {
		Op                   string          `json:"op"`
		Legs                 []DistanceLeg   `json:"legs"`
		ResolvedOrigins      []ResolvedPoint `json:"resolved_origins"`
		ResolvedDestinations []ResolvedPoint `json:"resolved_destinations"`
		PaidDrivingCalcs     int             `json:"paid_driving_calcs"`
		Notes                []string        `json:"notes"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye returned invalid distance JSON", Status: response.StatusCode}
	}
	if wire.Op != "distance" || len(wire.ResolvedOrigins) != len(input.Origins) || len(wire.ResolvedDestinations) != len(input.Destinations) {
		return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye distance response did not align with the request", Status: response.StatusCode}
	}
	if wire.PaidDrivingCalcs < 0 || wire.PaidDrivingCalcs > len(input.Origins)*len(input.Destinations) {
		return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye returned an invalid driving calculation count", Status: response.StatusCode}
	}
	for _, leg := range wire.Legs {
		if leg.OriginIndex != 0 || leg.DestinationIndex < 0 || leg.DestinationIndex >= len(input.Destinations) || leg.DistanceKM < 0 || leg.DistanceMiles < 0 {
			return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye returned an invalid distance leg", Status: response.StatusCode}
		}
		if leg.Flag != nil && *leg.Flag == "unreachable_or_snapped" && leg.DurationMinutes != nil {
			return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye returned a duration for an unreachable route", Status: response.StatusCode}
		}
	}
	result.ResponseHash = hash(body)
	result.Raw = append(json.RawMessage(nil), body...)
	result.Legs = wire.Legs
	result.ResolvedOrigins = wire.ResolvedOrigins
	result.ResolvedDestinations = wire.ResolvedDestinations
	result.PaidDrivingCalcs = wire.PaidDrivingCalcs
	result.Notes = wire.Notes
	return result, nil
}
