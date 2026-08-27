package domain

import "errors"

var (
	// ErrNotFound indicates that a requested domain entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates that a unique domain value already exists.
	ErrConflict = errors.New("conflict")
	// ErrInvalid indicates that domain input violates a business rule.
	ErrInvalid = errors.New("invalid input")
	// ErrUnauthorized indicates that a credential cannot authenticate a project.
	ErrUnauthorized = errors.New("unauthorized")
)
