package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/evidence"
)

type fieldEvaluationInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type evaluationInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type evaluationOutput struct{ Body evidence.Evaluation }

type createEvaluationOutput struct {
	Body struct {
		Evaluation evidence.Evaluation `json:"evaluation"`
		Created    bool                `json:"created"`
	}
}

func (a *API) registerEvidenceRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "start-physical-evaluation", Method: http.MethodPost,
		Path:    "/api/v1/candidate-fields/{id}/physical-evaluations",
		Summary: "Check physical conditions across a confirmed field", Tags: []string{"Physical evidence"},
		DefaultStatus: http.StatusAccepted,
	}, a.startPhysicalEvaluation)
	huma.Register(api, huma.Operation{
		OperationID: "get-latest-physical-evaluation", Method: http.MethodGet,
		Path:    "/api/v1/candidate-fields/{id}/physical-evaluations/latest",
		Summary: "Get the latest physical field evidence", Tags: []string{"Physical evidence"},
	}, a.latestPhysicalEvaluation)
	huma.Register(api, huma.Operation{
		OperationID: "get-physical-evaluation", Method: http.MethodGet,
		Path:    "/api/v1/physical-evaluations/{id}",
		Summary: "Get a physical evaluation and its sample evidence", Tags: []string{"Physical evidence"},
	}, a.getPhysicalEvaluation)
}

func (a *API) startPhysicalEvaluation(ctx context.Context, input *fieldEvaluationInput) (*createEvaluationOutput, error) {
	result, created, err := a.evidence.Start(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.evidenceError("start physical evaluation", err)
	}
	output := new(createEvaluationOutput)
	output.Body.Evaluation = result
	output.Body.Created = created
	return output, nil
}

func (a *API) latestPhysicalEvaluation(ctx context.Context, input *fieldEvaluationInput) (*evaluationOutput, error) {
	result, err := a.evidence.Latest(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.evidenceError("load latest physical evaluation", err)
	}
	return &evaluationOutput{Body: result}, nil
}

func (a *API) getPhysicalEvaluation(ctx context.Context, input *evaluationInput) (*evaluationOutput, error) {
	result, err := a.evidence.Get(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.evidenceError("load physical evaluation", err)
	}
	return &evaluationOutput{Body: result}, nil
}

func (a *API) evidenceError(operation string, err error) error {
	switch {
	case errors.Is(err, evidence.ErrNotFound):
		return huma.Error404NotFound("Physical field evidence not found.")
	case errors.Is(err, evidence.ErrInvalid):
		return huma.Error400BadRequest(cleanEvidenceError(err))
	case errors.Is(err, evidence.ErrConflict):
		return huma.Error409Conflict(cleanEvidenceError(err))
	default:
		a.logger.Error(operation+" failed", "error_type", fmt.Sprintf("%T", err))
		return huma.Error500InternalServerError("The physical field evidence could not be completed.")
	}
}

func cleanEvidenceError(err error) string {
	message := strings.TrimSpace(strings.TrimPrefix(err.Error(), evidence.ErrConflict.Error()+":"))
	message = strings.TrimSpace(strings.TrimPrefix(message, evidence.ErrInvalid.Error()+":"))
	if message == "" {
		return "The physical evidence request is invalid."
	}
	message = strings.ToUpper(message[:1]) + message[1:]
	if !strings.HasSuffix(message, ".") {
		message += "."
	}
	return message
}
