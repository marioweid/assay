package store

import (
	"context"
	"fmt"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
)

// CreateProject persists a project.
func (d *Database) CreateProject(
	ctx context.Context,
	project domain.Project,
) (domain.Project, error) {
	var judgeConfig []byte
	var err error
	if project.JudgeConfig != nil {
		judgeConfig, err = encodeJSON("project judge config", project.JudgeConfig)
		if err != nil {
			return domain.Project{}, err
		}
	}
	row, err := d.queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          project.ID,
		Name:        project.Name,
		JudgeConfig: judgeConfig,
	})
	if err != nil {
		return domain.Project{}, mapStoreError("insert project", err)
	}
	project, err = projectFromRow(row)
	if err != nil {
		return domain.Project{}, fmt.Errorf("read inserted project: %w", err)
	}
	return project, nil
}

// ListProjects returns all projects in deterministic storage order.
func (d *Database) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := d.queries.ListProjects(ctx)
	if err != nil {
		return nil, mapStoreError("select projects", err)
	}
	projects := make([]domain.Project, 0, len(rows))
	for _, row := range rows {
		project, convertErr := projectFromRow(row)
		if convertErr != nil {
			return nil, fmt.Errorf("read listed project %s: %w", row.ID, convertErr)
		}
		projects = append(projects, project)
	}
	return projects, nil
}

// GetProject returns one project by ID.
func (d *Database) GetProject(ctx context.Context, projectID uuid.UUID) (domain.Project, error) {
	row, err := d.queries.GetProject(ctx, projectID)
	if err != nil {
		return domain.Project{}, mapStoreError("select project", err)
	}
	project, err := projectFromRow(row)
	if err != nil {
		return domain.Project{}, fmt.Errorf("read project %s: %w", projectID, err)
	}
	return project, nil
}

// UpdateProject persists a complete merged project.
func (d *Database) UpdateProject(
	ctx context.Context,
	project domain.Project,
) (domain.Project, error) {
	var judgeConfig []byte
	var err error
	if project.JudgeConfig != nil {
		judgeConfig, err = encodeJSON("project judge config", project.JudgeConfig)
		if err != nil {
			return domain.Project{}, err
		}
	}
	row, err := d.queries.UpdateProject(ctx, db.UpdateProjectParams{
		ID:          project.ID,
		Name:        project.Name,
		JudgeConfig: judgeConfig,
	})
	if err != nil {
		return domain.Project{}, mapStoreError("update project", err)
	}
	project, err = projectFromRow(row)
	if err != nil {
		return domain.Project{}, fmt.Errorf("read updated project %s: %w", row.ID, err)
	}
	return project, nil
}

// DeleteProject removes a project by ID.
func (d *Database) DeleteProject(ctx context.Context, projectID uuid.UUID) error {
	return d.deleteWithJobLock(ctx, "delete project", func(queries *db.Queries) error {
		_, err := queries.DeleteProject(ctx, projectID)
		return err
	})
}
