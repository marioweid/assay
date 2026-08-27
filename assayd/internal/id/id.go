// Package id generates external entity identifiers.
package id

import (
	"fmt"

	"github.com/google/uuid"
)

// New returns a new UUID v7.
func New() (uuid.UUID, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate UUID v7: %w", err)
	}
	return value, nil
}
