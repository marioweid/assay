package api

import (
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestPageCursorRoundTrip(t *testing.T) {
	want := &domain.PageCursor{
		CreatedAt: time.Date(2026, time.August, 29, 12, 0, 0, 123, time.UTC),
		ID:        uuid.Must(uuid.NewV7()),
	}
	encoded, err := encodePageCursor(want)
	if err != nil {
		t.Fatalf("encode page cursor: %v", err)
	}
	got, err := decodePageCursor(encoded)
	if err != nil {
		t.Fatalf("decode page cursor: %v", err)
	}
	if got.ID != want.ID || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("page cursor = %#v, want %#v", got, want)
	}
}

func TestScoreCursorRoundTripAndRejectsUUIDCursor(t *testing.T) {
	want := &domain.ScoreCursor{
		CreatedAt: time.Date(2026, time.August, 29, 12, 0, 0, 123, time.UTC),
		ID:        9_007_199_254_740_993,
	}
	encoded, err := encodeScoreCursor(want)
	if err != nil {
		t.Fatalf("encode score cursor: %v", err)
	}
	got, err := decodeScoreCursor(encoded)
	if err != nil {
		t.Fatalf("decode score cursor: %v", err)
	}
	if got.ID != want.ID || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("score cursor = %#v, want %#v", got, want)
	}
	pageCursor, err := encodePageCursor(&domain.PageCursor{
		CreatedAt: want.CreatedAt, ID: uuid.Must(uuid.NewV7()),
	})
	if err != nil {
		t.Fatalf("encode UUID cursor: %v", err)
	}
	if _, err := decodeScoreCursor(pageCursor); err == nil {
		t.Fatal("decode UUID cursor as score cursor succeeded")
	}
}

func TestCursorsRejectMalformedValues(t *testing.T) {
	for _, encoded := range []string{"not-base64", "e30"} {
		if _, err := decodePageCursor(encoded); err == nil {
			t.Errorf("decode page cursor %q succeeded", encoded)
		}
		if _, err := decodeScoreCursor(encoded); err == nil {
			t.Errorf("decode score cursor %q succeeded", encoded)
		}
	}
}
