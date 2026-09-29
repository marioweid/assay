package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/marioweid/assay/assayd/internal/auth"
	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

func (h *handler) requireAdmin(ctx huma.Context, next func(huma.Context)) {
	if !h.isAdmin(ctx.Header("Authorization"), ctx.Header("x-api-key")) {
		if err := huma.WriteErr(h.api, ctx, http.StatusUnauthorized, "Unauthorized"); err != nil {
			h.logger.Error("write unauthorized response", "error", err)
		}
		return
	}
	next(ctx)
}

func (h *handler) isAdmin(authorization string, apiKey string) bool {
	// Explicit credentials never gain the anonymous local administrator's scope.
	if h.localMode && authorization == "" && apiKey == "" {
		return true
	}
	return auth.ValidAdminAuthorization(authorization, h.adminToken)
}

func (h *handler) authenticateProject(
	ctx context.Context,
	authorization string,
	xAPIKey string,
) (uuid.UUID, error) {
	token, ok := auth.APIKeyFromHeaders(authorization, xAPIKey)
	if !ok {
		return uuid.Nil, domain.ErrUnauthorized
	}
	projectID, err := h.service.AuthenticateAPIKey(ctx, token)
	if err != nil {
		return uuid.Nil, fmt.Errorf("authenticate project request: %w", err)
	}
	return projectID, nil
}
