package domain_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	secretcrypto "github.com/marioweid/assay/assayd/internal/crypto"
	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestEvaluationServiceValidatesDatasetItems(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	datasetID := uuid.Must(uuid.NewV7())
	repository := &evaluationRepositoryFake{
		application: domain.Application{ID: applicationID},
		dataset:     domain.Dataset{ID: datasetID, ApplicationID: applicationID},
	}
	service := newEvaluationService(t, repository)

	items, err := service.CreateDatasetItems(t.Context(), datasetID, []domain.CreateDatasetItemInput{
		{
			Input:   map[string]any{"question": " What is Assay? "},
			Output:  "An evaluation platform",
			Context: []domain.Chunk{{ID: "k0", Text: "Assay evaluates AI systems."}},
		},
	})
	if err != nil {
		t.Fatalf("create dataset items: %v", err)
	}
	if len(items) != 1 || repository.items[0].Input["question"] != "What is Assay?" {
		t.Fatalf("created items = %#v", items)
	}
	items, err = service.CreateDatasetItems(t.Context(), datasetID, []domain.CreateDatasetItemInput{{
		Input: map[string]any{"question": "Generate this"},
	}})
	if err != nil || items[0].Output != nil {
		t.Fatalf("create generation dataset item = %#v, %v", items, err)
	}

	_, err = service.CreateDatasetItems(t.Context(), datasetID, []domain.CreateDatasetItemInput{
		{Input: map[string]any{"question": ""}, Output: "answer"},
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("blank question error = %v, want ErrInvalid", err)
	}
}

func TestEvaluationServiceResolvesScorerOverrides(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	projectID := uuid.Must(uuid.NewV7())
	cipher, err := secretcrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	projectSecret, err := cipher.Encrypt([]byte("project-secret"))
	if err != nil {
		t.Fatalf("encrypt project secret: %v", err)
	}
	repository := &evaluationRepositoryFake{
		application: domain.Application{ID: applicationID, ProjectID: projectID},
		project: domain.Project{ID: projectID, JudgeConfig: &domain.JudgeConfig{
			BaseURL:          "https://judge.example/v1",
			APIKeyCiphertext: projectSecret,
		}},
		configs: []domain.ScorerConfig{{
			ID:               uuid.Must(uuid.NewV7()),
			ApplicationID:    applicationID,
			Scorer:           domain.ScorerGroundedness,
			Enabled:          true,
			Threshold:        0.7,
			PromptTemplateID: domain.GroundednessPromptV1,
			JudgeConfig:      &domain.JudgeConfig{Model: "project-model"},
		}},
	}
	service := domain.NewEvaluationService(repository, cipher, 3)

	configs, err := service.ResolveScorerConfigs(
		t.Context(), applicationID, []string{domain.ScorerGroundedness},
		domain.JudgeDefaults{Model: "global-model"},
	)
	if err != nil {
		t.Fatalf("resolve scorer config: %v", err)
	}
	got := configs[0]
	if got.Threshold != 0.7 || got.Judge.BaseURL != "https://judge.example/v1" ||
		got.Judge.Model != "project-model" || got.Judge.APIKey != "project-secret" {
		t.Fatalf("resolved config = %#v", got)
	}
}

func TestEvaluationServiceCreatesRunIntent(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	datasetID := uuid.Must(uuid.NewV7())
	repository := &evaluationRepositoryFake{
		application: domain.Application{ID: applicationID},
		dataset:     domain.Dataset{ID: datasetID, ApplicationID: applicationID},
		itemCount:   2,
	}
	service := newEvaluationService(t, repository)

	run, err := service.CreateEvalRun(t.Context(), domain.CreateEvalRunInput{
		ApplicationID: applicationID,
		DatasetID:     datasetID,
		Name:          "baseline",
		Mode:          domain.EvalModeScoreExisting,
		Scorers:       []string{domain.ScorerGroundedness, domain.ScorerCorrectness},
	})
	if err != nil {
		t.Fatalf("create eval run: %v", err)
	}
	if run.Status != domain.EvalStatusPending || repository.job.MaxAttempts != 3 ||
		repository.job.EvalRunID == nil || *repository.job.EvalRunID != run.ID {
		t.Fatalf("run/job = %#v/%#v", run, repository.job)
	}
}

func TestEvaluationServiceValidatesRunModeRequirements(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	datasetID := uuid.Must(uuid.NewV7())
	tests := []struct {
		name        string
		mode        string
		scorers     []string
		application domain.Application
		missingOut  int
		missingRef  int
		wantError   bool
	}{
		{
			name: "existing output required", mode: domain.EvalModeScoreExisting,
			scorers: []string{domain.ScorerGroundedness}, missingOut: 1, wantError: true,
		},
		{
			name: "generated endpoint required", mode: domain.EvalModeGenerateThenScore,
			scorers: []string{domain.ScorerGroundedness}, wantError: true,
		},
		{
			name: "generated output accepted", mode: domain.EvalModeGenerateThenScore,
			scorers: []string{domain.ScorerGroundedness}, missingOut: 1,
			application: domain.Application{TargetEndpoint: &domain.TargetEndpoint{}},
		},
		{
			name: "correctness reference required", mode: domain.EvalModeGenerateThenScore,
			scorers: []string{domain.ScorerCorrectness}, missingRef: 1, wantError: true,
			application: domain.Application{TargetEndpoint: &domain.TargetEndpoint{}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &evaluationRepositoryFake{
				application: test.application, dataset: domain.Dataset{
					ID: datasetID, ApplicationID: applicationID,
				},
				itemCount: 1, missingOutput: test.missingOut, missingReference: test.missingRef,
			}
			repository.application.ID = applicationID
			service := newEvaluationService(t, repository)
			run, err := service.CreateEvalRun(t.Context(), domain.CreateEvalRunInput{
				ApplicationID: applicationID, DatasetID: datasetID, Name: "run",
				Mode: test.mode, Scorers: test.scorers,
			})
			if test.wantError && !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("create eval run error = %v, want ErrInvalid", err)
			}
			if !test.wantError && (err != nil || run.Mode != test.mode) {
				t.Fatalf("create eval run = %#v, %v", run, err)
			}
		})
	}
}

func TestEvaluationServiceRejectsDisabledRunScorer(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	datasetID := uuid.Must(uuid.NewV7())
	repository := &evaluationRepositoryFake{
		application: domain.Application{ID: applicationID},
		dataset:     domain.Dataset{ID: datasetID, ApplicationID: applicationID},
		itemCount:   1,
		configs: []domain.ScorerConfig{{
			ApplicationID: applicationID,
			Scorer:        domain.ScorerGroundedness,
			Enabled:       false,
		}},
	}
	service := newEvaluationService(t, repository)
	_, err := service.CreateEvalRun(t.Context(), domain.CreateEvalRunInput{
		ApplicationID: applicationID,
		DatasetID:     datasetID,
		Name:          "disabled",
		Scorers:       []string{domain.ScorerGroundedness},
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("disabled scorer error = %v, want ErrInvalid", err)
	}
}

func TestEvaluationServicePutReplacesOverrideAndPreservesSecret(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	secret := []byte("encrypted-envelope")
	repository := &evaluationRepositoryFake{
		application: domain.Application{ID: applicationID},
		configs: []domain.ScorerConfig{{
			ID: uuid.Must(uuid.NewV7()), ApplicationID: applicationID,
			Scorer: domain.ScorerGroundedness, Enabled: false, Threshold: 0.9,
			PromptTemplateID: domain.GroundednessPromptV1,
			JudgeConfig: &domain.JudgeConfig{
				BaseURL: "https://old.example/v1", Model: "old",
				APIKeyCiphertext: secret,
			},
		}},
	}
	service := newEvaluationService(t, repository)
	config, err := service.PutScorerConfig(
		t.Context(), applicationID, domain.ScorerGroundedness, domain.PutScorerConfigInput{},
	)
	if err != nil {
		t.Fatalf("replace scorer config: %v", err)
	}
	if !config.Enabled || config.Threshold != 0.5 || config.JudgeConfig == nil ||
		config.JudgeConfig.BaseURL != "" || config.JudgeConfig.Model != "" ||
		!slices.Equal(config.JudgeConfig.APIKeyCiphertext, secret) {
		t.Fatalf("replaced scorer config = %#v", config)
	}
}

func newEvaluationService(
	t *testing.T,
	repository domain.EvaluationRepository,
) *domain.EvaluationService {
	t.Helper()
	cipher, err := secretcrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	return domain.NewEvaluationService(repository, cipher, 3)
}

type evaluationRepositoryFake struct {
	domain.EvaluationRepository
	application      domain.Application
	project          domain.Project
	dataset          domain.Dataset
	items            []domain.DatasetItem
	configs          []domain.ScorerConfig
	itemCount        int
	missingOutput    int
	missingReference int
	job              domain.Job
	upserted         domain.ScorerConfig
}

func (f *evaluationRepositoryFake) GetApplication(
	_ context.Context,
	_ uuid.UUID,
) (domain.Application, error) {
	return f.application, nil
}

func (f *evaluationRepositoryFake) GetProject(
	_ context.Context,
	_ uuid.UUID,
) (domain.Project, error) {
	return f.project, nil
}

func (f *evaluationRepositoryFake) GetDataset(
	_ context.Context,
	_ uuid.UUID,
) (domain.Dataset, error) {
	return f.dataset, nil
}

func (f *evaluationRepositoryFake) CreateDatasetItems(
	_ context.Context,
	_ uuid.UUID,
	items []domain.DatasetItem,
) ([]domain.DatasetItem, error) {
	f.items = items
	return items, nil
}

func (f *evaluationRepositoryFake) ListScorerConfigs(
	_ context.Context,
	_ uuid.UUID,
) ([]domain.ScorerConfig, error) {
	return f.configs, nil
}

func (f *evaluationRepositoryFake) UpsertScorerConfig(
	_ context.Context,
	config domain.ScorerConfig,
) (domain.ScorerConfig, error) {
	f.upserted = config
	return config, nil
}

func (f *evaluationRepositoryFake) CountDatasetItems(
	_ context.Context,
	_ uuid.UUID,
) (int, error) {
	return f.itemCount, nil
}

func (f *evaluationRepositoryFake) CountDatasetItemsMissingOutput(
	_ context.Context,
	_ uuid.UUID,
) (int, error) {
	return f.missingOutput, nil
}

func (f *evaluationRepositoryFake) CountDatasetItemsMissingReference(
	_ context.Context,
	_ uuid.UUID,
) (int, error) {
	return f.missingReference, nil
}

func (f *evaluationRepositoryFake) CreateEvalRun(
	_ context.Context,
	run domain.EvalRun,
	job domain.Job,
) (domain.EvalRun, error) {
	f.job = job
	return run, nil
}
