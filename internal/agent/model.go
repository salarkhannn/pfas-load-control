package agent

import (
	"encoding/json"
	"time"
)

type Run struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	NextStep    int        `json:"nextStep"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	Steps       []Step     `json:"steps"`
	ToolCalls   []ToolCall `json:"toolCalls"`
	DataGaps    []DataGap  `json:"dataGaps"`
}

type Step struct {
	ID           string          `json:"id"`
	Position     int             `json:"position"`
	ToolName     string          `json:"toolName"`
	Status       string          `json:"status"`
	AttemptCount int             `json:"attemptCount"`
	Summary      json.RawMessage `json:"summary,omitempty"`
	StartedAt    *time.Time      `json:"startedAt,omitempty"`
	CompletedAt  *time.Time      `json:"completedAt,omitempty"`
}

type ToolCall struct {
	ID           string    `json:"id"`
	StepID       string    `json:"stepId"`
	Attempt      int       `json:"attempt"`
	Status       string    `json:"status"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	RequestHash  string    `json:"requestHash"`
	ResponseHash *string   `json:"responseHash,omitempty"`
	RequestID    *string   `json:"requestId,omitempty"`
	SourceURL    string    `json:"sourceUrl"`
	HTTPStatus   *int32    `json:"httpStatus,omitempty"`
	DurationMS   int64     `json:"durationMs"`
	CreditCost   int32     `json:"creditCost"`
	ErrorCode    *string   `json:"errorCode,omitempty"`
	FetchedAt    time.Time `json:"fetchedAt"`
}

type DataGap struct {
	ID         string     `json:"id"`
	StepID     *string    `json:"stepId,omitempty"`
	Code       string     `json:"code"`
	Detail     string     `json:"detail"`
	Resolution string     `json:"resolution"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}
