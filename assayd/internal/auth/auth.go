// Package auth provides Assay's token generation and verification primitives.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"strings"
)

const (
	// APIKeyPrefix identifies Assay project API keys.
	APIKeyPrefix = "asy_"
	apiKeyChars  = 32
	base62       = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	maxBase62    = byte(248)
)

// GenerateAPIKey returns a cryptographically random Assay project API key.
func GenerateAPIKey() (string, error) {
	characters := make([]byte, apiKeyChars)
	random := make([]byte, apiKeyChars)
	for position := 0; position < len(characters); {
		if _, err := rand.Read(random); err != nil {
			return "", fmt.Errorf("generate API key randomness: %w", err)
		}
		for _, value := range random {
			if value >= maxBase62 {
				continue
			}
			characters[position] = base62[int(value)%len(base62)]
			position++
			if position == len(characters) {
				break
			}
		}
	}
	return APIKeyPrefix + string(characters), nil
}

// HashAPIKey returns the SHA-256 digest stored for a plaintext API key.
func HashAPIKey(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

// ValidAdminAuthorization checks an exact bearer credential in constant time.
func ValidAdminAuthorization(header string, expectedToken string) bool {
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found || expectedToken == "" || len(token) != len(expectedToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) == 1
}

// ValidAPIKeyFormat checks the public shape of an Assay project API key.
func ValidAPIKeyFormat(token string) bool {
	if len(token) != len(APIKeyPrefix)+apiKeyChars || !strings.HasPrefix(token, APIKeyPrefix) {
		return false
	}
	for _, character := range token[len(APIKeyPrefix):] {
		if !strings.ContainsRune(base62, character) {
			return false
		}
	}
	return true
}
