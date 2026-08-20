package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/application"
)

type applicationRecordInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type applicationListByFieldInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	FieldID      string `path:"fieldId" format:"uuid"`
}

type applicationListByContractorInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ContractorID string `path:"contractorId" format:"uuid"`
}

type applicationListOutput struct{ Body []application.AppRecord }
type applicationRecordOutput struct{ Body application.AppRecord }

type createApplicationRecordInput struct {
	WorkspaceKey string                           `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	Body         application.AppCreateRecordInput `json:"body"`
}

type loadingLedgerInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	FieldID      string `path:"fieldId" format:"uuid"`
	Year         int    `path:"year"`
}

type loadingLedgerOutput struct{ Body application.AppLoadingLedger }

type confirmApplicationInput struct {
	WorkspaceKey string                      `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string                      `path:"id" format:"uuid"`
	Body         application.AppConfirmInput `json:"body"`
}

type confirmationOutput struct{ Body application.AppConfirmation }

func (a *API) registerApplicationRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-application-record", Method: http.MethodGet, Path: "/api/v1/applications/{id}",
		Summary: "Get an application record", Tags: []string{"Applications"},
	}, a.getApplicationRecord)
	huma.Register(api, huma.Operation{
		OperationID: "list-application-records-by-field", Method: http.MethodGet, Path: "/api/v1/fields/{fieldId}/applications",
		Summary: "List application records for a field", Tags: []string{"Applications"},
	}, a.listApplicationsByField)
	huma.Register(api, huma.Operation{
		OperationID: "list-application-records-by-contractor", Method: http.MethodGet, Path: "/api/v1/parties/{contractorId}/applications",
		Summary: "List application records for a contractor", Tags: []string{"Applications"},
	}, a.listApplicationsByContractor)
	huma.Register(api, huma.Operation{
		OperationID: "create-application-record", Method: http.MethodPost, Path: "/api/v1/applications",
		Summary: "Create an application record", Tags: []string{"Applications"}, DefaultStatus: http.StatusCreated,
	}, a.createApplicationRecord)
	huma.Register(api, huma.Operation{
		OperationID: "get-loading-ledger", Method: http.MethodGet, Path: "/api/v1/fields/{fieldId}/loading/{year}",
		Summary: "Get field loading ledger for a year", Tags: []string{"Applications"},
	}, a.getLoadingLedger)
	huma.Register(api, huma.Operation{
		OperationID: "confirm-application", Method: http.MethodPost, Path: "/api/v1/applications/{id}/confirm",
		Summary: "Farmer confirms an application", Tags: []string{"Applications"},
	}, a.confirmApplication)
}

func (a *API) getApplicationRecord(ctx context.Context, input *applicationRecordInput) (*applicationRecordOutput, error) {
	rec, err := a.applications.GetRecord(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.applicationError("get application record", err)
	}
	return &applicationRecordOutput{Body: rec}, nil
}

func (a *API) listApplicationsByField(ctx context.Context, input *applicationListByFieldInput) (*applicationListOutput, error) {
	recs, err := a.applications.ListByField(ctx, input.WorkspaceKey, input.FieldID)
	if err != nil {
		return nil, a.applicationError("list applications by field", err)
	}
	return &applicationListOutput{Body: recs}, nil
}

func (a *API) listApplicationsByContractor(ctx context.Context, input *applicationListByContractorInput) (*applicationListOutput, error) {
	recs, err := a.applications.ListByContractor(ctx, input.WorkspaceKey, input.ContractorID)
	if err != nil {
		return nil, a.applicationError("list applications by contractor", err)
	}
	return &applicationListOutput{Body: recs}, nil
}

func (a *API) createApplicationRecord(ctx context.Context, input *createApplicationRecordInput) (*applicationRecordOutput, error) {
	rec, err := a.applications.CreateRecord(ctx, input.WorkspaceKey, input.Body)
	if err != nil {
		return nil, a.applicationError("create application record", err)
	}
	return &applicationRecordOutput{Body: rec}, nil
}

func (a *API) getLoadingLedger(ctx context.Context, input *loadingLedgerInput) (*loadingLedgerOutput, error) {
	ledger, err := a.applications.GetLoadingLedger(ctx, input.WorkspaceKey, input.FieldID, input.Year)
	if err != nil {
		return nil, a.applicationError("get loading ledger", err)
	}
	return &loadingLedgerOutput{Body: ledger}, nil
}

func (a *API) confirmApplication(ctx context.Context, input *confirmApplicationInput) (*confirmationOutput, error) {
	conf, err := a.applications.ConfirmApplication(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.applicationError("confirm application", err)
	}
	return &confirmationOutput{Body: conf}, nil
}

func (a *API) applicationError(operation string, err error) error {
	if errors.Is(err, application.ErrNotFound) {
		return huma.Error404NotFound("not found")
	}
	if errors.Is(err, application.ErrInvalid) {
		return huma.Error400BadRequest(err.Error())
	}
	a.logger.Error(operation, "error", err)
	return huma.Error500InternalServerError("internal error")
}
