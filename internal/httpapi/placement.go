package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/placement"
)

type placementInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type createPlacementInput struct {
	WorkspaceKey string              `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string              `path:"id" format:"uuid"`
	Body         placement.PlanInput `required:"true"`
}

type placementOutput struct{ Body placement.PlacementPlan }

type createPlacementOutput struct {
	Body struct {
		Evaluation placement.PlacementPlan `json:"evaluation"`
		Created    bool                    `json:"created"`
	}
}

func (a *API) registerPlacementRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-placement-evaluation", Method: http.MethodPost,
		Path: "/api/v1/policy-decisions/{id}/placement", Summary: "Compare fields and build a draft placement plan",
		Tags: []string{"Placement"}, DefaultStatus: http.StatusCreated,
	}, a.createPlacement)
	huma.Register(api, huma.Operation{
		OperationID: "get-latest-placement-evaluation", Method: http.MethodGet,
		Path: "/api/v1/policy-decisions/{id}/placement/latest", Summary: "Get the latest draft placement plan",
		Tags: []string{"Placement"},
	}, a.latestPlacement)
}

func (a *API) createPlacement(ctx context.Context, input *createPlacementInput) (*createPlacementOutput, error) {
	result, created, err := a.placement.Create(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.placementError("create placement evaluation", err)
	}
	output := new(createPlacementOutput)
	output.Body.Evaluation = result
	output.Body.Created = created
	return output, nil
}

func (a *API) latestPlacement(ctx context.Context, input *placementInput) (*placementOutput, error) {
	result, err := a.placement.Latest(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.placementError("load latest placement evaluation", err)
	}
	return &placementOutput{Body: result}, nil
}

func (a *API) placementError(operation string, err error) error {
	switch {
	case errors.Is(err, placement.ErrNotFound):
		return huma.Error404NotFound("placement plan not found")
	case errors.Is(err, placement.ErrInvalid):
		return huma.Error400BadRequest(strings.TrimPrefix(err.Error(), placement.ErrInvalid.Error()+": "))
	case errors.Is(err, placement.ErrConflict):
		return huma.Error409Conflict(strings.TrimPrefix(err.Error(), placement.ErrConflict.Error()+": "))
	default:
		a.logger.Error(operation+" failed", "error_type", typeName(err))
		return huma.Error500InternalServerError("placement plan could not be prepared")
	}
}
