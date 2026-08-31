package target

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/marioweid/assay/assayd/internal/domain"
)

const defaultMaxResponseBytes int64 = 8 << 20

type requestError struct {
	err       error
	retryable bool
}

func (e *requestError) Error() string { return e.err.Error() }
func (e *requestError) Unwrap() error { return e.err }

// IsRetryable reports whether a target failure can succeed on a later job attempt.
func IsRetryable(err error) bool {
	var targetErr *requestError
	return errors.As(err, &targetErr) && targetErr.retryable
}

// Client performs bounded target endpoint requests.
type Client struct {
	httpClient       *http.Client
	maxResponseBytes int64
}

// NewClient constructs a target client that rejects redirects.
func NewClient(transport http.RoundTripper) *Client {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		maxResponseBytes: defaultMaxResponseBytes,
	}
}

// Compile validates a response mapping for use by Generate.
func (c *Client) Compile(mapping domain.ResponseMapping) (Mapping, error) {
	return Compile(mapping)
}

// Generate calls one configured target endpoint and maps its JSON response.
//
//nolint:cyclop // The HTTP boundary handles each redacted failure stage explicitly.
func (c *Client) Generate(
	ctx context.Context,
	endpoint domain.ResolvedTargetEndpoint,
	mapping Mapping,
	item domain.DatasetItem,
) (domain.Generation, error) {
	headers, body, err := Render(endpoint, item)
	if err != nil {
		return domain.Generation{}, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return domain.Generation{}, fmt.Errorf("encode target request: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, endpoint.Timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestCtx, endpoint.Method, endpoint.URL, bytes.NewReader(payload),
	)
	if err != nil {
		return domain.Generation{}, fmt.Errorf("create target request: %w", domain.ErrInvalid)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	if request.Header.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return domain.Generation{}, &requestError{
			err: fmt.Errorf("call target endpoint: %w", err), retryable: true,
		}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return domain.Generation{}, statusError(response.StatusCode)
	}
	decoded, err := decodeResponse(response.Body, c.maxResponseBytes)
	if err != nil {
		return domain.Generation{}, err
	}
	return mapping.Extract(decoded, item.Context)
}

func statusError(status int) error {
	retryable := status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
	return &requestError{
		err: fmt.Errorf("call target endpoint: HTTP status %d", status), retryable: retryable,
	}
}

func decodeResponse(reader io.Reader, limit int64) (any, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, &requestError{err: fmt.Errorf("read target response: %w", err), retryable: true}
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("decode target response: %w: body exceeds size limit", domain.ErrInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode target response: %w", domain.ErrInvalid)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode target response: %w: trailing JSON", domain.ErrInvalid)
	}
	return decoded, nil
}
