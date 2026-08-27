package auth

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestGenerateAPIKeyProducesUniqueBase62Tokens(t *testing.T) {
	t.Parallel()

	const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	seen := make(map[string]struct{}, 100)
	for range 100 {
		token, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("generate API key: %v", err)
		}
		if len(token) != 36 {
			t.Fatalf("token length = %d, want 36", len(token))
		}
		if !strings.HasPrefix(token, "asy_") {
			t.Fatalf("token %q is missing asy_ prefix", token)
		}
		for _, character := range token[4:] {
			if !strings.ContainsRune(base62, character) {
				t.Fatalf("token %q contains non-base62 character %q", token, character)
			}
		}
		if _, exists := seen[token]; exists {
			t.Fatalf("duplicate token %q", token)
		}
		seen[token] = struct{}{}
	}
}

func TestHashAPIKeyUsesSHA256(t *testing.T) {
	t.Parallel()

	const want = "61611db11c655f5e2672eced4f2fbf4552eba9a925f1ffd5c3af50600b1786e7"
	got := HashAPIKey("asy_test")
	if encoded := hex.EncodeToString(got[:]); encoded != want {
		t.Fatalf("hash = %q, want %q", encoded, want)
	}
}

func TestValidAdminAuthorizationRequiresExactBearerToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "exact", header: "Bearer admin-secret", want: true},
		{name: "missing"},
		{name: "wrong scheme", header: "Basic admin-secret"},
		{name: "wrong token", header: "Bearer other-secret"},
		{name: "lowercase scheme", header: "bearer admin-secret"},
		{name: "leading whitespace", header: " Bearer admin-secret"},
		{name: "trailing whitespace", header: "Bearer admin-secret "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidAdminAuthorization(test.header, "admin-secret"); got != test.want {
				t.Fatalf("ValidAdminAuthorization() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestValidAPIKeyFormat(t *testing.T) {
	t.Parallel()

	valid := "asy_0123456789ABCDEFGHIJKLMNOPQRSTUV"
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{name: "valid", token: valid, want: true},
		{name: "missing", token: ""},
		{name: "wrong prefix", token: "bad_" + valid[4:]},
		{name: "too short", token: valid[:len(valid)-1]},
		{name: "too long", token: valid + "a"},
		{name: "non-base62", token: valid[:len(valid)-1] + "-"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidAPIKeyFormat(test.token); got != test.want {
				t.Fatalf("ValidAPIKeyFormat() = %t, want %t", got, test.want)
			}
		})
	}
}
