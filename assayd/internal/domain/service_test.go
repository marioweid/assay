package domain_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/auth"
	secretcrypto "github.com/marioweid/assay/assayd/internal/crypto"
	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/migrate"
	"github.com/marioweid/assay/assayd/internal/store"
	"github.com/marioweid/assay/assayd/internal/testutil"

	"github.com/google/uuid"
)

func TestServiceEncryptsProjectSecretsAndRejectsDuplicateName(t *testing.T) {
	service, cipher := newService(t)
	judgeSecret := "judge-secret"
	project, err := service.CreateProject(t.Context(), domain.CreateProjectInput{
		Name: "  Support  ",
		JudgeConfig: &domain.JudgeConfigInput{
			BaseURL: "https://judge.example.com/v1",
			Model:   "judge-model",
			APIKey:  &judgeSecret,
		},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.Name != "Support" {
		t.Fatalf("project name = %q, want %q", project.Name, "Support")
	}
	assertEncrypted(t, cipher, project.JudgeConfig.APIKeyCiphertext, judgeSecret)

	_, err = service.CreateProject(t.Context(), domain.CreateProjectInput{Name: "Support"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate project error = %v, want conflict", err)
	}
}

func TestServiceAuthenticatesAndRevokesAPIKeys(t *testing.T) {
	service, _ := newService(t)
	project := createProject(t, service)
	createdKey, err := service.CreateAPIKey(t.Context(), project.ID, "CI")
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}
	if !auth.ValidAPIKeyFormat(createdKey.Key) {
		t.Fatalf("created key %q has invalid format", createdKey.Key)
	}
	projectID, err := service.AuthenticateAPIKey(t.Context(), createdKey.Key)
	if err != nil {
		t.Fatalf("authenticate API key: %v", err)
	}
	if projectID != project.ID {
		t.Fatalf("authenticated project = %s, want %s", projectID, project.ID)
	}
	assertAPIKeyUsageRecorded(t, service, project.ID)
	if err := service.RevokeAPIKey(t.Context(), project.ID, createdKey.ID); err != nil {
		t.Fatalf("revoke API key: %v", err)
	}
	_, err = service.AuthenticateAPIKey(t.Context(), createdKey.Key)
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("authentication after revocation error = %v, want unauthorized", err)
	}
}

func assertAPIKeyUsageRecorded(t *testing.T, service *domain.Service, projectID uuid.UUID) {
	t.Helper()
	keys, err := service.ListAPIKeys(t.Context(), projectID)
	if err != nil {
		t.Fatalf("list used API key: %v", err)
	}
	if len(keys) != 1 || keys[0].LastUsedAt == nil {
		t.Fatalf("authenticated API key usage = %#v, want last-used timestamp", keys)
	}
}

func TestServicePreservesAndClearsEndpointSecret(t *testing.T) {
	service, cipher := newService(t)
	application := createApplication(t, service, createProject(t, service).ID)
	endpointSecret := "endpoint-secret"
	application, err := service.SetApplicationEndpoint(
		t.Context(),
		application.ID,
		domain.EndpointPatch{Endpoint: endpointInput(&endpointSecret)},
	)
	if err != nil {
		t.Fatalf("set application endpoint: %v", err)
	}
	if application.TargetEndpoint.Method != "POST" {
		t.Fatalf("endpoint method = %q, want POST", application.TargetEndpoint.Method)
	}
	if application.TargetEndpoint.TimeoutMS != 30000 {
		t.Fatalf("endpoint timeout = %d, want 30000", application.TargetEndpoint.TimeoutMS)
	}
	assertEncrypted(t, cipher, application.TargetEndpoint.SecretCiphertext, endpointSecret)
	originalCiphertext := bytes.Clone(application.TargetEndpoint.SecretCiphertext)

	update := endpointInput(nil)
	update.URL = "https://target.example.com/v2/answer"
	application, err = service.SetApplicationEndpoint(
		t.Context(),
		application.ID,
		domain.EndpointPatch{Endpoint: update},
	)
	if err != nil {
		t.Fatalf("update application endpoint: %v", err)
	}
	if !bytes.Equal(application.TargetEndpoint.SecretCiphertext, originalCiphertext) {
		t.Fatal("endpoint update without secret replaced existing ciphertext")
	}
	application, err = service.SetApplicationEndpoint(
		t.Context(),
		application.ID,
		domain.EndpointPatch{Clear: true},
	)
	if err != nil {
		t.Fatalf("clear application endpoint: %v", err)
	}
	if application.TargetEndpoint != nil {
		t.Fatalf("target endpoint = %#v, want nil", application.TargetEndpoint)
	}
}

func TestServiceResolvesExecutableTargetEndpoint(t *testing.T) {
	service, _ := newService(t)
	application := createApplication(t, service, createProject(t, service).ID)
	secret := "endpoint-secret"
	application, err := service.SetApplicationEndpoint(
		t.Context(), application.ID,
		domain.EndpointPatch{Endpoint: endpointInput(&secret)},
	)
	if err != nil {
		t.Fatalf("set application endpoint: %v", err)
	}

	resolved, err := service.ResolveTargetEndpoint(t.Context(), application.ID)
	if err != nil {
		t.Fatalf("resolve target endpoint: %v", err)
	}
	if resolved.URL != application.TargetEndpoint.URL || resolved.Secret != secret ||
		resolved.Timeout != 30*time.Second {
		t.Fatalf("resolved target endpoint = %#v", resolved)
	}
	resolved.Headers["X-Test"] = "changed"
	if application.TargetEndpoint.Headers["X-Test"] == "changed" {
		t.Fatal("resolved target headers alias persisted configuration")
	}
}

func TestServiceFiltersApplicationsAndCascadesProjectDelete(t *testing.T) {
	service, _ := newService(t)
	project := createProject(t, service)
	application := createApplication(t, service, project.ID)
	applications, err := service.ListApplications(t.Context(), &project.ID)
	if err != nil {
		t.Fatalf("list applications: %v", err)
	}
	if len(applications) != 1 || applications[0].ID != application.ID {
		t.Fatalf("applications = %#v, want created application", applications)
	}
	if err := service.DeleteProject(t.Context(), project.ID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	_, err = service.GetApplication(t.Context(), application.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get cascaded application error = %v, want not found", err)
	}
}

func TestServiceValidatesAutomaticScorers(t *testing.T) {
	service, _ := newService(t)
	project := createProject(t, service)
	_, err := service.CreateApplication(t.Context(), domain.CreateApplicationInput{
		ProjectID: project.ID, Name: "invalid", Slug: "invalid",
		AutoScoreScorers: []string{"unknown"},
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown automatic scorer error = %v, want ErrInvalid", err)
	}
	application, err := service.CreateApplication(t.Context(), domain.CreateApplicationInput{
		ProjectID: project.ID, Name: "valid", Slug: "valid",
		AutoScoreScorers: []string{domain.ScorerGroundedness},
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	duplicates := []string{domain.ScorerCorrectness, domain.ScorerCorrectness}
	_, err = service.UpdateApplication(t.Context(), application.ID, domain.UpdateApplicationInput{
		AutoScoreScorers: &duplicates,
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("duplicate automatic scorer error = %v, want ErrInvalid", err)
	}
}

func TestServiceRejectsInvalidInputs(t *testing.T) {
	service, _ := newService(t)
	if _, err := service.CreateProject(
		t.Context(),
		domain.CreateProjectInput{Name: "  "},
	); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("blank project error = %v, want invalid", err)
	}
	if _, err := service.AuthenticateAPIKey(t.Context(), "not-a-key"); !errors.Is(
		err,
		domain.ErrUnauthorized,
	) {
		t.Fatalf("malformed key error = %v, want unauthorized", err)
	}
	_, err := service.SetApplicationEndpoint(
		t.Context(),
		domain.Application{}.ID,
		domain.EndpointPatch{
			Clear:    true,
			Endpoint: &domain.TargetEndpointInput{URL: "https://example.com"},
		},
	)
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("set-and-clear endpoint error = %v, want invalid", err)
	}
}

func createProject(t *testing.T, service *domain.Service) domain.Project {
	t.Helper()
	project, err := service.CreateProject(t.Context(), domain.CreateProjectInput{Name: "Support"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return project
}

func createApplication(
	t *testing.T,
	service *domain.Service,
	projectID uuid.UUID,
) domain.Application {
	t.Helper()
	application, err := service.CreateApplication(t.Context(), domain.CreateApplicationInput{
		ProjectID:        projectID,
		Name:             "Support Bot",
		Slug:             "support-bot",
		Config:           map[string]any{"environment": "test"},
		AutoScoreScorers: []string{"groundedness"},
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	if application.Config["environment"] != "test" {
		t.Fatalf("application config = %#v", application.Config)
	}
	return application
}

func endpointInput(secret *string) *domain.TargetEndpointInput {
	return &domain.TargetEndpointInput{
		URL:     "https://target.example.com/answer",
		Headers: map[string]string{"Authorization": "Bearer {{secret}}"},
		ResponseMapping: domain.ResponseMapping{
			Output:  "$.answer",
			Context: "$.sources[*].text",
		},
		Secret: secret,
	}
}

func newService(t *testing.T) (*domain.Service, *secretcrypto.Cipher) {
	t.Helper()
	database, err := store.Open(t.Context(), testutil.Postgres(t))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Up(t.Context(), database.MigrationDB(), logger); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	cipher, err := secretcrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("create secret cipher: %v", err)
	}
	return domain.NewService(database, cipher), cipher
}

func assertEncrypted(
	t *testing.T,
	cipher *secretcrypto.Cipher,
	ciphertext []byte,
	want string,
) {
	t.Helper()
	if bytes.Equal(ciphertext, []byte(want)) {
		t.Fatal("ciphertext equals plaintext")
	}
	plaintext, err := cipher.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt stored secret: %v", err)
	}
	if string(plaintext) != want {
		t.Fatalf("decrypted secret = %q, want %q", plaintext, want)
	}
}
