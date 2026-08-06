package actioncenter

import "time"

type ExecutionMode string

const (
	ModeInternalRelease ExecutionMode = "INTERNAL_RELEASE"
	ModeOperatorHandoff ExecutionMode = "OPERATOR_HANDOFF"
	ModeControl         ExecutionMode = "CONTROL"
)

type ActionStatus string

const (
	StatusProposed ActionStatus = "PROPOSED"
	StatusApproved ActionStatus = "APPROVED"
	StatusRejected ActionStatus = "REJECTED"
	StatusExecuted ActionStatus = "EXECUTED"
)

type ActionAttachment struct {
	Label     string `json:"label"`
	Format    string `json:"format"`
	MediaType string `json:"mediaType"`
	SHA256    string `json:"sha256"`
	URL       string `json:"url"`
}

type ActionPayload struct {
	Channel     string             `json:"channel"`
	Recipient   string             `json:"recipient"`
	Subject     string             `json:"subject"`
	Message     string             `json:"message"`
	Attachments []ActionAttachment `json:"attachments"`
}

type ApprovalDecision struct {
	ID                   string    `json:"id"`
	Kind                 string    `json:"kind"`
	ActionRevision       int       `json:"actionRevision"`
	PayloadHash          string    `json:"payloadHash"`
	ActorName            string    `json:"actorName"`
	ActorRole            string    `json:"actorRole"`
	Note                 string    `json:"note,omitempty"`
	AcknowledgedGapCodes []string  `json:"acknowledgedGapCodes"`
	CreatedAt            time.Time `json:"createdAt"`
}

type ActionExecutionReceipt struct {
	ID             string    `json:"id"`
	Outcome        string    `json:"outcome"`
	Summary        string    `json:"summary"`
	ExternalEffect bool      `json:"externalEffect"`
	HandoffURL     string    `json:"handoffUrl,omitempty"`
	ReleaseID      string    `json:"releaseId,omitempty"`
	CompletedAt    time.Time `json:"completedAt"`
}

type ControlledAction struct {
	ID               string                  `json:"id"`
	PackageID        string                  `json:"packageId"`
	Position         int                     `json:"position"`
	Code             string                  `json:"code"`
	Category         string                  `json:"category"`
	Title            string                  `json:"title"`
	Detail           string                  `json:"detail"`
	Timing           string                  `json:"timing"`
	SourceID         string                  `json:"sourceId"`
	ExecutionMode    ExecutionMode           `json:"executionMode"`
	Status           ActionStatus            `json:"status"`
	ApprovalRequired bool                    `json:"approvalRequired"`
	Payload          ActionPayload           `json:"payload"`
	Revision         int                     `json:"revision"`
	PayloadHash      string                  `json:"payloadHash"`
	Decisions        []ApprovalDecision      `json:"decisions"`
	Execution        *ActionExecutionReceipt `json:"execution,omitempty"`
	CreatedAt        time.Time               `json:"createdAt"`
	UpdatedAt        time.Time               `json:"updatedAt"`
}

type Center struct {
	PackageID      string             `json:"packageId"`
	PackageHash    string             `json:"packageHash"`
	PackageStatus  string             `json:"packageStatus"`
	CriticalGaps   []ReviewGap        `json:"criticalGaps"`
	ReviewGaps     []ReviewGap        `json:"reviewGaps"`
	Actions        []ControlledAction `json:"actions"`
	ApprovalPolicy string             `json:"approvalPolicy"`
}

type ReviewGap struct {
	Code       string `json:"code"`
	Detail     string `json:"detail"`
	Resolution string `json:"resolution"`
}

type UpdatePayloadInput struct {
	Recipient string `json:"recipient" maxLength:"240"`
	Subject   string `json:"subject" minLength:"2" maxLength:"240"`
	Message   string `json:"message" minLength:"2" maxLength:"10000"`
}

type DecisionInput struct {
	ExpectedPayloadHash string `json:"expectedPayloadHash" minLength:"64" maxLength:"64"`
	ActorName           string `json:"actorName" minLength:"2" maxLength:"120"`
	ActorRole           string `json:"actorRole" minLength:"2" maxLength:"120"`
	Note                string `json:"note,omitempty" maxLength:"2000"`
	AcknowledgeGaps     bool   `json:"acknowledgeGaps"`
}

type ArtifactContent struct {
	Filename  string
	MediaType string
	Content   []byte
}
