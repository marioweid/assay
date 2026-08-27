package id

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewReturnsUniqueUUIDv7(t *testing.T) {
	t.Parallel()

	const count = 1_000
	seen := make(map[uuid.UUID]struct{}, count)
	for range count {
		value, err := New()
		if err != nil {
			t.Fatalf("generate UUID v7: %v", err)
		}
		if value.Version() != 7 {
			t.Fatalf("UUID version = %d, want 7", value.Version())
		}
		if value.Variant() != uuid.RFC4122 {
			t.Fatalf("UUID variant = %v, want RFC 4122", value.Variant())
		}
		if _, exists := seen[value]; exists {
			t.Fatalf("duplicate UUID generated: %s", value)
		}
		seen[value] = struct{}{}
	}
}
