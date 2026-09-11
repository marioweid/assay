package domain

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// QueueScoresAdmin validates and queues scores without requiring a project credential.
func (s *TraceService) QueueScoresAdmin(
	ctx context.Context,
	traceIDs []uuid.UUID,
	scorers []string,
) ([]Job, error) {
	if err := validateTraceScoreSelection(traceIDs, scorers); err != nil {
		return nil, err
	}
	traces, projectID, err := s.loadAdminTraceBatch(ctx, traceIDs)
	if err != nil {
		return nil, err
	}
	return s.queueScoresForTraces(ctx, projectID, traces, scorers, true)
}

func (s *TraceService) loadAdminTraceBatch(
	ctx context.Context,
	traceIDs []uuid.UUID,
) ([]Trace, uuid.UUID, error) {
	traces := make([]Trace, 0, len(traceIDs))
	projectID := uuid.Nil
	for _, traceID := range traceIDs {
		trace, err := s.repository.GetTraceDetailByID(ctx, traceID)
		if err != nil {
			return nil, uuid.Nil, fmt.Errorf("queue admin trace scores: %w", err)
		}
		application, err := s.repository.GetApplication(ctx, trace.ApplicationID)
		if err != nil {
			return nil, uuid.Nil, fmt.Errorf("queue admin trace scores: %w", err)
		}
		if projectID != uuid.Nil && application.ProjectID != projectID {
			return nil, uuid.Nil, fmt.Errorf(
				"queue admin trace scores: %w: traces belong to different projects", ErrInvalid,
			)
		}
		projectID = application.ProjectID
		traces = append(traces, trace)
	}
	return traces, projectID, nil
}

// AttachReferenceAdmin stores a reference without requiring a project credential.
func (s *TraceService) AttachReferenceAdmin(
	ctx context.Context,
	traceID uuid.UUID,
	reference string,
) (Trace, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Trace{}, fmt.Errorf("attach admin trace reference: %w: reference is blank", ErrInvalid)
	}
	trace, err := s.repository.GetTraceDetailByID(ctx, traceID)
	if err != nil {
		return Trace{}, fmt.Errorf("attach admin trace reference: %w", err)
	}
	application, err := s.repository.GetApplication(ctx, trace.ApplicationID)
	if err != nil {
		return Trace{}, fmt.Errorf("attach admin trace reference: %w", err)
	}
	return s.attachReference(ctx, application.ProjectID, trace, reference)
}

// DeleteAdmin permanently removes a trace and its dependent telemetry.
func (s *TraceService) DeleteAdmin(ctx context.Context, traceID uuid.UUID) error {
	if err := s.repository.DeleteTrace(ctx, traceID); err != nil {
		return fmt.Errorf("delete trace %s: %w", traceID, err)
	}
	return nil
}

// DeleteEvalRun permanently removes a terminal evaluation run and its dependent records.
func (s *EvaluationService) DeleteEvalRun(ctx context.Context, runID uuid.UUID) error {
	if err := s.repository.DeleteEvalRun(ctx, runID); err != nil {
		return fmt.Errorf("delete eval run %s: %w", runID, err)
	}
	return nil
}
