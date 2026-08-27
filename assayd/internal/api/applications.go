package api

import (
	"context"
	"net/http"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type createApplicationInput struct {
	Body struct {
		ProjectID        string         `json:"project_id" format:"uuid"`
		Name             string         `json:"name" minLength:"1"`
		Slug             string         `json:"slug" minLength:"1"`
		Config           map[string]any `json:"config,omitempty"`
		AutoScoreScorers []string       `json:"auto_score_scorers,omitempty"`
	}
}

type updateApplicationInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Name             *string         `json:"name,omitempty"`
		Slug             *string         `json:"slug,omitempty"`
		Config           *map[string]any `json:"config,omitempty"`
		AutoScoreScorers *[]string       `json:"auto_score_scorers,omitempty"`
	}
}

type applicationIDInput struct {
	ID string `path:"id" format:"uuid"`
}

type listApplicationsInput struct {
	ProjectID string `query:"project_id" format:"uuid" required:"false"`
}

type endpointInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Endpoint *targetEndpointInput `json:"endpoint,omitempty"`
		Clear    bool                 `json:"clear,omitempty"`
	}
}

type applicationResult struct {
	Body applicationResponse
}

type applicationCollectionResult struct {
	Body struct {
		Items []applicationResponse `json:"items"`
	}
}

func (h *handler) registerApplicationRoutes() {
	createOperation := h.operation(
		http.MethodPost,
		"/v1/applications",
		"create-application",
		"Create an application",
		http.StatusNotFound,
		http.StatusConflict,
	)
	createOperation.DefaultStatus = http.StatusCreated
	huma.Register(h.api, createOperation, h.createApplication)
	huma.Register(h.api, h.operation(
		http.MethodGet,
		"/v1/applications",
		"list-applications",
		"List applications",
		http.StatusNotFound,
	), h.listApplications)
	huma.Register(h.api, h.operation(
		http.MethodGet,
		"/v1/applications/{id}",
		"get-application",
		"Get an application",
		http.StatusNotFound,
	), h.getApplication)
	huma.Register(h.api, h.operation(
		http.MethodPatch,
		"/v1/applications/{id}",
		"update-application",
		"Update an application",
		http.StatusNotFound,
		http.StatusConflict,
	), h.updateApplication)
	deleteOperation := h.operation(
		http.MethodDelete,
		"/v1/applications/{id}",
		"delete-application",
		"Delete an application",
		http.StatusNotFound,
	)
	deleteOperation.DefaultStatus = http.StatusNoContent
	huma.Register(h.api, deleteOperation, h.deleteApplication)
	huma.Register(h.api, h.operation(
		http.MethodPatch,
		"/v1/applications/{id}/endpoint",
		"update-application-endpoint",
		"Set or clear an application target endpoint",
		http.StatusNotFound,
	), h.setApplicationEndpoint)
}

func (h *handler) createApplication(
	ctx context.Context,
	input *createApplicationInput,
) (*applicationResult, error) {
	projectID, err := parseID(input.Body.ProjectID, "project ID")
	if err != nil {
		return nil, h.responseError("create application", err)
	}
	application, err := h.service.CreateApplication(ctx, domain.CreateApplicationInput{
		ProjectID:        projectID,
		Name:             input.Body.Name,
		Slug:             input.Body.Slug,
		Config:           input.Body.Config,
		AutoScoreScorers: input.Body.AutoScoreScorers,
	})
	if err != nil {
		return nil, h.responseError("create application", err)
	}
	return &applicationResult{Body: applicationOutput(application)}, nil
}

func (h *handler) listApplications(
	ctx context.Context,
	input *listApplicationsInput,
) (*applicationCollectionResult, error) {
	projectID, err := optionalProjectID(input.ProjectID)
	if err != nil {
		return nil, h.responseError("list applications", err)
	}
	applications, err := h.service.ListApplications(ctx, projectID)
	if err != nil {
		return nil, h.responseError("list applications", err)
	}
	result := &applicationCollectionResult{}
	result.Body.Items = make([]applicationResponse, 0, len(applications))
	for _, application := range applications {
		result.Body.Items = append(result.Body.Items, applicationOutput(application))
	}
	return result, nil
}

func (h *handler) getApplication(
	ctx context.Context,
	input *applicationIDInput,
) (*applicationResult, error) {
	applicationID, err := parseID(input.ID, "application ID")
	if err != nil {
		return nil, h.responseError("get application", err)
	}
	application, err := h.service.GetApplication(ctx, applicationID)
	if err != nil {
		return nil, h.responseError("get application", err)
	}
	return &applicationResult{Body: applicationOutput(application)}, nil
}

func (h *handler) updateApplication(
	ctx context.Context,
	input *updateApplicationInput,
) (*applicationResult, error) {
	applicationID, err := parseID(input.ID, "application ID")
	if err != nil {
		return nil, h.responseError("update application", err)
	}
	application, err := h.service.UpdateApplication(ctx, applicationID, domain.UpdateApplicationInput{
		Name:             input.Body.Name,
		Slug:             input.Body.Slug,
		Config:           input.Body.Config,
		AutoScoreScorers: input.Body.AutoScoreScorers,
	})
	if err != nil {
		return nil, h.responseError("update application", err)
	}
	return &applicationResult{Body: applicationOutput(application)}, nil
}

func (h *handler) deleteApplication(
	ctx context.Context,
	input *applicationIDInput,
) (*emptyOutput, error) {
	applicationID, err := parseID(input.ID, "application ID")
	if err != nil {
		return nil, h.responseError("delete application", err)
	}
	if err := h.service.DeleteApplication(ctx, applicationID); err != nil {
		return nil, h.responseError("delete application", err)
	}
	return &emptyOutput{}, nil
}

func (h *handler) setApplicationEndpoint(
	ctx context.Context,
	input *endpointInput,
) (*applicationResult, error) {
	applicationID, err := parseID(input.ID, "application ID")
	if err != nil {
		return nil, h.responseError("update application endpoint", err)
	}
	application, err := h.service.SetApplicationEndpoint(ctx, applicationID, domain.EndpointPatch{
		Endpoint: input.Body.Endpoint.domainInput(),
		Clear:    input.Body.Clear,
	})
	if err != nil {
		return nil, h.responseError("update application endpoint", err)
	}
	return &applicationResult{Body: applicationOutput(application)}, nil
}

func optionalProjectID(value string) (*uuid.UUID, error) {
	if value == "" {
		return nil, nil
	}
	projectID, err := parseID(value, "project ID")
	if err != nil {
		return nil, err
	}
	return &projectID, nil
}
