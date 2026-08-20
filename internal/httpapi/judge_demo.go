package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/salarkhannn/pfas-load-control/internal/judgedemo"
)

type judgeDemoRunOutput struct {
	Body judgedemo.DemoRun
}

type judgeDemoRunInput struct {
	ID string `path:"id" format:"uuid"`
}

type createJudgeDemoRunInput struct {
	IdempotencyKey string `header:"Idempotency-Key" minLength:"1" maxLength:"200" doc:"Unique key for this replay attempt. Reusing it returns the original durable run without executing tools again."`
}

type createReviewedJudgeDemoRunInput struct {
	ID             string `path:"id" format:"uuid"`
	IdempotencyKey string `header:"Idempotency-Key" minLength:"1" maxLength:"200" doc:"Unique key for this reviewed-evidence replay. Reusing it returns the original durable run."`
}

type judgeDemoPackageOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	CacheControl       string `header:"Cache-Control"`
	Body               []byte
}

func (a *API) registerJudgeDemoRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-judge-demo-run", Method: http.MethodPost,
		Path: "/api/v1/judge-demo/runs", Summary: "Replay the frozen judging case through the decision agent",
		Description: "Executes frozen inputs through the production lab parser, policy classifier, evidence aggregation, placement engine, and package freezer; persists actual tool inputs, outputs, provenance, allocations, citations, and the package hash.",
		Tags:        []string{"Judge demo"}, DefaultStatus: http.StatusCreated,
	}, a.createJudgeDemoRun)
	huma.Register(api, huma.Operation{
		OperationID: "create-reviewed-judge-demo-run", Method: http.MethodPost,
		Path: "/api/v1/judge-demo/runs/{id}/reviewed", Summary: "Apply the seeded reviewed boundary evidence and rerun the decision agent",
		Description: "Loads and verifies the immutable boundary and source artifacts, reruns placement, records the evidence-verification tool call, and freezes a new package linked to the unresolved run.",
		Tags:        []string{"Judge demo"}, DefaultStatus: http.StatusCreated,
	}, a.createReviewedJudgeDemoRun)
	huma.Register(api, huma.Operation{
		OperationID: "get-judge-demo-run", Method: http.MethodGet,
		Path: "/api/v1/judge-demo/runs/{id}", Summary: "Retrieve a completed judging-case replay",
		Tags: []string{"Judge demo"},
	}, a.getJudgeDemoRun)
	huma.Register(api, huma.Operation{
		OperationID: "download-judge-demo-package", Method: http.MethodGet,
		Path: "/api/v1/judge-demo/runs/{id}/package", Summary: "Download the exact verified frozen judging package",
		Tags: []string{"Judge demo"},
	}, a.downloadJudgeDemoPackage)
}

func (a *API) downloadJudgeDemoPackage(ctx context.Context, input *judgeDemoRunInput) (*judgeDemoPackageOutput, error) {
	artifact, err := a.judgeDemo.PackageArtifact(ctx, input.ID)
	if errors.Is(err, judgedemo.ErrNotFound) {
		return nil, huma.Error404NotFound("judge demo run not found")
	}
	if errors.Is(err, judgedemo.ErrIntegrity) {
		return nil, huma.Error409Conflict("judge demo package failed integrity verification")
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("judge demo package could not be loaded")
	}
	return &judgeDemoPackageOutput{ContentType: "application/json", ContentDisposition: mime.FormatMediaType("attachment", map[string]string{"filename": "judge-demo-package-" + input.ID + ".json"}), CacheControl: "private, no-store", Body: artifact}, nil
}

func (a *API) createJudgeDemoRun(ctx context.Context, input *createJudgeDemoRunInput) (*judgeDemoRunOutput, error) {
	run, err := a.judgeDemo.Start(ctx, input.IdempotencyKey)
	if errors.Is(err, judgedemo.ErrInvalid) {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	if err != nil {
		a.logger.Error("judge demo run failed", "error_type", typeName(err))
		return nil, huma.Error500InternalServerError("judge demo run could not be completed")
	}
	return &judgeDemoRunOutput{Body: run}, nil
}

func (a *API) createReviewedJudgeDemoRun(ctx context.Context, input *createReviewedJudgeDemoRunInput) (*judgeDemoRunOutput, error) {
	run, err := a.judgeDemo.StartReviewed(ctx, input.IdempotencyKey, input.ID)
	if errors.Is(err, judgedemo.ErrNotFound) {
		return nil, huma.Error404NotFound("unresolved judge demo run not found")
	}
	if errors.Is(err, judgedemo.ErrInvalid) {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	if err != nil {
		a.logger.Error("reviewed judge demo run failed", "error_type", typeName(err))
		return nil, huma.Error500InternalServerError("reviewed judge demo run could not be completed")
	}
	return &judgeDemoRunOutput{Body: run}, nil
}

func (a *API) getJudgeDemoRun(ctx context.Context, input *judgeDemoRunInput) (*judgeDemoRunOutput, error) {
	run, err := a.judgeDemo.Get(ctx, input.ID)
	if errors.Is(err, judgedemo.ErrNotFound) {
		return nil, huma.Error404NotFound("judge demo run not found")
	}
	if errors.Is(err, judgedemo.ErrIntegrity) {
		return nil, huma.Error409Conflict("judge demo package failed integrity verification")
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("judge demo run could not be loaded")
	}
	return &judgeDemoRunOutput{Body: run}, nil
}
