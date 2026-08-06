package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/responseplan"
)

type responseDecisionInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type resolveFacilityLocationInput struct {
	WorkspaceKey string                     `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string                     `path:"id" format:"uuid"`
	Body         responseplan.LocationInput `required:"true"`
}

type facilityLocationInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type startResponseInput struct {
	WorkspaceKey string                  `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string                  `path:"id" format:"uuid"`
	Body         responseplan.StartInput `required:"true"`
}

type responseRunInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type facilityLocationOutput struct{ Body responseplan.FacilityLocation }
type responseRunOutput struct{ Body responseplan.ResponseRun }
type startResponseOutput struct {
	Body struct {
		Run     responseplan.ResponseRun `json:"run"`
		Created bool                     `json:"created"`
	}
}

func (a *API) registerResponseRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "resolve-response-facility-location", Method: http.MethodPost,
		Path: "/api/v1/policy-decisions/{id}/facility-location", Summary: "Resolve the treatment plant location",
		Tags: []string{"Elevated and prohibited response"}, DefaultStatus: http.StatusCreated,
	}, a.resolveResponseFacilityLocation)
	huma.Register(api, huma.Operation{
		OperationID: "confirm-response-facility-location", Method: http.MethodPost,
		Path: "/api/v1/facility-locations/{id}/confirmation", Summary: "Confirm the treatment plant location",
		Tags: []string{"Elevated and prohibited response"},
	}, a.confirmResponseFacilityLocation)
	huma.Register(api, huma.Operation{
		OperationID: "get-latest-response-facility-location", Method: http.MethodGet,
		Path: "/api/v1/policy-decisions/{id}/facility-location/latest", Summary: "Get the latest treatment plant location lookup",
		Tags: []string{"Elevated and prohibited response"},
	}, a.latestResponseFacilityLocation)
	huma.Register(api, huma.Operation{
		OperationID: "create-pfas-response", Method: http.MethodPost,
		Path: "/api/v1/policy-decisions/{id}/response", Summary: "Build the elevated or prohibited PFAS response",
		Tags: []string{"Elevated and prohibited response"}, DefaultStatus: http.StatusCreated,
	}, a.createPFASResponse)
	huma.Register(api, huma.Operation{
		OperationID: "get-latest-pfas-response", Method: http.MethodGet,
		Path: "/api/v1/policy-decisions/{id}/response/latest", Summary: "Get the latest elevated or prohibited PFAS response",
		Tags: []string{"Elevated and prohibited response"},
	}, a.latestPFASResponse)
	huma.Register(api, huma.Operation{
		OperationID: "get-pfas-response", Method: http.MethodGet,
		Path: "/api/v1/response-runs/{id}", Summary: "Get an elevated or prohibited PFAS response",
		Tags: []string{"Elevated and prohibited response"},
	}, a.getPFASResponse)
}

func (a *API) latestResponseFacilityLocation(ctx context.Context, input *responseDecisionInput) (*facilityLocationOutput, error) {
	result, err := a.response.LatestLocation(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.responseError("load latest response facility location", err)
	}
	return &facilityLocationOutput{Body: result}, nil
}

func (a *API) resolveResponseFacilityLocation(ctx context.Context, input *resolveFacilityLocationInput) (*facilityLocationOutput, error) {
	result, err := a.response.ResolveLocation(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.responseError("resolve response facility location", err)
	}
	return &facilityLocationOutput{Body: result}, nil
}

func (a *API) confirmResponseFacilityLocation(ctx context.Context, input *facilityLocationInput) (*facilityLocationOutput, error) {
	result, err := a.response.ConfirmLocation(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.responseError("confirm response facility location", err)
	}
	return &facilityLocationOutput{Body: result}, nil
}

func (a *API) createPFASResponse(ctx context.Context, input *startResponseInput) (*startResponseOutput, error) {
	result, created, err := a.response.Start(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.responseError("create PFAS response", err)
	}
	output := new(startResponseOutput)
	output.Body.Run, output.Body.Created = result, created
	return output, nil
}

func (a *API) latestPFASResponse(ctx context.Context, input *responseDecisionInput) (*responseRunOutput, error) {
	result, err := a.response.Latest(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.responseError("load latest PFAS response", err)
	}
	return &responseRunOutput{Body: result}, nil
}

func (a *API) getPFASResponse(ctx context.Context, input *responseRunInput) (*responseRunOutput, error) {
	result, err := a.response.Get(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.responseError("load PFAS response", err)
	}
	return &responseRunOutput{Body: result}, nil
}

func (a *API) responseError(operation string, err error) error {
	switch {
	case errors.Is(err, responseplan.ErrNotFound):
		return huma.Error404NotFound("PFAS response not found")
	case errors.Is(err, responseplan.ErrInvalid):
		return huma.Error400BadRequest(strings.TrimPrefix(err.Error(), responseplan.ErrInvalid.Error()+": "))
	case errors.Is(err, responseplan.ErrConflict):
		return huma.Error409Conflict(strings.TrimPrefix(err.Error(), responseplan.ErrConflict.Error()+": "))
	default:
		a.logger.Error(operation+" failed", "error_type", typeName(err))
		return huma.Error500InternalServerError("PFAS response could not be prepared")
	}
}
