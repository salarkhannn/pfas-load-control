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

const maxFetchResponseBytes = 16 << 20

type Coordinate struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
}

type FetchBatchRequest struct {
	Locations []Coordinate `json:"locations"`
	Fields    []string     `json:"fields"`
}

type FetchBatchResult struct {
	SourceURL    string
	RequestHash  string
	ResponseHash string
	RequestID    string
	HTTPStatus   int
	FetchedAt    time.Time
	Request      json.RawMessage
	Raw          json.RawMessage
	Results      []FetchLocationResult
}

type FetchLocationResult struct {
	Index           int                   `json:"index"`
	OK              bool                  `json:"ok"`
	Latitude        *float64              `json:"lat,omitempty"`
	Longitude       *float64              `json:"lng,omitempty"`
	FetchedAt       *time.Time            `json:"fetched_at,omitempty"`
	Fields          map[string]FetchField `json:"fields,omitempty"`
	PartialFailures []FetchFailure        `json:"partial_failures,omitempty"`
	Resolved        *ResolvedLocation     `json:"resolved_location,omitempty"`
	Error           *FetchEntryError      `json:"error,omitempty"`
}

type ResolvedLocation struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
	Source    string  `json:"source"`
}

type FetchField struct {
	Value          json.RawMessage `json:"value"`
	Unit           *string         `json:"unit"`
	Source         *string         `json:"source"`
	SourceURL      *string         `json:"source_url"`
	Confidence     *string         `json:"confidence"`
	FetchedAt      *time.Time      `json:"fetched_at"`
	DatasetVintage *string         `json:"dataset_vintage"`
	TTLSeconds     *int            `json:"ttl_seconds"`
	Notes          *string         `json:"notes"`
	Status         string          `json:"status"`
	Error          *string         `json:"error"`
	Retryable      bool            `json:"retryable"`
}

type FetchFailure struct {
	Field     string `json:"field"`
	Source    string `json:"source"`
	Error     string `json:"error"`
	Retryable bool   `json:"retryable"`
}

type FetchEntryError struct {
	Code      string `json:"error"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (c *Client) FetchBatch(ctx context.Context, input FetchBatchRequest) (FetchBatchResult, error) {
	result := FetchBatchResult{SourceURL: c.baseURL + "/v1/fetch/batch"}
	if err := validateBatchRequest(input); err != nil {
		return result, err
	}
	requestBody, err := json.Marshal(input)
	if err != nil {
		return result, fmt.Errorf("encode Mireye batch request: %w", err)
	}
	result.Request = requestBody
	result.RequestHash = hash(requestBody)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, result.SourceURL, bytes.NewReader(requestBody))
	if err != nil {
		return result, fmt.Errorf("build Mireye batch request: %w", err)
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
		return result, &CallError{Code: code, Detail: "Mireye physical data could not be reached", Retryable: true}
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	result.RequestID = response.Header.Get("X-Request-ID")
	result.FetchedAt = c.now().UTC()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return result, classifyStatus(response, Result{HTTPStatus: response.StatusCode, RequestID: result.RequestID, SourceURL: result.SourceURL})
	}
	if mediaType := strings.ToLower(response.Header.Get("Content-Type")); !strings.Contains(mediaType, "application/json") {
		return result, &CallError{Code: "MIREYE_INVALID_CONTENT_TYPE", Detail: "Mireye returned a non-JSON batch response", Status: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxFetchResponseBytes+1))
	if err != nil {
		return result, &CallError{Code: "MIREYE_RESPONSE_READ_ERROR", Detail: "Mireye batch response could not be read", Status: response.StatusCode, Retryable: true}
	}
	if len(body) > maxFetchResponseBytes {
		return result, &CallError{Code: "MIREYE_RESPONSE_TOO_LARGE", Detail: "Mireye batch response exceeded the allowed size", Status: response.StatusCode}
	}
	var payload struct {
		Results []FetchLocationResult `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye batch response was not valid JSON", Status: response.StatusCode}
	}
	if len(payload.Results) != len(input.Locations) {
		return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye batch response was not aligned to the requested locations", Status: response.StatusCode}
	}
	for index, item := range payload.Results {
		if item.Index != index {
			return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye batch response changed location order", Status: response.StatusCode}
		}
		if item.OK && item.Fields == nil {
			return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye batch result omitted requested fields", Status: response.StatusCode}
		}
		if !item.OK && item.Error == nil {
			return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: "Mireye batch result omitted its location error", Status: response.StatusCode}
		}
	}
	result.Raw = body
	result.ResponseHash = hash(body)
	result.Results = payload.Results
	return result, nil
}

func validateBatchRequest(input FetchBatchRequest) error {
	if len(input.Locations) == 0 || len(input.Locations) > 25 {
		return errors.New("mireye batch requests require 1 to 25 locations")
	}
	if len(input.Fields) == 0 || len(input.Fields) > 50 {
		return errors.New("mireye batch requests require 1 to 50 fields")
	}
	seen := make(map[string]struct{}, len(input.Fields))
	for _, field := range input.Fields {
		if strings.TrimSpace(field) == "" {
			return errors.New("mireye field names cannot be empty")
		}
		if _, exists := seen[field]; exists {
			return fmt.Errorf("mireye field %q was requested more than once", field)
		}
		seen[field] = struct{}{}
	}
	for _, location := range input.Locations {
		if location.Latitude < 18 || location.Latitude > 72 || location.Longitude < -180 || location.Longitude > -65 {
			return errors.New("mireye coordinates must fall inside the documented US envelope")
		}
	}
	return nil
}
