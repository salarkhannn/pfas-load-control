package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/coordination"
)

type workflowInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type workflowListInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
}

type workflowListOutput struct{ Body []coordination.CoordWorkflow }

type workflowDetailOutput struct {
	Body struct {
		Workflow coordination.CoordWorkflow `json:"workflow"`
		Steps    []coordination.CoordStep   `json:"steps"`
	}
}

type createWorkflowInput struct {
	WorkspaceKey string                                `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	Body         coordination.CoordCreateWorkflowInput `json:"body"`
}

type confirmStepInput struct {
	WorkspaceKey string                             `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	StepID       string                             `path:"stepId" format:"uuid"`
	Body         coordination.CoordConfirmStepInput `json:"body"`
}

type assignStepInput struct {
	WorkspaceKey string                            `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	StepID       string                            `path:"stepId" format:"uuid"`
	Body         coordination.CoordAssignStepInput `json:"body"`
}

type rejectStepInput = confirmStepInput

type stepOutput struct{ Body coordination.CoordStep }

type createDocumentInput struct {
	WorkspaceKey string                                `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	WorkflowID   string                                `path:"workflowId" format:"uuid"`
	Body         coordination.CoordCreateDocumentInput `json:"body"`
}

type documentOutput struct{ Body coordination.CoordDocument }

type documentListInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	WorkflowID   string `path:"workflowId" format:"uuid"`
}

type documentListOutput struct{ Body []coordination.CoordDocument }

type notificationListInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	PartyID      string `path:"partyId" format:"uuid"`
}

type notificationListOutput struct {
	Body []coordination.CoordNotification
}

type markNotificationReadInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

func (a *API) registerCoordinationRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-coordination-workflows", Method: http.MethodGet, Path: "/api/v1/coordination/workflows",
		Summary: "List coordination workflows", Tags: []string{"Coordination"},
	}, a.listWorkflows)
	huma.Register(api, huma.Operation{
		OperationID: "get-coordination-workflow", Method: http.MethodGet, Path: "/api/v1/coordination/workflows/{id}",
		Summary: "Get a workflow and its steps", Tags: []string{"Coordination"},
	}, a.getWorkflow)
	huma.Register(api, huma.Operation{
		OperationID: "create-coordination-workflow", Method: http.MethodPost, Path: "/api/v1/coordination/workflows",
		Summary: "Create a coordination workflow", Tags: []string{"Coordination"}, DefaultStatus: http.StatusCreated,
	}, a.createWorkflow)
	huma.Register(api, huma.Operation{
		OperationID: "assign-coordination-step", Method: http.MethodPost, Path: "/api/v1/coordination/steps/{stepId}/assign",
		Summary: "Assign a party without confirming the step", Tags: []string{"Coordination"},
	}, a.assignStep)
	huma.Register(api, huma.Operation{
		OperationID: "confirm-coordination-step", Method: http.MethodPost, Path: "/api/v1/coordination/steps/{stepId}/confirm",
		Summary: "Confirm a coordination step", Tags: []string{"Coordination"},
	}, a.confirmStep)
	huma.Register(api, huma.Operation{
		OperationID: "reject-coordination-step", Method: http.MethodPost, Path: "/api/v1/coordination/steps/{stepId}/reject",
		Summary: "Reject a coordination step", Tags: []string{"Coordination"},
	}, a.rejectStep)
	huma.Register(api, huma.Operation{
		OperationID: "upload-coordination-document", Method: http.MethodPost, Path: "/api/v1/coordination/workflows/{workflowId}/documents",
		Summary: "Upload a document to a workflow", Tags: []string{"Coordination"}, DefaultStatus: http.StatusCreated,
	}, a.uploadDocument)
	huma.Register(api, huma.Operation{
		OperationID: "list-coordination-documents", Method: http.MethodGet, Path: "/api/v1/coordination/workflows/{workflowId}/documents",
		Summary: "List documents for a workflow", Tags: []string{"Coordination"},
	}, a.listDocuments)
	huma.Register(api, huma.Operation{
		OperationID: "list-coordination-notifications", Method: http.MethodGet, Path: "/api/v1/parties/{partyId}/notifications",
		Summary: "List notifications for a party", Tags: []string{"Coordination"},
	}, a.listNotifications)
	huma.Register(api, huma.Operation{
		OperationID: "mark-notification-read", Method: http.MethodPost, Path: "/api/v1/notifications/{id}/read",
		Summary: "Mark a notification as read", Tags: []string{"Coordination"},
	}, a.markNotificationRead)
}

func (a *API) assignStep(ctx context.Context, input *assignStepInput) (*stepOutput, error) {
	step, err := a.coordination.AssignStep(ctx, input.WorkspaceKey, input.StepID, input.Body)
	if err != nil {
		return nil, a.coordinationError("assign step", err)
	}
	return &stepOutput{Body: step}, nil
}

func (a *API) listWorkflows(ctx context.Context, input *workflowListInput) (*workflowListOutput, error) {
	wfs, err := a.coordination.ListWorkflows(ctx, input.WorkspaceKey)
	if err != nil {
		return nil, a.coordinationError("list workflows", err)
	}
	return &workflowListOutput{Body: wfs}, nil
}

func (a *API) getWorkflow(ctx context.Context, input *workflowInput) (*workflowDetailOutput, error) {
	wf, steps, err := a.coordination.GetWorkflow(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.coordinationError("get workflow", err)
	}
	return &workflowDetailOutput{Body: struct {
		Workflow coordination.CoordWorkflow `json:"workflow"`
		Steps    []coordination.CoordStep   `json:"steps"`
	}{Workflow: wf, Steps: steps}}, nil
}

func (a *API) createWorkflow(ctx context.Context, input *createWorkflowInput) (*workflowListOutput, error) {
	// We need a single Workflow output, but Huma doesn't support single-object with custom struct easily
	// Use the list type wrapper but return single item
	wf, err := a.coordination.CreateWorkflow(ctx, input.WorkspaceKey, input.Body)
	if err != nil {
		return nil, a.coordinationError("create workflow", err)
	}
	return &workflowListOutput{Body: []coordination.CoordWorkflow{wf}}, nil
}

func (a *API) confirmStep(ctx context.Context, input *confirmStepInput) (*stepOutput, error) {
	step, err := a.coordination.ConfirmStep(ctx, input.WorkspaceKey, input.StepID, input.Body)
	if err != nil {
		return nil, a.coordinationError("confirm step", err)
	}
	return &stepOutput{Body: step}, nil
}

func (a *API) rejectStep(ctx context.Context, input *rejectStepInput) (*stepOutput, error) {
	step, err := a.coordination.RejectStep(ctx, input.WorkspaceKey, input.StepID, input.Body)
	if err != nil {
		return nil, a.coordinationError("reject step", err)
	}
	return &stepOutput{Body: step}, nil
}

func (a *API) uploadDocument(ctx context.Context, input *createDocumentInput) (*documentOutput, error) {
	doc, err := a.coordination.CreateDocument(ctx, input.WorkspaceKey, input.WorkflowID, input.Body)
	if err != nil {
		return nil, a.coordinationError("upload document", err)
	}
	return &documentOutput{Body: doc}, nil
}

func (a *API) listDocuments(ctx context.Context, input *documentListInput) (*documentListOutput, error) {
	docs, err := a.coordination.ListDocuments(ctx, input.WorkspaceKey, input.WorkflowID)
	if err != nil {
		return nil, a.coordinationError("list documents", err)
	}
	return &documentListOutput{Body: docs}, nil
}

func (a *API) listNotifications(ctx context.Context, input *notificationListInput) (*notificationListOutput, error) {
	notifs, err := a.coordination.ListNotifications(ctx, input.WorkspaceKey, input.PartyID)
	if err != nil {
		return nil, a.coordinationError("list notifications", err)
	}
	return &notificationListOutput{Body: notifs}, nil
}

func (a *API) markNotificationRead(ctx context.Context, input *markNotificationReadInput) (*struct{}, error) {
	if err := a.coordination.MarkNotificationRead(ctx, input.WorkspaceKey, input.ID); err != nil {
		return nil, a.coordinationError("mark notification read", err)
	}
	return &struct{}{}, nil
}

func (a *API) coordinationError(operation string, err error) error {
	if errors.Is(err, coordination.ErrNotFound) {
		return huma.Error404NotFound("not found")
	}
	if errors.Is(err, coordination.ErrInvalid) || errors.Is(err, coordination.ErrInvalidTransition) {
		return huma.Error400BadRequest(err.Error())
	}
	a.logger.Error(operation, "error", err)
	return huma.Error500InternalServerError("internal error")
}
