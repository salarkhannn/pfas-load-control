package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/party"
)

// Party routes

type partyInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type partyListInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	Role         string `query:"role" required:"false"`
}

type partyListOutput struct{ Body []party.Party }
type partyOutput struct{ Body party.Party }

type createPartyInput struct {
	WorkspaceKey string                 `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	Body         party.CreatePartyInput `json:"body"`
}

type updatePartyInput struct {
	WorkspaceKey string                 `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string                 `path:"id" format:"uuid"`
	Body         party.UpdatePartyInput `json:"body"`
}

// Consent routes

type consentListInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	PartyID      string `path:"partyId" format:"uuid"`
	Direction    string `query:"direction" required:"false"`
}

type consentListOutput struct{ Body []party.Consent }

type createConsentInput struct {
	WorkspaceKey string                   `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	Body         party.CreateConsentInput `json:"body"`
}

type consentOutput struct{ Body party.Consent }

type revokeConsentInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

// Field-Party routes

type fieldPartyListInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	FieldID      string `path:"fieldId" format:"uuid"`
}

type fieldPartyByPartyInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	PartyID      string `path:"partyId" format:"uuid"`
}

type fieldPartyListOutput struct{ Body []party.FieldParty }

type assignFieldPartyInput struct {
	WorkspaceKey string                      `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	Body         party.AssignFieldPartyInput `json:"body"`
}

type removeFieldPartyInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	FieldID      string `path:"fieldId" format:"uuid"`
	PartyID      string `path:"partyId" format:"uuid"`
}

func (a *API) registerPartyRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-parties", Method: http.MethodGet, Path: "/api/v1/parties",
		Summary: "List all parties in workspace", Tags: []string{"Parties"},
	}, a.listParties)
	huma.Register(api, huma.Operation{
		OperationID: "get-party", Method: http.MethodGet, Path: "/api/v1/parties/{id}",
		Summary: "Get a party", Tags: []string{"Parties"},
	}, a.getParty)
	huma.Register(api, huma.Operation{
		OperationID: "create-party", Method: http.MethodPost, Path: "/api/v1/parties",
		Summary: "Create a party", Tags: []string{"Parties"}, DefaultStatus: http.StatusCreated,
	}, a.createParty)
	huma.Register(api, huma.Operation{
		OperationID: "update-party", Method: http.MethodPut, Path: "/api/v1/parties/{id}",
		Summary: "Update a party", Tags: []string{"Parties"},
	}, a.updateParty)
	huma.Register(api, huma.Operation{
		OperationID: "delete-party", Method: http.MethodDelete, Path: "/api/v1/parties/{id}",
		Summary: "Delete a party", Tags: []string{"Parties"},
	}, a.deleteParty)

	// Consents
	huma.Register(api, huma.Operation{
		OperationID: "list-consents", Method: http.MethodGet, Path: "/api/v1/parties/{partyId}/consents",
		Summary: "List consents for a party", Tags: []string{"Consents"},
	}, a.listConsents)
	huma.Register(api, huma.Operation{
		OperationID: "create-consent", Method: http.MethodPost, Path: "/api/v1/consents",
		Summary: "Create a consent grant", Tags: []string{"Consents"}, DefaultStatus: http.StatusCreated,
	}, a.createConsent)
	huma.Register(api, huma.Operation{
		OperationID: "revoke-consent", Method: http.MethodPost, Path: "/api/v1/consents/{id}/revoke",
		Summary: "Revoke a consent", Tags: []string{"Consents"},
	}, a.revokeConsent)

	// Field-Party
	huma.Register(api, huma.Operation{
		OperationID: "list-parties-by-field", Method: http.MethodGet, Path: "/api/v1/fields/{fieldId}/parties",
		Summary: "List parties associated with a field", Tags: []string{"Field-Party"},
	}, a.listPartiesByField)
	huma.Register(api, huma.Operation{
		OperationID: "list-fields-by-party", Method: http.MethodGet, Path: "/api/v1/parties/{partyId}/fields",
		Summary: "List fields associated with a party", Tags: []string{"Field-Party"},
	}, a.listFieldsByParty)
	huma.Register(api, huma.Operation{
		OperationID: "assign-field-party", Method: http.MethodPost, Path: "/api/v1/field-parties",
		Summary: "Assign a party to a field", Tags: []string{"Field-Party"}, DefaultStatus: http.StatusCreated,
	}, a.assignFieldParty)
	huma.Register(api, huma.Operation{
		OperationID: "remove-field-party", Method: http.MethodDelete, Path: "/api/v1/fields/{fieldId}/parties/{partyId}",
		Summary: "Remove a party from a field", Tags: []string{"Field-Party"},
	}, a.removeFieldParty)
}

func (a *API) listParties(ctx context.Context, input *partyListInput) (*partyListOutput, error) {
	if input.Role != "" {
		parties, err := a.parties.ListByRole(ctx, input.WorkspaceKey, input.Role)
		if err != nil {
			return nil, a.partyError("list parties", err)
		}
		return &partyListOutput{Body: parties}, nil
	}
	parties, err := a.parties.List(ctx, input.WorkspaceKey)
	if err != nil {
		return nil, a.partyError("list parties", err)
	}
	return &partyListOutput{Body: parties}, nil
}

func (a *API) getParty(ctx context.Context, input *partyInput) (*partyOutput, error) {
	p, err := a.parties.Get(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.partyError("get party", err)
	}
	return &partyOutput{Body: p}, nil
}

func (a *API) createParty(ctx context.Context, input *createPartyInput) (*partyOutput, error) {
	p, err := a.parties.Create(ctx, input.WorkspaceKey, input.Body)
	if err != nil {
		return nil, a.partyError("create party", err)
	}
	return &partyOutput{Body: p}, nil
}

func (a *API) updateParty(ctx context.Context, input *updatePartyInput) (*partyOutput, error) {
	p, err := a.parties.Update(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.partyError("update party", err)
	}
	return &partyOutput{Body: p}, nil
}

func (a *API) deleteParty(ctx context.Context, input *partyInput) (*struct{}, error) {
	if err := a.parties.Delete(ctx, input.WorkspaceKey, input.ID); err != nil {
		return nil, a.partyError("delete party", err)
	}
	return &struct{}{}, nil
}

func (a *API) listConsents(ctx context.Context, input *consentListInput) (*consentListOutput, error) {
	consents, err := a.parties.ListConsents(ctx, input.WorkspaceKey, input.PartyID, input.Direction)
	if err != nil {
		return nil, a.partyError("list consents", err)
	}
	return &consentListOutput{Body: consents}, nil
}

func (a *API) createConsent(ctx context.Context, input *createConsentInput) (*consentOutput, error) {
	c, err := a.parties.CreateConsent(ctx, input.WorkspaceKey, input.Body)
	if err != nil {
		return nil, a.partyError("create consent", err)
	}
	return &consentOutput{Body: c}, nil
}

func (a *API) revokeConsent(ctx context.Context, input *revokeConsentInput) (*struct{}, error) {
	if err := a.parties.RevokeConsent(ctx, input.WorkspaceKey, input.ID); err != nil {
		return nil, a.partyError("revoke consent", err)
	}
	return &struct{}{}, nil
}

func (a *API) listPartiesByField(ctx context.Context, input *fieldPartyListInput) (*fieldPartyListOutput, error) {
	fp, err := a.parties.ListPartiesByField(ctx, input.WorkspaceKey, input.FieldID)
	if err != nil {
		return nil, a.partyError("list parties by field", err)
	}
	return &fieldPartyListOutput{Body: fp}, nil
}

func (a *API) listFieldsByParty(ctx context.Context, input *fieldPartyByPartyInput) (*fieldPartyListOutput, error) {
	fp, err := a.parties.ListFieldsByParty(ctx, input.WorkspaceKey, input.PartyID)
	if err != nil {
		return nil, a.partyError("list fields by party", err)
	}
	return &fieldPartyListOutput{Body: fp}, nil
}

func (a *API) assignFieldParty(ctx context.Context, input *assignFieldPartyInput) (*struct{}, error) {
	if err := a.parties.AssignFieldParty(ctx, input.WorkspaceKey, input.Body.FieldID, input.Body.PartyID, input.Body.Association); err != nil {
		return nil, a.partyError("assign field party", err)
	}
	return &struct{}{}, nil
}

func (a *API) removeFieldParty(ctx context.Context, input *removeFieldPartyInput) (*struct{}, error) {
	if err := a.parties.RemoveFieldParty(ctx, input.WorkspaceKey, input.FieldID, input.PartyID); err != nil {
		return nil, a.partyError("remove field party", err)
	}
	return &struct{}{}, nil
}

func (a *API) partyError(operation string, err error) error {
	if errors.Is(err, party.ErrNotFound) {
		return huma.Error404NotFound("not found")
	}
	if errors.Is(err, party.ErrInvalid) {
		return huma.Error400BadRequest(err.Error())
	}
	if errors.Is(err, party.ErrConsentExists) {
		return huma.Error409Conflict(err.Error())
	}
	a.logger.Error(operation, "error", err)
	return huma.Error500InternalServerError("internal error")
}
