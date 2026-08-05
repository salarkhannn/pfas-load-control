package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/field"
)

type fieldContextInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	FacilityID   string `query:"facilityId" required:"false"`
}

type fieldContextOutput struct{ Body field.FieldContext }

type candidateFieldInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type candidateFieldOutput struct{ Body field.Field }

type createCandidateFieldInput struct {
	WorkspaceKey string            `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	FacilityID   string            `path:"facilityId" format:"uuid"`
	Body         field.CreateInput `json:"body"`
}

type locationSelection struct {
	CandidateIndex int `json:"candidateIndex" minimum:"0" maximum:"2"`
}

type locationSelectionInput struct {
	WorkspaceKey string            `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string            `path:"id" format:"uuid"`
	Body         locationSelection `json:"body"`
}

type fieldGeometryInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
	Body         struct {
		GeoJSON string `json:"geojson" minLength:"1" maxLength:"1048576"`
	} `json:"body"`
}

type fieldDetailsInput struct {
	WorkspaceKey string             `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string             `path:"id" format:"uuid"`
	Body         field.DetailsInput `json:"body"`
}

type fieldImportForm struct {
	CSV huma.FormFile `form:"csv" required:"true" contentType:"text/csv,application/csv,application/vnd.ms-excel,text/plain,application/octet-stream"`
}

type fieldImportInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	FacilityID   string `path:"facilityId" format:"uuid"`
	RawBody      huma.MultipartFormFiles[fieldImportForm]
}

type fieldImportOutput struct{ Body field.Import }

func (a *API) registerFieldRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-field-context", Method: http.MethodGet, Path: "/api/v1/field-context",
		Summary: "Get facilities and candidate fields", Tags: []string{"Candidate fields"},
	}, a.fieldContext)
	huma.Register(api, huma.Operation{
		OperationID: "get-candidate-field", Method: http.MethodGet, Path: "/api/v1/candidate-fields/{id}",
		Summary: "Get one candidate field", Tags: []string{"Candidate fields"},
	}, a.getCandidateField)
	huma.Register(api, huma.Operation{
		OperationID: "create-candidate-field", Method: http.MethodPost, Path: "/api/v1/facilities/{facilityId}/candidate-fields",
		Summary: "Add a candidate field", Tags: []string{"Candidate fields"}, DefaultStatus: http.StatusCreated,
	}, a.createCandidateField)
	huma.Register(api, huma.Operation{
		OperationID: "import-candidate-fields", Method: http.MethodPost, Path: "/api/v1/facilities/{facilityId}/candidate-fields/import",
		Summary: "Import candidate fields from CSV", Tags: []string{"Candidate fields"}, DefaultStatus: http.StatusCreated,
		MaxBodyBytes: field.MaxCSVBytes + 64*1024,
	}, a.importCandidateFields)
	huma.Register(api, huma.Operation{
		OperationID: "resolve-candidate-field", Method: http.MethodPost, Path: "/api/v1/candidate-fields/{id}/resolution",
		Summary: "Resolve a field location with Mireye", Tags: []string{"Candidate fields"},
	}, a.resolveCandidateField)
	huma.Register(api, huma.Operation{
		OperationID: "select-field-location", Method: http.MethodPost, Path: "/api/v1/candidate-fields/{id}/location-selection",
		Summary: "Select the correct resolved location", Tags: []string{"Candidate fields"},
	}, a.selectFieldLocation)
	huma.Register(api, huma.Operation{
		OperationID: "confirm-field-parcel", Method: http.MethodPost, Path: "/api/v1/candidate-fields/{id}/parcel-confirmation",
		Summary: "Confirm a parcel as the actual field boundary", Tags: []string{"Candidate fields"},
	}, a.confirmFieldParcel)
	huma.Register(api, huma.Operation{
		OperationID: "set-field-geometry", Method: http.MethodPut, Path: "/api/v1/candidate-fields/{id}/geometry",
		Summary: "Set the actual field boundary", Tags: []string{"Candidate fields"},
	}, a.setFieldGeometry)
	huma.Register(api, huma.Operation{
		OperationID: "update-field-details", Method: http.MethodPut, Path: "/api/v1/candidate-fields/{id}/details",
		Summary: "Update field operating details", Tags: []string{"Candidate fields"},
	}, a.updateFieldDetails)
}

func (a *API) fieldContext(ctx context.Context, input *fieldContextInput) (*fieldContextOutput, error) {
	var facilityID *string
	if strings.TrimSpace(input.FacilityID) != "" {
		facilityID = &input.FacilityID
	}
	result, err := a.fields.Context(ctx, input.WorkspaceKey, facilityID)
	if err != nil {
		return nil, a.fieldError("load candidate fields", err)
	}
	return &fieldContextOutput{Body: result}, nil
}

func (a *API) getCandidateField(ctx context.Context, input *candidateFieldInput) (*candidateFieldOutput, error) {
	result, err := a.fields.Get(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.fieldError("load candidate field", err)
	}
	return &candidateFieldOutput{Body: result}, nil
}

func (a *API) createCandidateField(ctx context.Context, input *createCandidateFieldInput) (*candidateFieldOutput, error) {
	result, err := a.fields.Create(ctx, input.WorkspaceKey, input.FacilityID, input.Body)
	if err != nil {
		return nil, a.fieldError("create candidate field", err)
	}
	return &candidateFieldOutput{Body: result}, nil
}

func (a *API) importCandidateFields(ctx context.Context, input *fieldImportInput) (*fieldImportOutput, error) {
	form := input.RawBody.Data()
	defer form.CSV.Close()
	if strings.ToLower(filepath.Ext(form.CSV.Filename)) != ".csv" {
		return nil, huma.Error400BadRequest("Choose a CSV file.")
	}
	content, err := io.ReadAll(io.LimitReader(form.CSV, field.MaxCSVBytes+1))
	if err != nil {
		return nil, huma.Error400BadRequest("The CSV file could not be read.")
	}
	if len(content) > field.MaxCSVBytes {
		return nil, huma.Error413RequestEntityTooLarge("The CSV file must be 1 MiB or smaller.")
	}
	result, err := a.fields.ImportCSV(ctx, input.WorkspaceKey, input.FacilityID, content)
	if err != nil {
		return nil, a.fieldError("import candidate fields", err)
	}
	return &fieldImportOutput{Body: result}, nil
}

func (a *API) resolveCandidateField(ctx context.Context, input *candidateFieldInput) (*candidateFieldOutput, error) {
	result, err := a.fields.Resolve(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.fieldError("resolve candidate field", err)
	}
	return &candidateFieldOutput{Body: result}, nil
}

func (a *API) selectFieldLocation(ctx context.Context, input *locationSelectionInput) (*candidateFieldOutput, error) {
	result, err := a.fields.SelectCandidate(ctx, input.WorkspaceKey, input.ID, input.Body.CandidateIndex)
	if err != nil {
		return nil, a.fieldError("select field location", err)
	}
	return &candidateFieldOutput{Body: result}, nil
}

func (a *API) confirmFieldParcel(ctx context.Context, input *candidateFieldInput) (*candidateFieldOutput, error) {
	result, err := a.fields.ConfirmParcel(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.fieldError("confirm field parcel", err)
	}
	return &candidateFieldOutput{Body: result}, nil
}

func (a *API) setFieldGeometry(ctx context.Context, input *fieldGeometryInput) (*candidateFieldOutput, error) {
	result, err := a.fields.SetGeometry(ctx, input.WorkspaceKey, input.ID, input.Body.GeoJSON)
	if err != nil {
		return nil, a.fieldError("set field geometry", err)
	}
	return &candidateFieldOutput{Body: result}, nil
}

func (a *API) updateFieldDetails(ctx context.Context, input *fieldDetailsInput) (*candidateFieldOutput, error) {
	result, err := a.fields.UpdateDetails(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.fieldError("update field details", err)
	}
	return &candidateFieldOutput{Body: result}, nil
}

func (a *API) fieldError(operation string, err error) error {
	switch {
	case errors.Is(err, field.ErrNotFound):
		return huma.Error404NotFound("Candidate field not found.")
	case errors.Is(err, field.ErrInvalid):
		return huma.Error400BadRequest(cleanFieldError(err))
	case errors.Is(err, field.ErrConflict):
		return huma.Error409Conflict(cleanFieldError(err))
	case errors.Is(err, field.ErrExternal):
		return huma.Error503ServiceUnavailable("The field location could not be resolved right now. The field remains saved.")
	default:
		a.logger.Error(operation+" failed", "error_type", fmt.Sprintf("%T", err))
		return huma.Error500InternalServerError("The candidate field could not be updated.")
	}
}

func cleanFieldError(err error) string {
	message := err.Error()
	for _, sentinel := range []error{field.ErrInvalid, field.ErrConflict} {
		message = strings.TrimSpace(strings.TrimPrefix(message, sentinel.Error()+":"))
	}
	if message == "" {
		return "The candidate field input is invalid."
	}
	message = strings.ToUpper(message[:1]) + message[1:]
	if strings.HasSuffix(message, ".") {
		return message
	}
	return message + "."
}
