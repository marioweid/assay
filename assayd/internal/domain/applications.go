package domain

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/marioweid/assay/assayd/internal/id"

	"github.com/google/uuid"
)

const defaultEndpointTimeoutMS = 30000

// CreateApplication validates and creates an application.
func (s *Service) CreateApplication(
	ctx context.Context,
	input CreateApplicationInput,
) (Application, error) {
	scorers, err := normalizedAutoScoreScorers(input.AutoScoreScorers)
	if err != nil {
		return Application{}, err
	}
	name, err := requiredValue("application name", input.Name)
	if err != nil {
		return Application{}, err
	}
	slug, err := requiredValue("application slug", input.Slug)
	if err != nil {
		return Application{}, err
	}
	if _, err := s.GetProject(ctx, input.ProjectID); err != nil {
		return Application{}, err
	}
	applicationID, err := id.New()
	if err != nil {
		return Application{}, fmt.Errorf("create application: %w", err)
	}
	config := input.Config
	if config == nil {
		config = map[string]any{}
	}
	application, err := s.repository.CreateApplication(ctx, Application{
		ID:               applicationID,
		ProjectID:        input.ProjectID,
		Name:             name,
		Slug:             slug,
		Config:           config,
		AutoScoreScorers: scorers,
	})
	if err != nil {
		return Application{}, fmt.Errorf("create application: %w", err)
	}
	return application, nil
}

func normalizedAutoScoreScorers(scorers []string) ([]string, error) {
	if err := validateAutoScoreScorers(scorers); err != nil {
		return nil, err
	}
	if scorers == nil {
		return []string{}, nil
	}
	return scorers, nil
}

// ListApplications returns all applications or those owned by one project.
func (s *Service) ListApplications(
	ctx context.Context,
	projectID *uuid.UUID,
) ([]Application, error) {
	if projectID != nil {
		if _, err := s.GetProject(ctx, *projectID); err != nil {
			return nil, err
		}
	}
	applications, err := s.repository.ListApplications(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	return applications, nil
}

// GetApplication returns one application by ID.
func (s *Service) GetApplication(
	ctx context.Context,
	applicationID uuid.UUID,
) (Application, error) {
	application, err := s.repository.GetApplication(ctx, applicationID)
	if err != nil {
		return Application{}, fmt.Errorf("get application %s: %w", applicationID, err)
	}
	return application, nil
}

// ResolveTargetEndpoint returns executable endpoint settings with the secret decrypted.
func (s *Service) ResolveTargetEndpoint(
	ctx context.Context,
	applicationID uuid.UUID,
) (ResolvedTargetEndpoint, error) {
	application, err := s.GetApplication(ctx, applicationID)
	if err != nil {
		return ResolvedTargetEndpoint{}, err
	}
	endpoint := application.TargetEndpoint
	if endpoint == nil {
		return ResolvedTargetEndpoint{}, fmt.Errorf(
			"resolve target endpoint: %w: application has no target endpoint", ErrInvalid,
		)
	}
	secret := ""
	if len(endpoint.SecretCiphertext) > 0 {
		plaintext, err := s.cipher.Decrypt(endpoint.SecretCiphertext)
		if err != nil {
			return ResolvedTargetEndpoint{}, fmt.Errorf("decrypt target endpoint secret: %w", err)
		}
		secret = string(plaintext)
	}
	return ResolvedTargetEndpoint{
		URL:             endpoint.URL,
		Method:          endpoint.Method,
		Headers:         maps.Clone(endpoint.Headers),
		RequestTemplate: cloneJSONMap(endpoint.RequestTemplate),
		ResponseMapping: endpoint.ResponseMapping,
		Timeout:         time.Duration(endpoint.TimeoutMS) * time.Millisecond,
		Secret:          secret,
	}, nil
}

func cloneJSONMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = cloneJSONValue(item)
	}
	return cloned
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneJSONValue(item)
		}
		return cloned
	default:
		return typed
	}
}

// UpdateApplication merges and persists supplied application fields.
func (s *Service) UpdateApplication(
	ctx context.Context,
	applicationID uuid.UUID,
	input UpdateApplicationInput,
) (Application, error) {
	if err := validateApplicationUpdate(input); err != nil {
		return Application{}, err
	}
	application, err := s.GetApplication(ctx, applicationID)
	if err != nil {
		return Application{}, err
	}
	application.Name, err = mergeRequiredValue("application name", application.Name, input.Name)
	if err != nil {
		return Application{}, err
	}
	application.Slug, err = mergeRequiredValue("application slug", application.Slug, input.Slug)
	if err != nil {
		return Application{}, err
	}
	application.Config = mergedConfig(application.Config, input.Config)
	application.AutoScoreScorers = mergedScorers(
		application.AutoScoreScorers,
		input.AutoScoreScorers,
	)
	application, err = s.repository.UpdateApplication(ctx, application)
	if err != nil {
		return Application{}, fmt.Errorf("update application %s: %w", applicationID, err)
	}
	return application, nil
}

func validateApplicationUpdate(input UpdateApplicationInput) error {
	if input.Name == nil && input.Slug == nil && input.Config == nil && input.AutoScoreScorers == nil {
		return fmt.Errorf("update application: %w: no fields supplied", ErrInvalid)
	}
	if input.AutoScoreScorers != nil {
		return validateAutoScoreScorers(*input.AutoScoreScorers)
	}
	return nil
}

func validateAutoScoreScorers(scorers []string) error {
	seen := make(map[string]struct{}, len(scorers))
	for _, scorer := range scorers {
		if scorer != ScorerGroundedness && scorer != ScorerCorrectness {
			return fmt.Errorf("automatic scorers: %w: unsupported scorer %q", ErrInvalid, scorer)
		}
		if _, duplicate := seen[scorer]; duplicate {
			return fmt.Errorf("automatic scorers: %w: duplicate scorer %q", ErrInvalid, scorer)
		}
		seen[scorer] = struct{}{}
	}
	return nil
}

func mergedConfig(current map[string]any, update *map[string]any) map[string]any {
	if update == nil {
		return current
	}
	if *update == nil {
		return map[string]any{}
	}
	return *update
}

func mergedScorers(current []string, update *[]string) []string {
	if update == nil {
		return current
	}
	if *update == nil {
		return []string{}
	}
	return *update
}

func mergeRequiredValue(name string, current string, update *string) (string, error) {
	if update == nil {
		return current, nil
	}
	return requiredValue(name, *update)
}

// DeleteApplication removes an application.
func (s *Service) DeleteApplication(ctx context.Context, applicationID uuid.UUID) error {
	if err := s.repository.DeleteApplication(ctx, applicationID); err != nil {
		return fmt.Errorf("delete application %s: %w", applicationID, err)
	}
	return nil
}

// SetApplicationEndpoint replaces or clears an application's target endpoint.
func (s *Service) SetApplicationEndpoint(
	ctx context.Context,
	applicationID uuid.UUID,
	patch EndpointPatch,
) (Application, error) {
	if (patch.Endpoint == nil) == !patch.Clear {
		return Application{}, fmt.Errorf(
			"update target endpoint: %w: select exactly one of endpoint or clear",
			ErrInvalid,
		)
	}
	application, err := s.GetApplication(ctx, applicationID)
	if err != nil {
		return Application{}, err
	}
	if patch.Clear {
		application.TargetEndpoint = nil
	} else {
		application.TargetEndpoint, err = s.buildTargetEndpoint(
			patch.Endpoint,
			application.TargetEndpoint,
		)
		if err != nil {
			return Application{}, err
		}
	}
	application, err = s.repository.UpdateApplication(ctx, application)
	if err != nil {
		return Application{}, fmt.Errorf("update target endpoint for %s: %w", applicationID, err)
	}
	return application, nil
}

func (s *Service) buildTargetEndpoint(
	input *TargetEndpointInput,
	current *TargetEndpoint,
) (*TargetEndpoint, error) {
	request, method, err := normalizedEndpointRequest(input.Method, input.URL)
	if err != nil {
		return nil, err
	}
	responseMapping, err := normalizedResponseMapping(input.ResponseMapping)
	if err != nil {
		return nil, err
	}
	timeout, err := normalizedTimeout(input.TimeoutMS)
	if err != nil {
		return nil, err
	}
	endpoint := &TargetEndpoint{
		URL:             request.URL.String(),
		Method:          method,
		Headers:         input.Headers,
		RequestTemplate: input.RequestTemplate,
		ResponseMapping: responseMapping,
		TimeoutMS:       timeout,
	}
	var currentCiphertext []byte
	if current != nil {
		currentCiphertext = current.SecretCiphertext
	}
	endpoint.SecretCiphertext, err = s.mergeSecret(input.Secret, currentCiphertext)
	if err != nil {
		return nil, fmt.Errorf("encrypt target endpoint secret: %w", err)
	}
	return endpoint, nil
}

func normalizedEndpointRequest(method string, rawURL string) (*http.Request, string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodPost
	}
	request, err := http.NewRequest(method, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return nil, "", fmt.Errorf(
			"target endpoint URL: %w",
			errors.Join(ErrInvalid, err),
		)
	}
	if err := validateHTTPURL("target endpoint URL", request.URL.String(), false); err != nil {
		return nil, "", err
	}
	return request, method, nil
}

func normalizedResponseMapping(mapping ResponseMapping) (ResponseMapping, error) {
	mapping.Output = strings.TrimSpace(mapping.Output)
	mapping.Context = strings.TrimSpace(mapping.Context)
	if mapping.Output == "" {
		return ResponseMapping{}, fmt.Errorf("target response output: %w: must not be blank", ErrInvalid)
	}
	return mapping, nil
}

func normalizedTimeout(timeout int) (int, error) {
	if timeout == 0 {
		return defaultEndpointTimeoutMS, nil
	}
	if timeout < 0 {
		return 0, fmt.Errorf("target endpoint timeout: %w: must be positive", ErrInvalid)
	}
	return timeout, nil
}
