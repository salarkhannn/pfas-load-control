package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/lab"
)

type labContextInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
}

type labContextOutput struct {
	Body lab.Context
}

type labReportInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type labReportOutput struct {
	Body lab.Report
}

type labUploadForm struct {
	FacilityName  string        `form:"facilityName" required:"true" minLength:"1" maxLength:"160"`
	BatchID       string        `form:"batchId" required:"true" minLength:"1" maxLength:"120"`
	WetMassKG     string        `form:"wetMassKg" required:"false"`
	PercentSolids string        `form:"percentSolids" required:"false"`
	Report        huma.FormFile `form:"report" required:"true" contentType:"application/pdf,text/csv,application/json,text/plain,application/octet-stream"`
}

type labUploadInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	RawBody      huma.MultipartFormFiles[labUploadForm]
}

type labUploadOutput struct {
	Body struct {
		Report  lab.Report `json:"report"`
		Created bool       `json:"created"`
	}
}

type labCorrectionInput struct {
	WorkspaceKey string         `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string         `path:"id" format:"uuid"`
	Body         lab.Correction `json:"body"`
}

type labContentOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	CacheControl       string `header:"Cache-Control"`
	Body               []byte
}

func (a *API) registerLabRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-lab-intake-context",
		Method:      http.MethodGet,
		Path:        "/api/v1/lab-context",
		Summary:     "Get facilities and batches available for laboratory intake",
		Tags:        []string{"Laboratory evidence"},
	}, a.labContext)
	huma.Register(api, huma.Operation{
		OperationID:   "upload-lab-report",
		Method:        http.MethodPost,
		Path:          "/api/v1/lab-reports",
		Summary:       "Store and begin extracting a laboratory report",
		Tags:          []string{"Laboratory evidence"},
		DefaultStatus: http.StatusCreated,
		MaxBodyBytes:  lab.MaxReportBytes + 64*1024,
	}, a.uploadLabReport)
	huma.Register(api, huma.Operation{
		OperationID: "get-lab-report",
		Method:      http.MethodGet,
		Path:        "/api/v1/lab-reports/{id}",
		Summary:     "Get one private laboratory report and its evidence",
		Tags:        []string{"Laboratory evidence"},
	}, a.getLabReport)
	huma.Register(api, huma.Operation{
		OperationID: "get-lab-report-content",
		Method:      http.MethodGet,
		Path:        "/api/v1/lab-reports/{id}/content",
		Summary:     "Get the private original laboratory report",
		Tags:        []string{"Laboratory evidence"},
	}, a.getLabReportContent)
	huma.Register(api, huma.Operation{
		OperationID: "correct-lab-report",
		Method:      http.MethodPut,
		Path:        "/api/v1/lab-reports/{id}/evidence",
		Summary:     "Create a corrected laboratory evidence version",
		Tags:        []string{"Laboratory evidence"},
	}, a.correctLabReport)
	huma.Register(api, huma.Operation{
		OperationID: "confirm-lab-report",
		Method:      http.MethodPost,
		Path:        "/api/v1/lab-reports/{id}/confirmation",
		Summary:     "Confirm the current laboratory evidence version",
		Tags:        []string{"Laboratory evidence"},
	}, a.confirmLabReport)
}

func (a *API) labContext(ctx context.Context, input *labContextInput) (*labContextOutput, error) {
	result, err := a.lab.Context(ctx, input.WorkspaceKey)
	if err != nil {
		return nil, a.labError("load laboratory intake", err)
	}
	return &labContextOutput{Body: result}, nil
}

func (a *API) uploadLabReport(ctx context.Context, input *labUploadInput) (*labUploadOutput, error) {
	form := input.RawBody.Data()
	defer form.Report.Close()
	content, err := io.ReadAll(io.LimitReader(form.Report, lab.MaxReportBytes+1))
	if err != nil {
		return nil, huma.Error400BadRequest("The laboratory report could not be read.")
	}
	if len(content) > lab.MaxReportBytes {
		return nil, huma.Error413RequestEntityTooLarge("The laboratory report must be 10 MiB or smaller.")
	}
	report, created, err := a.lab.Create(ctx, input.WorkspaceKey, lab.Intake{
		FacilityName:  form.FacilityName,
		BatchID:       form.BatchID,
		WetMassKG:     optionalFormValue(form.WetMassKG),
		PercentSolids: optionalFormValue(form.PercentSolids),
		Filename:      form.Report.Filename,
		MediaType:     form.Report.ContentType,
		Content:       content,
	})
	if err != nil {
		return nil, a.labError("upload laboratory report", err)
	}
	output := new(labUploadOutput)
	output.Body.Report = report
	output.Body.Created = created
	return output, nil
}

func (a *API) getLabReport(ctx context.Context, input *labReportInput) (*labReportOutput, error) {
	report, err := a.lab.Get(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.labError("load laboratory report", err)
	}
	return &labReportOutput{Body: report}, nil
}

func (a *API) getLabReportContent(ctx context.Context, input *labReportInput) (*labContentOutput, error) {
	filename, mediaType, content, err := a.lab.Content(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.labError("load laboratory source", err)
	}
	disposition := mime.FormatMediaType("inline", map[string]string{"filename": filename})
	return &labContentOutput{
		ContentType:        mediaType,
		ContentDisposition: disposition,
		CacheControl:       "private, no-store",
		Body:               content,
	}, nil
}

func (a *API) correctLabReport(ctx context.Context, input *labCorrectionInput) (*labReportOutput, error) {
	report, err := a.lab.Correct(ctx, input.WorkspaceKey, input.ID, input.Body)
	if err != nil {
		return nil, a.labError("correct laboratory evidence", err)
	}
	return &labReportOutput{Body: report}, nil
}

func (a *API) confirmLabReport(ctx context.Context, input *labReportInput) (*labReportOutput, error) {
	report, err := a.lab.Confirm(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.labError("confirm laboratory evidence", err)
	}
	return &labReportOutput{Body: report}, nil
}

func (a *API) labError(operation string, err error) error {
	switch {
	case errors.Is(err, lab.ErrNotFound):
		return huma.Error404NotFound("Laboratory report not found.")
	case errors.Is(err, lab.ErrInvalid):
		return huma.Error400BadRequest(cleanLabError(err))
	case errors.Is(err, lab.ErrConflict):
		return huma.Error409Conflict(cleanLabError(err))
	default:
		a.logger.Error(operation+" failed", "error_type", fmt.Sprintf("%T", err))
		return huma.Error500InternalServerError("The laboratory evidence could not be updated.")
	}
}

func cleanLabError(err error) string {
	message := err.Error()
	for _, prefix := range []string{lab.ErrInvalid.Error() + ": ", lab.ErrConflict.Error() + ": "} {
		message = strings.TrimPrefix(message, prefix)
	}
	if message == "" {
		return "The laboratory evidence is invalid."
	}
	message = strings.ToUpper(message[:1]) + message[1:]
	if strings.HasSuffix(message, ".") {
		return message
	}
	return message + "."
}

func optionalFormValue(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
