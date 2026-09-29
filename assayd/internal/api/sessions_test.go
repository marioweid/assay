package api

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestSessionCursorRoundTripAndRejectsMalformedInput(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	cursor := &domain.SessionCursor{Time: anchor.Add(-time.Hour), ID: "opaque-session"}
	encoded, err := encodeSessionCursor(cursor, anchor)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decoded, err := decodeSessionCursor(encoded)
	if err != nil || decoded.ID != cursor.ID || !decoded.Time.Equal(cursor.Time) ||
		!decoded.Anchor.Equal(anchor) {
		t.Fatalf("decoded cursor = %#v, error = %v", decoded, err)
	}
	longID := strings.Repeat("&", 128)
	longCursor, err := encodeSessionCursor(&domain.SessionCursor{Time: anchor, ID: longID}, anchor)
	if err != nil {
		t.Fatalf("encode long cursor: %v", err)
	}
	if got, err := decodeSessionCursor(longCursor); err != nil || got.ID != longID {
		t.Fatalf("long cursor = %#v, error = %v", got, err)
	}
	for _, malformed := range []string{"!", strings.Repeat("x", 4097),
		base64.RawURLEncoding.EncodeToString([]byte(`{"time":"2026-09-01T11:00:00Z"}`)),
	} {
		if _, err := decodeSessionCursor(malformed); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("cursor %q: %v, want invalid", malformed, err)
		}
	}
}
