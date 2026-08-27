package store

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func projectFromRow(row db.Project) (domain.Project, error) {
	var judgeConfig *domain.JudgeConfig
	if len(row.JudgeConfig) > 0 {
		judgeConfig = &domain.JudgeConfig{}
		if err := json.Unmarshal(row.JudgeConfig, judgeConfig); err != nil {
			return domain.Project{}, fmt.Errorf("decode project judge config: %w", err)
		}
	}
	return domain.Project{
		ID:          row.ID,
		Name:        row.Name,
		JudgeConfig: judgeConfig,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}

func apiKeyFromRow(row db.ApiKey) (domain.APIKey, error) {
	if len(row.KeyHash) != sha256.Size {
		return domain.APIKey{}, fmt.Errorf(
			"decode API key hash: got %d bytes, want %d",
			len(row.KeyHash),
			sha256.Size,
		)
	}
	key := domain.APIKey{
		ID:         row.ID,
		ProjectID:  row.ProjectID,
		Name:       row.Name,
		KeyPrefix:  row.KeyPrefix,
		LastUsedAt: optionalTime(row.LastUsedAt),
		RevokedAt:  optionalTime(row.RevokedAt),
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
	copy(key.KeyHash[:], row.KeyHash)
	return key, nil
}

func applicationFromRow(row db.Application) (domain.Application, error) {
	config := make(map[string]any)
	if err := json.Unmarshal(row.Config, &config); err != nil {
		return domain.Application{}, fmt.Errorf("decode application config: %w", err)
	}
	var targetEndpoint *domain.TargetEndpoint
	if len(row.TargetEndpoint) > 0 {
		targetEndpoint = &domain.TargetEndpoint{}
		if err := json.Unmarshal(row.TargetEndpoint, targetEndpoint); err != nil {
			return domain.Application{}, fmt.Errorf("decode target endpoint: %w", err)
		}
	}
	return domain.Application{
		ID:               row.ID,
		ProjectID:        row.ProjectID,
		Name:             row.Name,
		Slug:             row.Slug,
		Config:           config,
		AutoScoreScorers: row.AutoScoreScorers,
		TargetEndpoint:   targetEndpoint,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}, nil
}

func encodeJSON(name string, value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", name, err)
	}
	return encoded, nil
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func mapStoreError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, domain.ErrConflict)
		case "23503":
			return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
