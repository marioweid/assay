package store

import (
	"context"
	"fmt"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
)

// CreateApplication persists an application.
func (d *Database) CreateApplication(
	ctx context.Context,
	application domain.Application,
) (domain.Application, error) {
	parameters, err := applicationParameters(application)
	if err != nil {
		return domain.Application{}, err
	}
	row, err := d.queries.CreateApplication(ctx, db.CreateApplicationParams{
		ID:               application.ID,
		ProjectID:        application.ProjectID,
		Name:             parameters.Name,
		Slug:             parameters.Slug,
		Config:           parameters.Config,
		AutoScoreScorers: parameters.AutoScoreScorers,
		TargetEndpoint:   parameters.TargetEndpoint,
	})
	if err != nil {
		return domain.Application{}, mapStoreError("insert application", err)
	}
	application, err = applicationFromRow(row)
	if err != nil {
		return domain.Application{}, fmt.Errorf("read inserted application: %w", err)
	}
	return application, nil
}

// ListApplications returns all applications or those owned by one project.
func (d *Database) ListApplications(
	ctx context.Context,
	projectID *uuid.UUID,
) ([]domain.Application, error) {
	var rows []db.Application
	var err error
	if projectID == nil {
		rows, err = d.queries.ListApplications(ctx)
	} else {
		rows, err = d.queries.ListApplicationsByProject(ctx, *projectID)
	}
	if err != nil {
		return nil, mapStoreError("select applications", err)
	}
	applications := make([]domain.Application, 0, len(rows))
	for _, row := range rows {
		application, convertErr := applicationFromRow(row)
		if convertErr != nil {
			return nil, fmt.Errorf("read listed application %s: %w", row.ID, convertErr)
		}
		applications = append(applications, application)
	}
	return applications, nil
}

// GetApplication returns one application by ID.
func (d *Database) GetApplication(
	ctx context.Context,
	applicationID uuid.UUID,
) (domain.Application, error) {
	row, err := d.queries.GetApplication(ctx, applicationID)
	if err != nil {
		return domain.Application{}, mapStoreError("select application", err)
	}
	application, err := applicationFromRow(row)
	if err != nil {
		return domain.Application{}, fmt.Errorf("read application %s: %w", applicationID, err)
	}
	return application, nil
}

// GetApplicationByProjectSlug returns one application through its ingest identity.
func (d *Database) GetApplicationByProjectSlug(
	ctx context.Context,
	projectID uuid.UUID,
	slug string,
) (domain.Application, error) {
	row, err := d.queries.GetApplicationByProjectSlug(
		ctx,
		db.GetApplicationByProjectSlugParams{ProjectID: projectID, Slug: slug},
	)
	if err != nil {
		return domain.Application{}, mapStoreError("select application by project and slug", err)
	}
	application, err := applicationFromRow(row)
	if err != nil {
		return domain.Application{}, fmt.Errorf("read application by project and slug: %w", err)
	}
	return application, nil
}

// UpdateApplication persists a complete merged application.
func (d *Database) UpdateApplication(
	ctx context.Context,
	application domain.Application,
) (domain.Application, error) {
	parameters, err := applicationParameters(application)
	if err != nil {
		return domain.Application{}, err
	}
	row, err := d.queries.UpdateApplication(ctx, db.UpdateApplicationParams{
		ID:               application.ID,
		Name:             parameters.Name,
		Slug:             parameters.Slug,
		Config:           parameters.Config,
		AutoScoreScorers: parameters.AutoScoreScorers,
		TargetEndpoint:   parameters.TargetEndpoint,
	})
	if err != nil {
		return domain.Application{}, mapStoreError("update application", err)
	}
	application, err = applicationFromRow(row)
	if err != nil {
		return domain.Application{}, fmt.Errorf("read updated application %s: %w", row.ID, err)
	}
	return application, nil
}

// DeleteApplication removes an application by ID.
func (d *Database) DeleteApplication(ctx context.Context, applicationID uuid.UUID) error {
	return d.deleteWithJobLock(ctx, "delete application", func(queries *db.Queries) error {
		_, err := queries.DeleteApplication(ctx, applicationID)
		return err
	})
}

type applicationParams struct {
	Name             string
	Slug             string
	Config           []byte
	AutoScoreScorers []string
	TargetEndpoint   []byte
}

func applicationParameters(application domain.Application) (applicationParams, error) {
	config, err := encodeJSON("application config", application.Config)
	if err != nil {
		return applicationParams{}, err
	}
	var targetEndpoint []byte
	if application.TargetEndpoint != nil {
		targetEndpoint, err = encodeJSON("target endpoint", application.TargetEndpoint)
		if err != nil {
			return applicationParams{}, err
		}
	}
	return applicationParams{
		Name:             application.Name,
		Slug:             application.Slug,
		Config:           config,
		AutoScoreScorers: application.AutoScoreScorers,
		TargetEndpoint:   targetEndpoint,
	}, nil
}
