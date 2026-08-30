package api

import (
	"context"
	"net/http"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type scorerConfigsInput struct {
	ApplicationID string `path:"application_id" format:"uuid"`
}

type putScorerConfigInput struct {
	ApplicationID string `path:"application_id" format:"uuid"`
	Scorer        string `path:"scorer"`
	Body          struct {
		Enabled          *bool             `json:"enabled,omitempty"`
		Threshold        *float64          `json:"threshold,omitempty" minimum:"0" maximum:"1"`
		JudgeConfig      *judgeConfigInput `json:"judge_config,omitempty"`
		PromptTemplateID *string           `json:"prompt_template_id,omitempty"`
	}
}

type scorerConfigResult struct{ Body scorerConfigResponse }
type scorerConfigCollectionResult struct {
	Body struct {
		Items []scorerConfigResponse `json:"items"`
	}
}

type scorerConfigResponse struct {
	ID               *string              `json:"id,omitempty" format:"uuid"`
	ApplicationID    string               `json:"application_id" format:"uuid"`
	Scorer           string               `json:"scorer"`
	Enabled          bool                 `json:"enabled"`
	Threshold        float64              `json:"threshold"`
	JudgeConfig      *judgeConfigResponse `json:"judge_config,omitempty"`
	PromptTemplateID string               `json:"prompt_template_id"`
	Persisted        bool                 `json:"persisted"`
}

func (h *handler) registerScorerConfigRoutes() {
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/applications/{application_id}/scorers",
		"list-scorer-configs", "List scorer configurations", http.StatusNotFound,
	), h.listScorerConfigs)
	huma.Register(h.api, h.operation(
		http.MethodPut, "/v1/applications/{application_id}/scorers/{scorer}",
		"put-scorer-config", "Set a scorer configuration", http.StatusNotFound,
	), h.putScorerConfig)
}

func (h *handler) listScorerConfigs(
	ctx context.Context,
	input *scorerConfigsInput,
) (*scorerConfigCollectionResult, error) {
	applicationID, err := parseID(input.ApplicationID, "application ID")
	if err != nil {
		return nil, h.responseError("list scorer configs", err)
	}
	configs, err := h.evaluations.ListScorerConfigs(ctx, applicationID)
	if err != nil {
		return nil, h.responseError("list scorer configs", err)
	}
	result := &scorerConfigCollectionResult{}
	result.Body.Items = make([]scorerConfigResponse, 0, len(configs))
	for _, config := range configs {
		result.Body.Items = append(result.Body.Items, scorerConfigOutput(config))
	}
	return result, nil
}

func (h *handler) putScorerConfig(
	ctx context.Context,
	input *putScorerConfigInput,
) (*scorerConfigResult, error) {
	applicationID, err := parseID(input.ApplicationID, "application ID")
	if err != nil {
		return nil, h.responseError("put scorer config", err)
	}
	config, err := h.evaluations.PutScorerConfig(
		ctx, applicationID, input.Scorer,
		domain.PutScorerConfigInput{
			Enabled: input.Body.Enabled, Threshold: input.Body.Threshold,
			JudgeConfig:      input.Body.JudgeConfig.domainInput(),
			PromptTemplateID: input.Body.PromptTemplateID,
		},
	)
	if err != nil {
		return nil, h.responseError("put scorer config", err)
	}
	return &scorerConfigResult{Body: scorerConfigOutput(config)}, nil
}

func scorerConfigOutput(config domain.ScorerConfig) scorerConfigResponse {
	response := scorerConfigResponse{
		ApplicationID: config.ApplicationID.String(), Scorer: config.Scorer,
		Enabled: config.Enabled, Threshold: config.Threshold,
		PromptTemplateID: config.PromptTemplateID, Persisted: config.Persisted,
	}
	if config.Persisted {
		id := config.ID.String()
		response.ID = &id
	}
	if config.JudgeConfig != nil {
		response.JudgeConfig = &judgeConfigResponse{
			BaseURL: config.JudgeConfig.BaseURL, Model: config.JudgeConfig.Model,
			HasAPIKey: len(config.JudgeConfig.APIKeyCiphertext) > 0,
		}
	}
	return response
}
