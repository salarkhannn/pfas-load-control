package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/policy"
)

type policyDecisionInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type policyDecisionOutput struct {
	Body policy.Decision
}

type createPolicyDecisionOutput struct {
	Body struct {
		Decision policy.Decision `json:"decision"`
		Created  bool            `json:"created"`
	}
}

type activeRulePackOutput struct {
	Body policy.RulePack
}

func (a *API) registerPolicyRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-policy-classification", Method: http.MethodGet,
		Path: "/api/v1/lab-reports/{id}/classification", Summary: "Get the report's immutable policy classification",
		Tags: []string{"Policy classification"},
	}, a.getPolicyClassification)
	huma.Register(api, huma.Operation{
		OperationID: "classify-lab-report", Method: http.MethodPost,
		Path: "/api/v1/lab-reports/{id}/classification", Summary: "Classify confirmed evidence with the active reviewed rule pack",
		Tags: []string{"Policy classification"}, DefaultStatus: http.StatusCreated,
	}, a.classifyLabReport)
	huma.Register(api, huma.Operation{
		OperationID: "get-active-michigan-rule-pack", Method: http.MethodGet,
		Path: "/api/v1/policy/rule-packs/mi/active", Summary: "Get the active reviewed Michigan PFAS rule pack",
		Tags: []string{"Policy classification"},
	}, a.getActiveMichiganRulePack)
}

func (a *API) getPolicyClassification(ctx context.Context, input *policyDecisionInput) (*policyDecisionOutput, error) {
	decision, err := a.policy.Get(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.policyError("load policy classification", err)
	}
	return &policyDecisionOutput{Body: decision}, nil
}

func (a *API) classifyLabReport(ctx context.Context, input *policyDecisionInput) (*createPolicyDecisionOutput, error) {
	decision, created, err := a.policy.Classify(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.policyError("classify laboratory report", err)
	}
	output := new(createPolicyDecisionOutput)
	output.Body.Decision = decision
	output.Body.Created = created
	return output, nil
}

func (a *API) getActiveMichiganRulePack(ctx context.Context, _ *struct{}) (*activeRulePackOutput, error) {
	pack, err := a.policy.ActiveRulePack(ctx, "MI")
	if err != nil {
		return nil, a.policyError("load active Michigan rule pack", err)
	}
	return &activeRulePackOutput{Body: pack}, nil
}

func (a *API) policyError(operation string, err error) error {
	switch {
	case errors.Is(err, policy.ErrNotFound):
		return huma.Error404NotFound("Policy classification not found.")
	case errors.Is(err, policy.ErrNotConfirmed):
		return huma.Error409Conflict("Confirm the laboratory evidence before classification.")
	case errors.Is(err, policy.ErrRulePack):
		a.logger.Error(operation+" failed", "error_type", fmt.Sprintf("%T", err))
		return huma.Error503ServiceUnavailable("An active reviewed policy version is unavailable.")
	default:
		a.logger.Error(operation+" failed", "error_type", fmt.Sprintf("%T", err))
		return huma.Error500InternalServerError("The policy classification could not be completed.")
	}
}
