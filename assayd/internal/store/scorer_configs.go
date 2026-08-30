package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListScorerConfigs returns persisted scorer overrides for an application.
func (d *Database) ListScorerConfigs(
	ctx context.Context,
	applicationID uuid.UUID,
) ([]domain.ScorerConfig, error) {
	rows, err := d.queries.ListScorerConfigs(ctx, applicationID)
	if err != nil {
		return nil, mapStoreError("select scorer configs", err)
	}
	configs := make([]domain.ScorerConfig, 0, len(rows))
	for _, row := range rows {
		config, convertErr := scorerConfigFromRow(row)
		if convertErr != nil {
			return nil, convertErr
		}
		configs = append(configs, config)
	}
	return configs, nil
}

// UpsertScorerConfig persists an application scorer override.
func (d *Database) UpsertScorerConfig(
	ctx context.Context,
	config domain.ScorerConfig,
) (domain.ScorerConfig, error) {
	threshold, err := numeric(config.Threshold)
	if err != nil {
		return domain.ScorerConfig{}, fmt.Errorf("encode scorer threshold: %w", err)
	}
	var judgeConfig []byte
	if config.JudgeConfig != nil {
		judgeConfig, err = encodeJSON("scorer judge config", config.JudgeConfig)
		if err != nil {
			return domain.ScorerConfig{}, err
		}
	}
	row, err := d.queries.UpsertScorerConfig(ctx, db.UpsertScorerConfigParams{
		ID: config.ID, ApplicationID: config.ApplicationID, Scorer: config.Scorer,
		Enabled: config.Enabled, Threshold: threshold, JudgeConfig: judgeConfig,
		PromptTemplateID: nullableText(&config.PromptTemplateID),
	})
	if err != nil {
		return domain.ScorerConfig{}, mapStoreError("upsert scorer config", err)
	}
	return scorerConfigFromRow(row)
}

func scorerConfigFromRow(row db.ScorerConfig) (domain.ScorerConfig, error) {
	threshold, err := numericFloat(row.Threshold)
	if err != nil {
		return domain.ScorerConfig{}, fmt.Errorf("decode scorer threshold: %w", err)
	}
	config := domain.ScorerConfig{
		ID: row.ID, ApplicationID: row.ApplicationID, Scorer: row.Scorer,
		Enabled: row.Enabled, Threshold: threshold, PromptTemplateID: row.PromptTemplateID.String,
		Persisted: true, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if len(row.JudgeConfig) > 0 {
		config.JudgeConfig = &domain.JudgeConfig{}
		if err := decodeStoredJSON(row.JudgeConfig, config.JudgeConfig); err != nil {
			return domain.ScorerConfig{}, fmt.Errorf("decode scorer judge config: %w", err)
		}
	}
	return config, nil
}

func numeric(value float64) (pgtype.Numeric, error) {
	var result pgtype.Numeric
	if err := result.Scan(strconv.FormatFloat(value, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}, err
	}
	return result, nil
}

func numericFloat(value pgtype.Numeric) (float64, error) {
	databaseValue, err := value.Value()
	if err != nil {
		return 0, err
	}
	text, ok := databaseValue.(string)
	if !ok {
		return 0, fmt.Errorf("database numeric value has type %T", databaseValue)
	}
	return strconv.ParseFloat(text, 64)
}
