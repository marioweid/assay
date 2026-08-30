package store

import (
	"testing"
	"time"

	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDatasetItemFromRowPreservesIntegersAndNulls(t *testing.T) {
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	item, err := datasetItemFromRow(db.DatasetItem{
		ID: uuid.Must(uuid.NewV7()), DatasetID: uuid.Must(uuid.NewV7()),
		Input:     []byte(`{"question":"value","large":9007199254740993}`),
		Metadata:  []byte(`{"nested":{"count":9223372036854775807}}`),
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("convert dataset item: %v", err)
	}
	if item.Input["large"] != int64(9_007_199_254_740_993) {
		t.Fatalf("large input = %#v", item.Input["large"])
	}
	nested, ok := item.Metadata["nested"].(map[string]any)
	if !ok || nested["count"] != int64(9_223_372_036_854_775_807) {
		t.Fatalf("nested metadata = %#v", item.Metadata["nested"])
	}
	if item.Context != nil || item.Output != nil || item.ExpectedOutput != nil {
		t.Fatalf("nullable fields = %#v/%#v/%#v", item.Context, item.Output, item.ExpectedOutput)
	}
}
