package api

import (
	"context"
	"net/http"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type createProjectInput struct {
	Body struct {
		Name        string            `json:"name" minLength:"1"`
		JudgeConfig *judgeConfigInput `json:"judge_config,omitempty"`
	}
}

type updateProjectInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Name             *string           `json:"name,omitempty"`
		JudgeConfig      *judgeConfigInput `json:"judge_config,omitempty"`
		ClearJudgeConfig bool              `json:"clear_judge_config,omitempty"`
	}
}

type projectIDInput struct {
	ID string `path:"id" format:"uuid"`
}

type projectResult struct {
	Body projectResponse
}

type projectCollectionResult struct {
	Body struct {
		Items []projectResponse `json:"items"`
	}
}

func (h *handler) registerProjectRoutes() {
	createOperation := h.operation(
		http.MethodPost,
		"/v1/projects",
		"create-project",
		"Create a project",
		http.StatusConflict,
	)
	createOperation.DefaultStatus = http.StatusCreated
	huma.Register(h.api, createOperation, h.createProject)
	huma.Register(h.api, h.operation(
		http.MethodGet,
		"/v1/projects",
		"list-projects",
		"List projects",
	), h.listProjects)
	huma.Register(h.api, h.operation(
		http.MethodGet,
		"/v1/projects/{id}",
		"get-project",
		"Get a project",
		http.StatusNotFound,
	), h.getProject)
	huma.Register(h.api, h.operation(
		http.MethodPatch,
		"/v1/projects/{id}",
		"update-project",
		"Update a project",
		http.StatusNotFound,
		http.StatusConflict,
	), h.updateProject)
	deleteOperation := h.operation(
		http.MethodDelete,
		"/v1/projects/{id}",
		"delete-project",
		"Delete a project",
		http.StatusNotFound,
	)
	deleteOperation.DefaultStatus = http.StatusNoContent
	huma.Register(h.api, deleteOperation, h.deleteProject)
}

func (h *handler) createProject(
	ctx context.Context,
	input *createProjectInput,
) (*projectResult, error) {
	project, err := h.service.CreateProject(ctx, domain.CreateProjectInput{
		Name:        input.Body.Name,
		JudgeConfig: input.Body.JudgeConfig.domainInput(),
	})
	if err != nil {
		return nil, h.responseError("create project", err)
	}
	return &projectResult{Body: projectOutput(project)}, nil
}

func (h *handler) listProjects(
	ctx context.Context,
	_ *struct{},
) (*projectCollectionResult, error) {
	projects, err := h.service.ListProjects(ctx)
	if err != nil {
		return nil, h.responseError("list projects", err)
	}
	result := &projectCollectionResult{}
	result.Body.Items = make([]projectResponse, 0, len(projects))
	for _, project := range projects {
		result.Body.Items = append(result.Body.Items, projectOutput(project))
	}
	return result, nil
}

func (h *handler) getProject(
	ctx context.Context,
	input *projectIDInput,
) (*projectResult, error) {
	projectID, err := parseID(input.ID, "project ID")
	if err != nil {
		return nil, h.responseError("get project", err)
	}
	project, err := h.service.GetProject(ctx, projectID)
	if err != nil {
		return nil, h.responseError("get project", err)
	}
	return &projectResult{Body: projectOutput(project)}, nil
}

func (h *handler) updateProject(
	ctx context.Context,
	input *updateProjectInput,
) (*projectResult, error) {
	projectID, err := parseID(input.ID, "project ID")
	if err != nil {
		return nil, h.responseError("update project", err)
	}
	project, err := h.service.UpdateProject(ctx, projectID, domain.UpdateProjectInput{
		Name:             input.Body.Name,
		JudgeConfig:      input.Body.JudgeConfig.domainInput(),
		ClearJudgeConfig: input.Body.ClearJudgeConfig,
	})
	if err != nil {
		return nil, h.responseError("update project", err)
	}
	return &projectResult{Body: projectOutput(project)}, nil
}

func (h *handler) deleteProject(
	ctx context.Context,
	input *projectIDInput,
) (*emptyOutput, error) {
	projectID, err := parseID(input.ID, "project ID")
	if err != nil {
		return nil, h.responseError("delete project", err)
	}
	if err := h.service.DeleteProject(ctx, projectID); err != nil {
		return nil, h.responseError("delete project", err)
	}
	return &emptyOutput{}, nil
}
