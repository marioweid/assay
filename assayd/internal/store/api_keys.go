package store

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
)

// CreateAPIKey persists API-key metadata and its digest.
func (d *Database) CreateAPIKey(ctx context.Context, key domain.APIKey) (domain.APIKey, error) {
	row, err := d.queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:        key.ID,
		ProjectID: key.ProjectID,
		Name:      key.Name,
		KeyHash:   key.KeyHash[:],
		KeyPrefix: key.KeyPrefix,
	})
	if err != nil {
		return domain.APIKey{}, mapStoreError("insert API key", err)
	}
	key, err = apiKeyFromRow(row)
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("read inserted API key: %w", err)
	}
	return key, nil
}

// ListAPIKeys returns key metadata for one project.
func (d *Database) ListAPIKeys(ctx context.Context, projectID uuid.UUID) ([]domain.APIKey, error) {
	rows, err := d.queries.ListAPIKeys(ctx, projectID)
	if err != nil {
		return nil, mapStoreError("select API keys", err)
	}
	keys := make([]domain.APIKey, 0, len(rows))
	for _, row := range rows {
		key, convertErr := apiKeyFromRow(row)
		if convertErr != nil {
			return nil, fmt.Errorf("read listed API key %s: %w", row.ID, convertErr)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// UseActiveAPIKeyByHash authenticates an active digest and records its use.
func (d *Database) UseActiveAPIKeyByHash(
	ctx context.Context,
	hash [sha256.Size]byte,
) (domain.APIKey, error) {
	row, err := d.queries.UseActiveAPIKeyByHash(ctx, hash[:])
	if err != nil {
		return domain.APIKey{}, mapStoreError("select active API key", err)
	}
	key, err := apiKeyFromRow(row)
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("read active API key: %w", err)
	}
	return key, nil
}

// RevokeAPIKey marks an active project key as revoked.
func (d *Database) RevokeAPIKey(ctx context.Context, projectID uuid.UUID, keyID uuid.UUID) error {
	_, err := d.queries.RevokeAPIKey(ctx, db.RevokeAPIKeyParams{ID: keyID, ProjectID: projectID})
	if err != nil {
		return mapStoreError("revoke API key", err)
	}
	return nil
}
