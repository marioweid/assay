package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type createAPIKeyInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Name string `json:"name" minLength:"1"`
	}
}

type apiKeyIDInput struct {
	ID    string `path:"id" format:"uuid"`
	KeyID string `path:"keyId" format:"uuid"`
}

type createdAPIKeyResponse struct {
	ID         string     `json:"id" format:"uuid"`
	ProjectID  string     `json:"project_id" format:"uuid"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Key        string     `json:"key"`
}

type createdAPIKeyResult struct {
	Body createdAPIKeyResponse
}

type apiKeyCollectionResult struct {
	Body struct {
		Items []apiKeyResponse `json:"items"`
	}
}

func (h *handler) registerAPIKeyRoutes() {
	createOperation := h.operation(
		http.MethodPost,
		"/v1/projects/{id}/keys",
		"create-api-key",
		"Create a project API key",
		http.StatusNotFound,
		http.StatusConflict,
	)
	createOperation.DefaultStatus = http.StatusCreated
	huma.Register(h.api, createOperation, h.createAPIKey)
	huma.Register(h.api, h.operation(
		http.MethodGet,
		"/v1/projects/{id}/keys",
		"list-api-keys",
		"List project API keys",
		http.StatusNotFound,
	), h.listAPIKeys)
	deleteOperation := h.operation(
		http.MethodDelete,
		"/v1/projects/{id}/keys/{keyId}",
		"revoke-api-key",
		"Revoke a project API key",
		http.StatusNotFound,
	)
	deleteOperation.DefaultStatus = http.StatusNoContent
	huma.Register(h.api, deleteOperation, h.revokeAPIKey)
}

func (h *handler) createAPIKey(
	ctx context.Context,
	input *createAPIKeyInput,
) (*createdAPIKeyResult, error) {
	projectID, err := parseID(input.ID, "project ID")
	if err != nil {
		return nil, h.responseError("create API key", err)
	}
	created, err := h.service.CreateAPIKey(ctx, projectID, input.Body.Name)
	if err != nil {
		return nil, h.responseError("create API key", err)
	}
	metadata := apiKeyOutput(created.APIKey)
	return &createdAPIKeyResult{Body: createdAPIKeyResponse{
		ID:         metadata.ID,
		ProjectID:  metadata.ProjectID,
		Name:       metadata.Name,
		KeyPrefix:  metadata.KeyPrefix,
		LastUsedAt: metadata.LastUsedAt,
		RevokedAt:  metadata.RevokedAt,
		CreatedAt:  metadata.CreatedAt,
		UpdatedAt:  metadata.UpdatedAt,
		Key:        created.Key,
	}}, nil
}

func (h *handler) listAPIKeys(
	ctx context.Context,
	input *projectIDInput,
) (*apiKeyCollectionResult, error) {
	projectID, err := parseID(input.ID, "project ID")
	if err != nil {
		return nil, h.responseError("list API keys", err)
	}
	keys, err := h.service.ListAPIKeys(ctx, projectID)
	if err != nil {
		return nil, h.responseError("list API keys", err)
	}
	result := &apiKeyCollectionResult{}
	result.Body.Items = make([]apiKeyResponse, 0, len(keys))
	for _, key := range keys {
		result.Body.Items = append(result.Body.Items, apiKeyOutput(key))
	}
	return result, nil
}

func (h *handler) revokeAPIKey(
	ctx context.Context,
	input *apiKeyIDInput,
) (*emptyOutput, error) {
	projectID, err := parseID(input.ID, "project ID")
	if err != nil {
		return nil, h.responseError("revoke API key", err)
	}
	keyID, err := parseID(input.KeyID, "API key ID")
	if err != nil {
		return nil, h.responseError("revoke API key", err)
	}
	if err := h.service.RevokeAPIKey(ctx, projectID, keyID); err != nil {
		return nil, h.responseError("revoke API key", err)
	}
	return &emptyOutput{}, nil
}
