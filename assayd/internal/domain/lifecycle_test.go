package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestQueueScoresAdminQueuesOneProject(t *testing.T) {
	projectID := uuid.Must(uuid.NewV7())
	applicationID := uuid.Must(uuid.NewV7())
	application := domain.Application{ID: applicationID, ProjectID: projectID}
	first := adminScoreTrace(applicationID, 1)
	second := adminScoreTrace(applicationID, 2)
	repository := &traceRepositoryFake{
		applications: map[uuid.UUID]domain.Application{applicationID: application},
		details: map[uuid.UUID]domain.Trace{
			first.ID: first, second.ID: second,
		},
	}
	service := domain.NewTraceService(repository, &applicationCreatorFake{}, 3)

	jobs, err := service.QueueScoresAdmin(
		t.Context(), []uuid.UUID{first.ID, second.ID}, []string{domain.ScorerGroundedness},
	)
	if err != nil {
		t.Fatalf("queue admin scores: %v", err)
	}
	if len(jobs) != 2 || repository.queueCalls != 1 || len(repository.requests) != 2 {
		t.Fatalf("admin queue jobs = %#v, requests = %#v", jobs, repository.requests)
	}
}

func TestQueueScoresAdminRejectsMixedProjectsAtomically(t *testing.T) {
	projectID := uuid.Must(uuid.NewV7())
	otherProjectID := uuid.Must(uuid.NewV7())
	firstApp := uuid.Must(uuid.NewV7())
	secondApp := uuid.Must(uuid.NewV7())
	first := adminScoreTrace(firstApp, 1)
	second := adminScoreTrace(secondApp, 2)
	repository := &traceRepositoryFake{
		applications: map[uuid.UUID]domain.Application{
			firstApp:  {ID: firstApp, ProjectID: projectID},
			secondApp: {ID: secondApp, ProjectID: otherProjectID},
		},
		details: map[uuid.UUID]domain.Trace{
			first.ID: first, second.ID: second,
		},
	}
	service := domain.NewTraceService(repository, &applicationCreatorFake{}, 3)

	_, err := service.QueueScoresAdmin(
		t.Context(), []uuid.UUID{first.ID, second.ID}, []string{domain.ScorerGroundedness},
	)
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("mixed project error = %v, want ErrInvalid", err)
	}
	if repository.queueCalls != 0 || len(repository.requests) != 0 {
		t.Fatalf("mixed project queued %d requests", len(repository.requests))
	}
}

func TestAttachReferenceAdminTrimsAndScopes(t *testing.T) {
	projectID := uuid.Must(uuid.NewV7())
	applicationID := uuid.Must(uuid.NewV7())
	application := domain.Application{ID: applicationID, ProjectID: projectID}
	trace := domain.Trace{ID: uuid.Must(uuid.NewV7()), ApplicationID: applicationID}
	repository := &traceRepositoryFake{
		application:  application,
		applications: map[uuid.UUID]domain.Application{applicationID: application},
		trace:        trace,
	}
	service := domain.NewTraceService(repository, &applicationCreatorFake{}, 3)

	updated, err := service.AttachReferenceAdmin(t.Context(), trace.ID, "  expected  ")
	if err != nil {
		t.Fatalf("attach admin reference: %v", err)
	}
	if repository.reference != "expected" || repository.projectID != projectID ||
		updated.ReferenceAnswer == nil || *updated.ReferenceAnswer != "expected" {
		t.Fatalf("attached reference = %#v, repository = %#v", updated, repository)
	}
	if _, err := service.AttachReferenceAdmin(t.Context(), trace.ID, "  "); !errors.Is(
		err, domain.ErrInvalid,
	) {
		t.Fatalf("blank admin reference error = %v, want ErrInvalid", err)
	}
}

func TestAdminDeletesForwardRepositoryErrors(t *testing.T) {
	traceID := uuid.Must(uuid.NewV7())
	repository := &traceRepositoryFake{detailErr: domain.ErrNotFound}
	service := domain.NewTraceService(repository, &applicationCreatorFake{}, 3)

	if err := service.DeleteAdmin(t.Context(), traceID); err != nil {
		t.Fatalf("delete admin trace: %v", err)
	}
	if repository.deletedTraceID != traceID {
		t.Fatalf("deleted trace ID = %s, want %s", repository.deletedTraceID, traceID)
	}
}

func adminScoreTrace(applicationID uuid.UUID, seed byte) domain.Trace {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return domain.Trace{
		ID:            uuid.Must(uuid.NewV7()),
		ApplicationID: applicationID,
		OTelTraceID:   [16]byte{seed},
		RootName:      "answer",
		StartTime:     now,
		EndTime:       now.Add(time.Second),
		Status:        "ok",
		Attributes:    map[string]any{"service.name": "support"},
		Spans: []domain.Span{{
			ApplicationID: applicationID,
			OTelSpanID:    [8]byte{seed},
			Name:          "answer",
			Kind:          "server",
			StartTime:     now,
			EndTime:       now.Add(time.Second),
			StatusCode:    "ok",
			IsScorable:    true,
			Attributes: map[string]any{
				"gen_ai.input.messages": []any{
					map[string]any{"role": "user", "content": "question"},
				},
				"gen_ai.output.messages": []any{
					map[string]any{"role": "assistant", "content": "answer"},
				},
				"assay.context.chunk.count":   1,
				"assay.context.chunks.0.id":   "c0",
				"assay.context.chunks.0.text": "retrieved context",
			},
			Events: []domain.SpanEvent{},
		}},
	}
}
