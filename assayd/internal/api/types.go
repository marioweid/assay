package api

import (
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
)

type judgeConfigInput struct {
	BaseURL string  `json:"base_url,omitempty" format:"uri"`
	Model   string  `json:"model,omitempty"`
	APIKey  *string `json:"api_key,omitempty" writeOnly:"true"`
}

func (input *judgeConfigInput) domainInput() *domain.JudgeConfigInput {
	if input == nil {
		return nil
	}
	return &domain.JudgeConfigInput{
		BaseURL: input.BaseURL,
		Model:   input.Model,
		APIKey:  input.APIKey,
	}
}

type judgeConfigResponse struct {
	BaseURL   string `json:"base_url,omitempty"`
	Model     string `json:"model,omitempty"`
	HasAPIKey bool   `json:"has_api_key"`
}

type projectResponse struct {
	ID          string               `json:"id" format:"uuid"`
	Name        string               `json:"name"`
	JudgeConfig *judgeConfigResponse `json:"judge_config,omitempty"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

func projectOutput(project domain.Project) projectResponse {
	response := projectResponse{
		ID:        project.ID.String(),
		Name:      project.Name,
		CreatedAt: project.CreatedAt,
		UpdatedAt: project.UpdatedAt,
	}
	if project.JudgeConfig != nil {
		response.JudgeConfig = &judgeConfigResponse{
			BaseURL:   project.JudgeConfig.BaseURL,
			Model:     project.JudgeConfig.Model,
			HasAPIKey: len(project.JudgeConfig.APIKeyCiphertext) > 0,
		}
	}
	return response
}

type apiKeyResponse struct {
	ID         string     `json:"id" format:"uuid"`
	ProjectID  string     `json:"project_id" format:"uuid"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func apiKeyOutput(key domain.APIKey) apiKeyResponse {
	return apiKeyResponse{
		ID:         key.ID.String(),
		ProjectID:  key.ProjectID.String(),
		Name:       key.Name,
		KeyPrefix:  key.KeyPrefix,
		LastUsedAt: key.LastUsedAt,
		RevokedAt:  key.RevokedAt,
		CreatedAt:  key.CreatedAt,
		UpdatedAt:  key.UpdatedAt,
	}
}

type responseMappingInput struct {
	Output  string `json:"output"`
	Context string `json:"context,omitempty"`
}

type targetEndpointInput struct {
	URL             string               `json:"url" format:"uri"`
	Method          string               `json:"method,omitempty"`
	Headers         map[string]string    `json:"headers,omitempty"`
	RequestTemplate map[string]any       `json:"request_template,omitempty"`
	ResponseMapping responseMappingInput `json:"response_mapping"`
	TimeoutMS       int                  `json:"timeout_ms,omitempty" minimum:"0"`
	Secret          *string              `json:"secret,omitempty" writeOnly:"true"`
}

func (input *targetEndpointInput) domainInput() *domain.TargetEndpointInput {
	if input == nil {
		return nil
	}
	return &domain.TargetEndpointInput{
		URL:             input.URL,
		Method:          input.Method,
		Headers:         input.Headers,
		RequestTemplate: input.RequestTemplate,
		ResponseMapping: domain.ResponseMapping{
			Output:  input.ResponseMapping.Output,
			Context: input.ResponseMapping.Context,
		},
		TimeoutMS: input.TimeoutMS,
		Secret:    input.Secret,
	}
}

type responseMappingResponse struct {
	Output  string `json:"output"`
	Context string `json:"context,omitempty"`
}

type targetEndpointResponse struct {
	URL             string                  `json:"url"`
	Method          string                  `json:"method"`
	Headers         map[string]string       `json:"headers,omitempty"`
	RequestTemplate map[string]any          `json:"request_template,omitempty"`
	ResponseMapping responseMappingResponse `json:"response_mapping"`
	TimeoutMS       int                     `json:"timeout_ms"`
	HasSecret       bool                    `json:"has_secret"`
}

type applicationResponse struct {
	ID               string                  `json:"id" format:"uuid"`
	ProjectID        string                  `json:"project_id" format:"uuid"`
	Name             string                  `json:"name"`
	Slug             string                  `json:"slug"`
	Config           map[string]any          `json:"config"`
	AutoScoreScorers []string                `json:"auto_score_scorers"`
	TargetEndpoint   *targetEndpointResponse `json:"target_endpoint,omitempty"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
}

func applicationOutput(application domain.Application) applicationResponse {
	response := applicationResponse{
		ID:               application.ID.String(),
		ProjectID:        application.ProjectID.String(),
		Name:             application.Name,
		Slug:             application.Slug,
		Config:           application.Config,
		AutoScoreScorers: application.AutoScoreScorers,
		CreatedAt:        application.CreatedAt,
		UpdatedAt:        application.UpdatedAt,
	}
	if application.TargetEndpoint != nil {
		endpoint := application.TargetEndpoint
		response.TargetEndpoint = &targetEndpointResponse{
			URL:             endpoint.URL,
			Method:          endpoint.Method,
			Headers:         endpoint.Headers,
			RequestTemplate: endpoint.RequestTemplate,
			ResponseMapping: responseMappingResponse{
				Output:  endpoint.ResponseMapping.Output,
				Context: endpoint.ResponseMapping.Context,
			},
			TimeoutMS: endpoint.TimeoutMS,
			HasSecret: len(endpoint.SecretCiphertext) > 0,
		}
	}
	return response
}

type emptyOutput struct{}
