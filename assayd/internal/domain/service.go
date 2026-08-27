package domain

import (
	"context"
	"crypto/sha256"

	secretcrypto "github.com/marioweid/assay/assayd/internal/crypto"

	"github.com/google/uuid"
)

// Repository persists M1 domain entities.
type Repository interface {
	CreateProject(context.Context, Project) (Project, error)
	ListProjects(context.Context) ([]Project, error)
	GetProject(context.Context, uuid.UUID) (Project, error)
	UpdateProject(context.Context, Project) (Project, error)
	DeleteProject(context.Context, uuid.UUID) error
	CreateAPIKey(context.Context, APIKey) (APIKey, error)
	ListAPIKeys(context.Context, uuid.UUID) ([]APIKey, error)
	GetActiveAPIKeyByHash(context.Context, [sha256.Size]byte) (APIKey, error)
	RevokeAPIKey(context.Context, uuid.UUID, uuid.UUID) error
	CreateApplication(context.Context, Application) (Application, error)
	ListApplications(context.Context, *uuid.UUID) ([]Application, error)
	GetApplication(context.Context, uuid.UUID) (Application, error)
	UpdateApplication(context.Context, Application) (Application, error)
	DeleteApplication(context.Context, uuid.UUID) error
}

// Service owns project, API-key, and application workflows.
type Service struct {
	repository Repository
	cipher     *secretcrypto.Cipher
}

// NewService constructs the M1 domain service.
func NewService(repository Repository, cipher *secretcrypto.Cipher) *Service {
	return &Service{repository: repository, cipher: cipher}
}
