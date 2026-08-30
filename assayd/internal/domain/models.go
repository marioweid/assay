// Package domain owns Assay's application rules and orchestration.
package domain

import (
	"crypto/sha256"
	"time"

	"github.com/google/uuid"
)

// Project groups applications and API keys.
type Project struct {
	ID          uuid.UUID
	Name        string
	JudgeConfig *JudgeConfig
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// JudgeConfig overrides the process-level judge configuration for a project.
type JudgeConfig struct {
	BaseURL          string `json:"base_url,omitempty"`
	Model            string `json:"model,omitempty"`
	APIKeyCiphertext []byte `json:"api_key_ciphertext,omitempty"`
}

// JudgeConfigInput accepts a write-only judge API key.
type JudgeConfigInput struct {
	BaseURL string
	Model   string
	APIKey  *string
}

// CreateProjectInput contains values accepted when creating a project.
type CreateProjectInput struct {
	Name        string
	JudgeConfig *JudgeConfigInput
}

// UpdateProjectInput contains optional project changes.
type UpdateProjectInput struct {
	Name             *string
	JudgeConfig      *JudgeConfigInput
	ClearJudgeConfig bool
}

// APIKey is stored project-key metadata and its one-way digest.
type APIKey struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	Name       string
	KeyHash    [sha256.Size]byte
	KeyPrefix  string
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreatedAPIKey includes the plaintext key returned once at creation.
type CreatedAPIKey struct {
	APIKey
	Key string
}

// ResponseMapping identifies output and context fields in a target response.
type ResponseMapping struct {
	Output  string `json:"output"`
	Context string `json:"context,omitempty"`
}

// TargetEndpoint configures offline generation against an external endpoint.
type TargetEndpoint struct {
	URL              string            `json:"url"`
	Method           string            `json:"method"`
	Headers          map[string]string `json:"headers,omitempty"`
	RequestTemplate  map[string]any    `json:"request_template,omitempty"`
	ResponseMapping  ResponseMapping   `json:"response_mapping"`
	TimeoutMS        int               `json:"timeout_ms"`
	SecretCiphertext []byte            `json:"secret_ciphertext,omitempty"`
}

// TargetEndpointInput accepts write-only target credentials.
type TargetEndpointInput struct {
	URL             string
	Method          string
	Headers         map[string]string
	RequestTemplate map[string]any
	ResponseMapping ResponseMapping
	TimeoutMS       int
	Secret          *string
}

// EndpointPatch either replaces or clears an application's target endpoint.
type EndpointPatch struct {
	Endpoint *TargetEndpointInput
	Clear    bool
}

// Application is a GenAI use case owned by a project.
type Application struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	Name             string
	Slug             string
	Config           map[string]any
	AutoScoreScorers []string
	TargetEndpoint   *TargetEndpoint
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CreateApplicationInput contains values accepted when creating an application.
type CreateApplicationInput struct {
	ProjectID        uuid.UUID
	Name             string
	Slug             string
	Config           map[string]any
	AutoScoreScorers []string
}

// UpdateApplicationInput contains optional application changes.
type UpdateApplicationInput struct {
	Name             *string
	Slug             *string
	Config           *map[string]any
	AutoScoreScorers *[]string
}

// Trace is one application-owned OpenTelemetry trace and its optional spans.
type Trace struct {
	ID              uuid.UUID
	ApplicationID   uuid.UUID
	OTelTraceID     [16]byte
	RootName        string
	StartTime       time.Time
	EndTime         time.Time
	Status          string
	SpanCount       int
	TotalTokens     int64
	TotalCost       *string
	ReferenceAnswer *string
	Attributes      map[string]any
	Spans           []Span
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Span is one stored OpenTelemetry span.
type Span struct {
	ID              int64
	TraceID         uuid.UUID
	ApplicationID   uuid.UUID
	OTelSpanID      [8]byte
	ParentSpanID    *[8]byte
	Name            string
	Kind            string
	OperationName   string
	StartTime       time.Time
	EndTime         time.Time
	DurationMS      int64
	StatusCode      string
	StatusMessage   string
	IsScorable      bool
	ScorableKind    string
	Attributes      map[string]any
	Events          []SpanEvent
	InputTokens     int64
	OutputTokens    int64
	ReferenceAnswer *string
	CreatedAt       time.Time
}

// SpanEvent is an event attached to an OpenTelemetry span.
type SpanEvent struct {
	Time                   time.Time      `json:"time"`
	Name                   string         `json:"name"`
	Attributes             map[string]any `json:"attributes"`
	DroppedAttributesCount uint32         `json:"dropped_attributes_count"`
}

// TraceCursor identifies the last item returned by a trace page.
type TraceCursor struct {
	StartTime time.Time
	ID        uuid.UUID
}

// TraceQuery contains project-scoped trace list filters.
type TraceQuery struct {
	ApplicationID *uuid.UUID
	Start         *time.Time
	End           *time.Time
	Status        string
	Limit         int
	Cursor        *TraceCursor
}

// TracePage is one cursor-paginated trace list result.
type TracePage struct {
	Items      []Trace
	NextCursor *TraceCursor
}
