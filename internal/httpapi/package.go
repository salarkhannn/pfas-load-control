package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/decisionpackage"
)

type decisionPackageInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
}

type packageExportInput struct {
	WorkspaceKey string `header:"X-Workspace-Key" minLength:"43" maxLength:"128"`
	ID           string `path:"id" format:"uuid"`
	Format       string `path:"format" enum:"html,pdf,json"`
}

type decisionPackageOutput struct {
	Body decisionpackage.DecisionPackage
}

type createDecisionPackageOutput struct {
	Body struct {
		Package decisionpackage.DecisionPackage `json:"package"`
		Created bool                            `json:"created"`
	}
}

type packageExportOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	CacheControl       string `header:"Cache-Control"`
	Body               []byte
}

func (a *API) registerDecisionPackageRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-decision-package", Method: http.MethodPost,
		Path: "/api/v1/policy-decisions/{id}/decision-packages", Summary: "Freeze the current decision, evidence, and proposed actions",
		Tags: []string{"Decision packages"}, DefaultStatus: http.StatusCreated,
	}, a.createDecisionPackage)
	huma.Register(api, huma.Operation{
		OperationID: "get-latest-decision-package", Method: http.MethodGet,
		Path: "/api/v1/policy-decisions/{id}/decision-packages/latest", Summary: "Get the latest decision package",
		Tags: []string{"Decision packages"},
	}, a.latestDecisionPackage)
	huma.Register(api, huma.Operation{
		OperationID: "get-decision-package", Method: http.MethodGet,
		Path: "/api/v1/decision-packages/{id}", Summary: "Get one immutable decision package",
		Tags: []string{"Decision packages"},
	}, a.getDecisionPackage)
	huma.Register(api, huma.Operation{
		OperationID: "export-decision-package", Method: http.MethodGet,
		Path: "/api/v1/decision-packages/{id}/exports/{format}", Summary: "Download an immutable HTML, PDF, or JSON package artifact",
		Tags: []string{"Decision packages"},
	}, a.exportDecisionPackage)
}

func (a *API) createDecisionPackage(ctx context.Context, input *decisionPackageInput) (*createDecisionPackageOutput, error) {
	result, created, err := a.packages.Create(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.decisionPackageError("create decision package", err)
	}
	if _, err := a.actions.Ensure(ctx, input.WorkspaceKey, result.ID); err != nil {
		return nil, a.actionError("prepare package actions", err)
	}
	output := new(createDecisionPackageOutput)
	output.Body.Package = result
	output.Body.Created = created
	return output, nil
}

func (a *API) latestDecisionPackage(ctx context.Context, input *decisionPackageInput) (*decisionPackageOutput, error) {
	result, err := a.packages.Latest(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.decisionPackageError("load latest decision package", err)
	}
	return &decisionPackageOutput{Body: result}, nil
}

func (a *API) getDecisionPackage(ctx context.Context, input *decisionPackageInput) (*decisionPackageOutput, error) {
	result, err := a.packages.Get(ctx, input.WorkspaceKey, input.ID)
	if err != nil {
		return nil, a.decisionPackageError("load decision package", err)
	}
	return &decisionPackageOutput{Body: result}, nil
}

func (a *API) exportDecisionPackage(ctx context.Context, input *packageExportInput) (*packageExportOutput, error) {
	artifact, err := a.packages.Artifact(ctx, input.WorkspaceKey, input.ID, input.Format)
	if err != nil {
		return nil, a.decisionPackageError("export decision package", err)
	}
	return &packageExportOutput{
		ContentType: artifact.MediaType, ContentDisposition: mime.FormatMediaType("attachment", map[string]string{"filename": artifact.Filename}),
		CacheControl: "private, no-store", Body: artifact.Content,
	}, nil
}

func (a *API) decisionPackageError(operation string, err error) error {
	switch {
	case errors.Is(err, decisionpackage.ErrNotFound):
		return huma.Error404NotFound("Decision package not found.")
	case errors.Is(err, decisionpackage.ErrInvalid):
		return huma.Error400BadRequest(cleanDecisionPackageError(err))
	case errors.Is(err, decisionpackage.ErrConflict):
		return huma.Error409Conflict(cleanDecisionPackageError(err))
	default:
		a.logger.Error(operation+" failed", "error_type", typeName(err))
		return huma.Error500InternalServerError("The decision package could not be prepared.")
	}
}

func cleanDecisionPackageError(err error) string {
	message := err.Error()
	for _, prefix := range []string{decisionpackage.ErrInvalid.Error() + ": ", decisionpackage.ErrConflict.Error() + ": "} {
		message = strings.TrimPrefix(message, prefix)
	}
	if message == "" {
		return "The decision package request is invalid."
	}
	message = strings.ToUpper(message[:1]) + message[1:]
	if strings.HasSuffix(message, ".") {
		return message
	}
	return message + "."
}
