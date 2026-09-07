// Package api exposes Assay's typed REST API through Huma.
package api

import (
	"log/slog"
	"net/http"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

type handler struct {
	analytics   *domain.AnalyticsService
	api         huma.API
	service     *domain.Service
	traces      *domain.TraceService
	evaluations *domain.EvaluationService
	adminToken  string
	logger      *slog.Logger
}

// Dependencies contains process-scoped collaborators used by REST handlers.
type Dependencies struct {
	Analytics   *domain.AnalyticsService
	Service     *domain.Service
	Traces      *domain.TraceService
	Evaluations *domain.EvaluationService
	AdminToken  string
	Logger      *slog.Logger
}

// Register adds Assay's API and OpenAPI routes to a standard-library mux.
func Register(
	router *http.ServeMux,
	dependencies Dependencies,
) huma.API {
	config := huma.DefaultConfig("Assay API", "1.0.0")
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"adminBearer": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "Assay admin token",
		},
		"projectBearer": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "Assay project API key",
		},
		"projectAPIKey": {
			Type: "apiKey",
			In:   "header",
			Name: "x-api-key",
		},
	}
	humaAPI := humago.New(router, config)
	handlers := &handler{
		api:         humaAPI,
		analytics:   dependencies.Analytics,
		service:     dependencies.Service,
		traces:      dependencies.Traces,
		evaluations: dependencies.Evaluations,
		adminToken:  dependencies.AdminToken,
		logger:      dependencies.Logger,
	}
	handlers.registerProjectRoutes()
	handlers.registerAPIKeyRoutes()
	handlers.registerApplicationRoutes()
	handlers.registerTraceRoutes()
	handlers.registerDatasetRoutes()
	handlers.registerScorerConfigRoutes()
	handlers.registerEvalRunRoutes()
	handlers.registerAnalyticsRoutes()
	return humaAPI
}

func (h *handler) projectOperation(
	method string,
	path string,
	operationID string,
	summary string,
	errors ...int,
) huma.Operation {
	return huma.Operation{
		Method:      method,
		Path:        path,
		OperationID: operationID,
		Summary:     summary,
		Errors:      append([]int{http.StatusUnauthorized}, errors...),
		Security: []map[string][]string{
			{"projectBearer": {}},
			{"projectAPIKey": {}},
		},
	}
}

func traceReadOperation(operation huma.Operation) huma.Operation {
	operation.Security = append(operation.Security, map[string][]string{"adminBearer": {}})
	return operation
}

func (h *handler) operation(
	method string,
	path string,
	operationID string,
	summary string,
	errors ...int,
) huma.Operation {
	return huma.Operation{
		Method:      method,
		Path:        path,
		OperationID: operationID,
		Summary:     summary,
		Errors:      append([]int{http.StatusUnauthorized}, errors...),
		Security: []map[string][]string{
			{"adminBearer": {}},
		},
		Middlewares: huma.Middlewares{h.requireAdmin},
	}
}
