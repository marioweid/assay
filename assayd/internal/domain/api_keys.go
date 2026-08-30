package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/marioweid/assay/assayd/internal/auth"
	"github.com/marioweid/assay/assayd/internal/id"

	"github.com/google/uuid"
)

// CreateAPIKey creates a project key and returns its plaintext exactly once.
func (s *Service) CreateAPIKey(
	ctx context.Context,
	projectID uuid.UUID,
	name string,
) (CreatedAPIKey, error) {
	name, err := requiredValue("API key name", name)
	if err != nil {
		return CreatedAPIKey{}, err
	}
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return CreatedAPIKey{}, err
	}
	token, err := auth.GenerateAPIKey()
	if err != nil {
		return CreatedAPIKey{}, fmt.Errorf("create API key: %w", err)
	}
	keyID, err := id.New()
	if err != nil {
		return CreatedAPIKey{}, fmt.Errorf("create API key: %w", err)
	}
	key, err := s.repository.CreateAPIKey(ctx, APIKey{
		ID:        keyID,
		ProjectID: projectID,
		Name:      name,
		KeyHash:   auth.HashAPIKey(token),
		KeyPrefix: token[:8],
	})
	if err != nil {
		return CreatedAPIKey{}, fmt.Errorf("create API key: %w", err)
	}
	return CreatedAPIKey{APIKey: key, Key: token}, nil
}

// ListAPIKeys returns key metadata for an existing project.
func (s *Service) ListAPIKeys(ctx context.Context, projectID uuid.UUID) ([]APIKey, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	keys, err := s.repository.ListAPIKeys(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list API keys for project %s: %w", projectID, err)
	}
	return keys, nil
}

// RevokeAPIKey revokes an active key owned by a project.
func (s *Service) RevokeAPIKey(ctx context.Context, projectID uuid.UUID, keyID uuid.UUID) error {
	if err := s.repository.RevokeAPIKey(ctx, projectID, keyID); err != nil {
		return fmt.Errorf("revoke API key %s for project %s: %w", keyID, projectID, err)
	}
	return nil
}

// AuthenticateAPIKey resolves a valid active plaintext key to its project.
func (s *Service) AuthenticateAPIKey(ctx context.Context, token string) (uuid.UUID, error) {
	if !auth.ValidAPIKeyFormat(token) {
		return uuid.Nil, ErrUnauthorized
	}
	key, err := s.repository.UseActiveAPIKeyByHash(ctx, auth.HashAPIKey(token))
	if errors.Is(err, ErrNotFound) {
		return uuid.Nil, ErrUnauthorized
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("authenticate API key with prefix %q: %w", token[:8], err)
	}
	return key.ProjectID, nil
}
