package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/actioncenter"
	"github.com/salarkhannn/pfas-load-control/internal/agent"
	"github.com/salarkhannn/pfas-load-control/internal/decisionpackage"
	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/field"
	"github.com/salarkhannn/pfas-load-control/internal/lab"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
	"github.com/salarkhannn/pfas-load-control/internal/policy"
	"github.com/salarkhannn/pfas-load-control/internal/responseplan"
)

type API struct {
	service   *agent.Service
	lab       *lab.Service
	policy    *policy.Service
	fields    *field.Service
	evidence  *evidence.Service
	placement *placement.Service
	response  *responseplan.Service
	packages  *decisionpackage.Service
	actions   *actioncenter.Service
	pool      *pgxpool.Pool
	logger    *slog.Logger
}

type runOutput struct {
	Body agent.Run
}

type createRunOutput struct {
	Body struct {
		Run     agent.Run `json:"run"`
		Created bool      `json:"created" description:"True when this request created the run; false when an active run was reused."`
	}
}

type runInput struct {
	ID string `path:"id" format:"uuid"`
}

type healthOutput struct {
	Body struct {
		Status string `json:"status" enum:"ok"`
	}
}

func NewRouter(service *agent.Service, labService *lab.Service, policyService *policy.Service, fieldService *field.Service, evidenceService *evidence.Service, placementService *placement.Service, responseService *responseplan.Service, packageService *decisionpackage.Service, actionService *actioncenter.Service, pool *pgxpool.Pool, logger *slog.Logger, webOrigin string) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedWebOrigins(webOrigin),
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Content-Type", "X-Workspace-Key", "Idempotency-Key"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	config := huma.DefaultConfig("PFAS Load Control API", "0.8.0")
	config.Info.Description = "A bounded, auditable control plane for PFAS decision workflows."
	api := humachi.New(router, config)
	handler := &API{service: service, lab: labService, policy: policyService, fields: fieldService, evidence: evidenceService, placement: placementService, response: responseService, packages: packageService, actions: actionService, pool: pool, logger: logger}

	huma.Register(api, huma.Operation{
		OperationID: "health-live",
		Method:      http.MethodGet,
		Path:        "/health/live",
		Summary:     "Check process liveness",
		Tags:        []string{"Health"},
	}, handler.live)
	huma.Register(api, huma.Operation{
		OperationID: "health-ready",
		Method:      http.MethodGet,
		Path:        "/health/ready",
		Summary:     "Check database readiness",
		Tags:        []string{"Health"},
	}, handler.ready)
	huma.Register(api, huma.Operation{
		OperationID:   "create-readiness-run",
		Method:        http.MethodPost,
		Path:          "/api/v1/readiness-runs",
		Summary:       "Start the fixed Mireye readiness agent",
		Description:   "Creates a run with exactly three read-only, zero-credit Mireye checks. An active run is returned instead of duplicated.",
		Tags:          []string{"Readiness"},
		DefaultStatus: http.StatusCreated,
	}, handler.createRun)
	huma.Register(api, huma.Operation{
		OperationID: "get-latest-readiness-run",
		Method:      http.MethodGet,
		Path:        "/api/v1/readiness-runs/latest",
		Summary:     "Get the most recent readiness run",
		Tags:        []string{"Readiness"},
	}, handler.latestRun)
	huma.Register(api, huma.Operation{
		OperationID: "get-readiness-run",
		Method:      http.MethodGet,
		Path:        "/api/v1/readiness-runs/{id}",
		Summary:     "Get a readiness run and its evidence trace",
		Tags:        []string{"Readiness"},
	}, handler.getRun)
	handler.registerLabRoutes(api)
	handler.registerPolicyRoutes(api)
	handler.registerFieldRoutes(api)
	handler.registerEvidenceRoutes(api)
	handler.registerPlacementRoutes(api)
	handler.registerResponseRoutes(api)
	handler.registerDecisionPackageRoutes(api)
	handler.registerActionRoutes(api)
	return router
}

func allowedWebOrigins(configured string) []string {
	origins := []string{configured}
	parsed, err := url.Parse(configured)
	if err != nil {
		return origins
	}
	port := parsed.Port()
	switch parsed.Hostname() {
	case "localhost":
		parsed.Host = "127.0.0.1"
	case "127.0.0.1":
		parsed.Host = "localhost"
	default:
		return origins
	}
	if port != "" {
		parsed.Host += ":" + port
	}
	return append(origins, parsed.String())
}

func (a *API) live(_ context.Context, _ *struct{}) (*healthOutput, error) {
	output := new(healthOutput)
	output.Body.Status = "ok"
	return output, nil
}

func (a *API) ready(ctx context.Context, _ *struct{}) (*healthOutput, error) {
	if err := a.pool.Ping(ctx); err != nil {
		a.logger.Error("readiness check failed", "error_type", typeName(err))
		return nil, huma.Error503ServiceUnavailable("database is not ready")
	}
	output := new(healthOutput)
	output.Body.Status = "ok"
	return output, nil
}

func (a *API) createRun(ctx context.Context, _ *struct{}) (*createRunOutput, error) {
	run, created, err := a.service.CreateReadinessRun(ctx)
	if err != nil {
		a.logger.Error("create readiness run failed", "error_type", typeName(err))
		return nil, huma.Error500InternalServerError("readiness run could not be created")
	}
	output := new(createRunOutput)
	output.Body.Run = run
	output.Body.Created = created
	return output, nil
}

func (a *API) latestRun(ctx context.Context, _ *struct{}) (*runOutput, error) {
	run, err := a.service.GetLatestRun(ctx)
	if errors.Is(err, agent.ErrNotFound) {
		return nil, huma.Error404NotFound("no readiness run exists")
	}
	if err != nil {
		a.logger.Error("get latest readiness run failed", "error_type", typeName(err))
		return nil, huma.Error500InternalServerError("readiness run could not be loaded")
	}
	return &runOutput{Body: run}, nil
}

func (a *API) getRun(ctx context.Context, input *runInput) (*runOutput, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("run ID must be a UUID")
	}
	run, err := a.service.GetRun(ctx, id)
	if errors.Is(err, agent.ErrNotFound) {
		return nil, huma.Error404NotFound("readiness run not found")
	}
	if err != nil {
		a.logger.Error("get readiness run failed", "error_type", typeName(err))
		return nil, huma.Error500InternalServerError("readiness run could not be loaded")
	}
	return &runOutput{Body: run}, nil
}

func typeName(value error) string {
	return fmt.Sprintf("%T", value)
}
