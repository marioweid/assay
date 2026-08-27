package api

import (
	"errors"
	"fmt"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

func (h *handler) responseError(operation string, err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return huma.Error422UnprocessableEntity("Invalid request", err)
	case errors.Is(err, domain.ErrUnauthorized):
		return huma.Error401Unauthorized("Unauthorized")
	case errors.Is(err, domain.ErrNotFound):
		return huma.Error404NotFound("Resource not found")
	case errors.Is(err, domain.ErrConflict):
		return huma.Error409Conflict("Resource already exists")
	default:
		h.logger.Error("API operation failed", "operation", operation, "error", err)
		return huma.Error500InternalServerError("Internal server error")
	}
}

func parseID(value string, name string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s: %w: must be a UUID", name, domain.ErrInvalid)
	}
	return parsed, nil
}
