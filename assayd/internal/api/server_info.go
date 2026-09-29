package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type serverInfoResult struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		LocalMode bool `json:"local_mode"`
	}
}

func (h *handler) registerServerInfoRoute() {
	huma.Register(h.api, huma.Operation{
		Method: http.MethodGet, Path: "/v1/server-info", OperationID: "get-server-info",
		Summary: "Get public server authentication mode",
	}, func(_ context.Context, _ *struct{}) (*serverInfoResult, error) {
		result := &serverInfoResult{CacheControl: "no-store"}
		result.Body.LocalMode = h.localMode
		return result, nil
	})
}
