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
	api        huma.API
	service    *domain.Service
	adminToken string
	logger     *slog.Logger
}

// Register adds Assay's M1 API and OpenAPI routes to a standard-library mux.
func Register(
	router *http.ServeMux,
	service *domain.Service,
	adminToken string,
	logger *slog.Logger,
) huma.API {
	config := huma.DefaultConfig("Assay API", "1.0.0")
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"adminBearer": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "Assay admin token",
		},
	}
	humaAPI := humago.New(router, config)
	handlers := &handler{
		api:        humaAPI,
		service:    service,
		adminToken: adminToken,
		logger:     logger,
	}
	handlers.registerProjectRoutes()
	handlers.registerAPIKeyRoutes()
	handlers.registerApplicationRoutes()
	return humaAPI
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
