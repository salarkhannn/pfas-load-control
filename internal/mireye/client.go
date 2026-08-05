package mireye

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

type Tool string

const (
	ToolFields Tool = "mireye.meta.fields"
	ToolPlans  Tool = "mireye.meta.plans"
	ToolUsage  Tool = "mireye.users.me.usage"
)

type Result struct {
	Tool         Tool
	Path         string
	SourceURL    string
	RequestHash  string
	ResponseHash string
	RequestID    string
	ETag         string
	CacheControl string
	HTTPStatus   int
	Duration     time.Duration
	FetchedAt    time.Time
	Raw          json.RawMessage
	Summary      any
}

type CallError struct {
	Code       string
	Detail     string
	Status     int
	Retryable  bool
	RetryAfter time.Duration
	Result     Result
}

func (e *CallError) Error() string {
	return e.Code + ": " + e.Detail
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
	now     func() time.Time
}

func NewClient(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 130 * time.Second}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    httpClient,
		now:     time.Now,
	}
}

func (c *Client) Call(ctx context.Context, tool Tool) (Result, error) {
	path, validator, err := toolSpec(tool)
	if err != nil {
		return Result{}, err
	}

	startedAt := c.now()
	result := Result{
		Tool:        tool,
		Path:        path,
		SourceURL:   c.baseURL + path,
		RequestHash: hash([]byte(http.MethodGet + " " + path)),
		FetchedAt:   startedAt.UTC(),
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, result.SourceURL, nil)
	if err != nil {
		return result, fmt.Errorf("build Mireye request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("User-Agent", "pfas-load-control/0.1")

	response, err := c.http.Do(request)
	result.Duration = c.now().Sub(startedAt)
	if err != nil {
		code := "MIREYE_NETWORK_ERROR"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = "MIREYE_TIMEOUT"
		}
		return result, &CallError{Code: code, Detail: "Mireye could not be reached", Retryable: true, Result: result}
	}
	defer response.Body.Close()

	result.HTTPStatus = response.StatusCode
	result.RequestID = response.Header.Get("X-Request-ID")
	result.ETag = response.Header.Get("ETag")
	result.CacheControl = response.Header.Get("Cache-Control")
	result.FetchedAt = c.now().UTC()
	result.Duration = c.now().Sub(startedAt)

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return result, classifyStatus(response, result)
	}
	if mediaType := strings.ToLower(response.Header.Get("Content-Type")); !strings.Contains(mediaType, "application/json") {
		return result, &CallError{Code: "MIREYE_INVALID_CONTENT_TYPE", Detail: "Mireye returned a non-JSON response", Status: response.StatusCode, Result: result}
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return result, &CallError{Code: "MIREYE_RESPONSE_READ_ERROR", Detail: "Mireye response could not be read", Status: response.StatusCode, Retryable: true, Result: result}
	}
	if len(body) > maxResponseBytes {
		return result, &CallError{Code: "MIREYE_RESPONSE_TOO_LARGE", Detail: "Mireye response exceeded the allowed size", Status: response.StatusCode, Result: result}
	}

	summary, err := validator(body)
	if err != nil {
		return result, &CallError{Code: "MIREYE_SCHEMA_MISMATCH", Detail: err.Error(), Status: response.StatusCode, Result: result}
	}
	result.Raw = json.RawMessage(body)
	result.ResponseHash = hash(body)
	result.Summary = summary
	return result, nil
}

type validateFunc func([]byte) (any, error)

func toolSpec(tool Tool) (string, validateFunc, error) {
	switch tool {
	case ToolFields:
		return "/v1/meta/fields", validateFields, nil
	case ToolPlans:
		return "/v1/meta/plans", validatePlans, nil
	case ToolUsage:
		return "/v1/users/me/usage", validateUsage, nil
	default:
		return "", nil, fmt.Errorf("unsupported Mireye tool %q", tool)
	}
}

func classifyStatus(response *http.Response, result Result) error {
	callErr := &CallError{
		Code:   "MIREYE_HTTP_ERROR",
		Detail: "Mireye rejected the readiness request",
		Status: response.StatusCode,
		Result: result,
	}
	switch response.StatusCode {
	case http.StatusUnauthorized:
		callErr.Code = "MIREYE_AUTHENTICATION_FAILED"
		callErr.Detail = "Mireye did not accept the configured API token"
	case http.StatusForbidden:
		callErr.Code = "MIREYE_ACCESS_DENIED"
		callErr.Detail = "The Mireye account cannot access this readiness endpoint"
	case http.StatusTooManyRequests:
		callErr.Code = "MIREYE_RATE_LIMITED"
		callErr.Detail = "Mireye rate-limited the readiness request"
		callErr.Retryable = true
		callErr.RetryAfter = parseRetryAfter(response.Header.Get("Retry-After"), time.Now())
	default:
		if response.StatusCode >= http.StatusInternalServerError {
			callErr.Code = "MIREYE_SERVER_ERROR"
			callErr.Detail = "Mireye returned a server error"
			callErr.Retryable = true
		}
	}
	return callErr
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if target, err := http.ParseTime(value); err == nil && target.After(now) {
		return target.Sub(now)
	}
	return 0
}

func hash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
