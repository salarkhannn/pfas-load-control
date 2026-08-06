package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/actioncenter"
)

type packageActionInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type actionInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type updateActionInput struct {
	WorkspaceKey string                          `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string                          `path:"id" format:"uuid"`
	Body         actioncenter.UpdatePayloadInput `json:"body"`
}

type decideActionInput struct {
	WorkspaceKey string                     `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string                     `path:"id" format:"uuid"`
	Body         actioncenter.DecisionInput `json:"body"`
}

type executeActionInput struct {
	WorkspaceKey   string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	IdempotencyKey string `header:"Idempotency-Key" minLength:"16" maxLength:"128"`
	ID             string `path:"id" format:"uuid"`
}

type actionCenterOutput struct {
	Body actioncenter.Center
}

type actionOutput struct {
	Body actioncenter.ControlledAction
}

type handoffOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	CacheControl       string `header:"Cache-Control"`
	Body               []byte
}

func (a *API) registerActionRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "prepare-action-center", Method: http.MethodPost,
		Path: "/api/v1/decision-packages/{id}/action-center", Summary: "Prepare controlled actions from an immutable decision package",
		Tags: []string{"Action center"},
	}, a.prepareActionCenter)
	huma.Register(api, huma.Operation{
		OperationID: "get-action-center", Method: http.MethodGet,
		Path: "/api/v1/decision-packages/{id}/action-center", Summary: "Review exact action payloads, decisions, and receipts",
		Tags: []string{"Action center"},
	}, a.getActionCenter)
	huma.Register(api, huma.Operation{
		OperationID: "update-action-payload", Method: http.MethodPut,
		Path: "/api/v1/actions/{id}/payload", Summary: "Edit an action payload and invalidate any earlier approval",
		Tags: []string{"Action center"},
	}, a.updateActionPayload)
	huma.Register(api, huma.Operation{
		OperationID: "approve-action", Method: http.MethodPost,
		Path: "/api/v1/actions/{id}/approve", Summary: "Approve the exact current action payload",
		Tags: []string{"Action center"},
	}, a.approveAction)
	huma.Register(api, huma.Operation{
		OperationID: "reject-action", Method: http.MethodPost,
		Path: "/api/v1/actions/{id}/reject", Summary: "Reject the exact current action payload",
		Tags: []string{"Action center"},
	}, a.rejectAction)
	huma.Register(api, huma.Operation{
		OperationID: "execute-action", Method: http.MethodPost,
		Path: "/api/v1/actions/{id}/execute", Summary: "Release an approved internal plan or prepare an approved operator handoff",
		Description: "Execution is idempotent. Operator handoffs never contact an external system or person.",
		Tags:        []string{"Action center"},
	}, a.executeAction)
	huma.Register(api, huma.Operation{
		OperationID: "download-action-handoff", Method: http.MethodGet,
		Path: "/api/v1/execution-attempts/{id}/handoff", Summary: "Download the exact approved operator handoff",
		Tags: []string{"Action center"},
	}, a.downloadActionHandoff)
}

func (a *API) prepareActionCenter(ctx context.Context, input *packageActionInput) (*actionCenterOutput, error) {
	result, err := a.actions.Ensure(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.actionError("prepare action center", err)
	}
	return &actionCenterOutput{Body: result}, nil
}

func (a *API) getActionCenter(ctx context.Context, input *packageActionInput) (*actionCenterOutput, error) {
	result, err := a.actions.Get(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.actionError("load action center", err)
	}
	return &actionCenterOutput{Body: result}, nil
}

func (a *API) updateActionPayload(ctx context.Context, input *updateActionInput) (*actionOutput, error) {
	result, err := a.actions.UpdatePayload(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.actionError("update action payload", err)
	}
	return &actionOutput{Body: result}, nil
}

func (a *API) approveAction(ctx context.Context, input *decideActionInput) (*actionOutput, error) {
	result, err := a.actions.Approve(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.actionError("approve action", err)
	}
	return &actionOutput{Body: result}, nil
}

func (a *API) rejectAction(ctx context.Context, input *decideActionInput) (*actionOutput, error) {
	result, err := a.actions.Reject(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.actionError("reject action", err)
	}
	return &actionOutput{Body: result}, nil
}

func (a *API) executeAction(ctx context.Context, input *executeActionInput) (*actionOutput, error) {
	result, err := a.actions.Execute(ctx, input.WorkspaceKey, input.ID, input.IdempotencyKey)
	if err != nil {
		return nil, a.actionError("execute action", err)
	}
	return &actionOutput{Body: result}, nil
}

func (a *API) downloadActionHandoff(ctx context.Context, input *actionInput) (*handoffOutput, error) {
	artifact, err := a.actions.Handoff(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.actionError("download action handoff", err)
	}
	return &handoffOutput{
		ContentType:        artifact.MediaType,
		ContentDisposition: mime.FormatMediaType("attachment", map[string]string{"filename": artifact.Filename}),
		CacheControl:       "private, no-store", Body: artifact.Content,
	}, nil
}

func (a *API) actionError(operation string, err error) error {
	switch {
	case errors.Is(err, actioncenter.ErrNotFound):
		return huma.Error404NotFound("Action not found.")
	case errors.Is(err, actioncenter.ErrInvalid):
		return huma.Error400BadRequest(cleanActionError(err))
	case errors.Is(err, actioncenter.ErrStale):
		return huma.Error409Conflict(cleanActionError(err))
	case errors.Is(err, actioncenter.ErrConflict):
		return huma.Error409Conflict(cleanActionError(err))
	default:
		a.logger.Error(operation+" failed", "error_type", typeName(err))
		return huma.Error500InternalServerError("The action could not be completed.")
	}
}

func cleanActionError(err error) string {
	message := err.Error()
	for _, sentinel := range []error{actioncenter.ErrInvalid, actioncenter.ErrConflict, actioncenter.ErrStale} {
		message = strings.TrimPrefix(message, sentinel.Error()+": ")
	}
	if message == "" {
		return "The action request is invalid."
	}
	message = strings.ToUpper(message[:1]) + message[1:]
	if strings.HasSuffix(message, ".") {
		return message
	}
	return message + "."
}
