package api

import (
	"net/http"

	"github.com/marioweid/assay/assayd/internal/auth"

	"github.com/danielgtaylor/huma/v2"
)

func (h *handler) requireAdmin(ctx huma.Context, next func(huma.Context)) {
	if !auth.ValidAdminAuthorization(ctx.Header("Authorization"), h.adminToken) {
		if err := huma.WriteErr(h.api, ctx, http.StatusUnauthorized, "Unauthorized"); err != nil {
			h.logger.Error("write unauthorized response", "error", err)
		}
		return
	}
	next(ctx)
}
