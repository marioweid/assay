package domain

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/marioweid/assay/assayd/internal/id"

	"github.com/google/uuid"
)

// CreateProject validates and creates a project.
func (s *Service) CreateProject(ctx context.Context, input CreateProjectInput) (Project, error) {
	name, err := requiredValue("project name", input.Name)
	if err != nil {
		return Project{}, err
	}
	judgeConfig, err := s.buildJudgeConfig(input.JudgeConfig, nil)
	if err != nil {
		return Project{}, err
	}
	projectID, err := id.New()
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	project, err := s.repository.CreateProject(ctx, Project{
		ID:          projectID,
		Name:        name,
		JudgeConfig: judgeConfig,
	})
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return project, nil
}

// ListProjects returns projects in storage order.
func (s *Service) ListProjects(ctx context.Context) ([]Project, error) {
	projects, err := s.repository.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projects, nil
}

// GetProject returns one project by ID.
func (s *Service) GetProject(ctx context.Context, projectID uuid.UUID) (Project, error) {
	project, err := s.repository.GetProject(ctx, projectID)
	if err != nil {
		return Project{}, fmt.Errorf("get project %s: %w", projectID, err)
	}
	return project, nil
}

// UpdateProject merges and persists supplied project fields.
func (s *Service) UpdateProject(
	ctx context.Context,
	projectID uuid.UUID,
	input UpdateProjectInput,
) (Project, error) {
	if err := validateProjectUpdate(input); err != nil {
		return Project{}, err
	}
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return Project{}, err
	}
	project, err = s.mergeProjectUpdate(project, input)
	if err != nil {
		return Project{}, err
	}
	project, err = s.repository.UpdateProject(ctx, project)
	if err != nil {
		return Project{}, fmt.Errorf("update project %s: %w", projectID, err)
	}
	return project, nil
}

// DeleteProject removes a project and its owned M1 entities.
func (s *Service) DeleteProject(ctx context.Context, projectID uuid.UUID) error {
	if err := s.repository.DeleteProject(ctx, projectID); err != nil {
		return fmt.Errorf("delete project %s: %w", projectID, err)
	}
	return nil
}

func (s *Service) buildJudgeConfig(
	input *JudgeConfigInput,
	current *JudgeConfig,
) (*JudgeConfig, error) {
	if input == nil {
		return nil, nil
	}
	config := &JudgeConfig{
		BaseURL: strings.TrimSpace(input.BaseURL),
		Model:   strings.TrimSpace(input.Model),
	}
	var currentCiphertext []byte
	if current != nil {
		currentCiphertext = current.APIKeyCiphertext
	}
	if err := validateHTTPURL("judge base URL", config.BaseURL, true); err != nil {
		return nil, err
	}
	ciphertext, err := s.mergeSecret(input.APIKey, currentCiphertext)
	if err != nil {
		return nil, fmt.Errorf("encrypt judge API key: %w", err)
	}
	config.APIKeyCiphertext = ciphertext
	if config.BaseURL == "" && config.Model == "" && len(config.APIKeyCiphertext) == 0 {
		return nil, fmt.Errorf("judge config: %w: at least one value is required", ErrInvalid)
	}
	return config, nil
}

func validateProjectUpdate(input UpdateProjectInput) error {
	if input.JudgeConfig != nil && input.ClearJudgeConfig {
		return fmt.Errorf("update project: %w: cannot set and clear judge config", ErrInvalid)
	}
	if input.Name == nil && input.JudgeConfig == nil && !input.ClearJudgeConfig {
		return fmt.Errorf("update project: %w: no fields supplied", ErrInvalid)
	}
	return nil
}

func (s *Service) mergeProjectUpdate(
	project Project,
	input UpdateProjectInput,
) (Project, error) {
	var err error
	if input.Name != nil {
		project.Name, err = requiredValue("project name", *input.Name)
		if err != nil {
			return Project{}, err
		}
	}
	if input.ClearJudgeConfig {
		project.JudgeConfig = nil
		return project, nil
	}
	if input.JudgeConfig != nil {
		project.JudgeConfig, err = s.buildJudgeConfig(input.JudgeConfig, project.JudgeConfig)
	}
	return project, err
}

func (s *Service) mergeSecret(value *string, current []byte) ([]byte, error) {
	if value == nil {
		return current, nil
	}
	if *value == "" {
		return nil, nil
	}
	ciphertext, err := s.cipher.Encrypt([]byte(*value))
	if err != nil {
		return nil, err
	}
	return ciphertext, nil
}

func requiredValue(name string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s: %w: must not be blank", name, ErrInvalid)
	}
	return value, nil
}

func validateHTTPURL(name string, value string, allowEmpty bool) error {
	if value == "" && allowEmpty {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s: %w: must be an absolute HTTP URL", name, ErrInvalid)
	}
	return nil
}
