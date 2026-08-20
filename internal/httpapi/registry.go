package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/registry"
)

type registryEntryInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type registryListInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	EntryType    string `query:"entryType" required:"false"`
}

type registryListOutput struct{ Body []registry.RegistryEntry }
type registryEntryOutput struct{ Body registry.RegistryEntry }

type createRegistryEntryInput struct {
	WorkspaceKey string                            `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	Body         registry.RegistryCreateEntryInput `json:"body"`
}

type registrySearchInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	Query        string `query:"q" required:"true" minLength:"1"`
}

type registrySearchOutput struct{ Body []registry.RegistryEntry }

type registryNearbyInput struct {
	WorkspaceKey string  `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	Latitude     float64 `query:"lat" required:"true"`
	Longitude    float64 `query:"lng" required:"true"`
	EntryType    string  `query:"entryType" required:"true"`
	Limit        int     `query:"limit" required:"false"`
}

func (a *API) registerRegistryRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-registry-entries", Method: http.MethodGet, Path: "/api/v1/registry/entries",
		Summary: "List discovery registry entries", Tags: []string{"Registry"},
	}, a.listRegistryEntries)
	huma.Register(api, huma.Operation{
		OperationID: "get-registry-entry", Method: http.MethodGet, Path: "/api/v1/registry/entries/{id}",
		Summary: "Get a registry entry", Tags: []string{"Registry"},
	}, a.getRegistryEntry)
	huma.Register(api, huma.Operation{
		OperationID: "create-registry-entry", Method: http.MethodPost, Path: "/api/v1/registry/entries",
		Summary: "Create a registry entry", Tags: []string{"Registry"}, DefaultStatus: http.StatusCreated,
	}, a.createRegistryEntry)
	huma.Register(api, huma.Operation{
		OperationID: "delete-registry-entry", Method: http.MethodDelete, Path: "/api/v1/registry/entries/{id}",
		Summary: "Delete a registry entry", Tags: []string{"Registry"},
	}, a.deleteRegistryEntry)
	huma.Register(api, huma.Operation{
		OperationID: "search-registry", Method: http.MethodGet, Path: "/api/v1/registry/search",
		Summary: "Full-text search registry entries", Tags: []string{"Registry"},
	}, a.searchRegistry)
	huma.Register(api, huma.Operation{
		OperationID: "find-nearby-registry", Method: http.MethodGet, Path: "/api/v1/registry/nearby",
		Summary: "Find nearby registry entries by location", Tags: []string{"Registry"},
	}, a.findNearbyRegistry)
}

func (a *API) listRegistryEntries(ctx context.Context, input *registryListInput) (*registryListOutput, error) {
	entries, err := a.registry.List(ctx, input.WorkspaceKey, input.EntryType)
	if err != nil {
		return nil, a.registryError("list registry entries", err)
	}
	return &registryListOutput{Body: entries}, nil
}

func (a *API) getRegistryEntry(ctx context.Context, input *registryEntryInput) (*registryEntryOutput, error) {
	entries, err := a.registry.List(ctx, input.WorkspaceKey, "")
	if err != nil {
		return nil, a.registryError("get registry entry", err)
	}
	for _, e := range entries {
		if e.ID == input.ID {
			return &registryEntryOutput{Body: e}, nil
		}
	}
	return nil, huma.Error404NotFound("registry entry not found")
}

func (a *API) createRegistryEntry(ctx context.Context, input *createRegistryEntryInput) (*registryEntryOutput, error) {
	entry, err := a.registry.CreateEntry(ctx, input.WorkspaceKey, input.Body)
	if err != nil {
		return nil, a.registryError("create registry entry", err)
	}
	return &registryEntryOutput{Body: entry}, nil
}

func (a *API) deleteRegistryEntry(ctx context.Context, input *registryEntryInput) (*struct{}, error) {
	if err := a.registry.Delete(ctx, input.WorkspaceKey, input.ID); err != nil {
		return nil, a.registryError("delete registry entry", err)
	}
	return &struct{}{}, nil
}

func (a *API) searchRegistry(ctx context.Context, input *registrySearchInput) (*registrySearchOutput, error) {
	entries, err := a.registry.Search(ctx, input.WorkspaceKey, input.Query)
	if err != nil {
		return nil, a.registryError("search registry", err)
	}
	return &registrySearchOutput{Body: entries}, nil
}

func (a *API) findNearbyRegistry(ctx context.Context, input *registryNearbyInput) (*registryListOutput, error) {
	if input.Limit <= 0 || input.Limit > 50 {
		input.Limit = 20
	}
	entries, err := a.registry.FindNearby(ctx, input.WorkspaceKey, input.Latitude, input.Longitude, input.EntryType, input.Limit)
	if err != nil {
		return nil, a.registryError("find nearby registry", err)
	}
	return &registryListOutput{Body: entries}, nil
}

func (a *API) registryError(operation string, err error) error {
	if errors.Is(err, registry.ErrNotFound) {
		return huma.Error404NotFound("not found")
	}
	if errors.Is(err, registry.ErrInvalid) {
		return huma.Error400BadRequest(err.Error())
	}
	if errors.Is(err, registry.ErrExternal) {
		return huma.Error502BadGateway(err.Error())
	}
	a.logger.Error(operation, "error", err)
	return huma.Error500InternalServerError("internal error")
}
