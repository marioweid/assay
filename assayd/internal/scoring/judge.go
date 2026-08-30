// Package scoring implements Assay's built-in evaluation scorers and judge adapter.
package scoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/marioweid/assay/assayd/internal/domain"
)

const maxJudgeResponseBytes = 1 << 20

// JudgeRequest contains structured prompt inputs for a judge call.
type JudgeRequest struct {
	System     string
	User       any
	Correction string
}

// JudgeResponse contains raw structured content and token usage.
type JudgeResponse struct {
	Content string
	Tokens  int
}

// Judge completes structured scoring prompts.
type Judge interface {
	Complete(context.Context, JudgeRequest) (JudgeResponse, error)
}

type judgeError struct {
	operation string
	retryable bool
	cause     error
}

func (e *judgeError) Error() string { return fmt.Sprintf("%s: %v", e.operation, e.cause) }
func (e *judgeError) Unwrap() error { return e.cause }

// IsRetryable reports whether a failed judge call may succeed later.
func IsRetryable(err error) bool {
	var target *judgeError
	return errors.As(err, &target) && target.retryable
}

type httpJudge struct {
	client *http.Client
	config domain.ResolvedJudgeConfig
}

// NewHTTPJudge creates an OpenAI-compatible HTTP judge.
func NewHTTPJudge(client *http.Client, config domain.ResolvedJudgeConfig) Judge {
	return &httpJudge{client: client, config: config}
}

func (j *httpJudge) Complete(
	ctx context.Context,
	request JudgeRequest,
) (JudgeResponse, error) {
	httpRequest, err := j.newRequest(ctx, request)
	if err != nil {
		return JudgeResponse{}, err
	}
	response, err := j.client.Do(httpRequest)
	if err != nil {
		return JudgeResponse{}, requestError(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return JudgeResponse{}, responseError(response.StatusCode)
	}
	return decodeJudgeResponse(response)
}

func (j *httpJudge) newRequest(ctx context.Context, request JudgeRequest) (*http.Request, error) {
	body, err := judgeRequestBody(j.config.Model, request)
	if err != nil {
		return nil, &judgeError{operation: "encode judge request", cause: err}
	}
	endpoint := strings.TrimRight(j.config.BaseURL, "/") + "/chat/completions"
	httpRequest, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint, bytes.NewReader(body),
	)
	if err != nil {
		return nil, &judgeError{operation: "create judge request", cause: err}
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if j.config.APIKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+j.config.APIKey)
	}
	return httpRequest, nil
}

func requestError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	return &judgeError{
		operation: "send judge request", retryable: true, cause: errors.New("request failed"),
	}
}

func responseError(status int) error {
	return &judgeError{
		operation: "judge response",
		retryable: status == http.StatusRequestTimeout || status == http.StatusTooManyRequests ||
			status >= 500,
		cause: fmt.Errorf("HTTP status %d", status),
	}
}

func judgeRequestBody(model string, request JudgeRequest) ([]byte, error) {
	user, err := json.Marshal(request.User)
	if err != nil {
		return nil, err
	}
	messages := []map[string]string{
		{"role": "system", "content": request.System},
		{"role": "user", "content": string(user)},
	}
	if request.Correction != "" {
		messages = append(messages, map[string]string{
			"role": "system", "content": "Correct the JSON response: " + request.Correction,
		})
	}
	return json.Marshal(map[string]any{
		"model": model, "temperature": 0, "messages": messages,
		"response_format": map[string]string{"type": "json_object"},
	})
}

func decodeJudgeResponse(response *http.Response) (JudgeResponse, error) {
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxJudgeResponseBytes+1))
	if err != nil {
		return JudgeResponse{}, &judgeError{
			operation: "read judge response", retryable: true, cause: errors.New("response read failed"),
		}
	}
	if len(payload) > maxJudgeResponseBytes {
		return JudgeResponse{}, &judgeError{
			operation: "decode judge response", cause: errors.New("response exceeds size limit"),
		}
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return JudgeResponse{}, &judgeError{operation: "decode judge response", cause: err}
	}
	if len(envelope.Choices) == 0 || strings.TrimSpace(envelope.Choices[0].Message.Content) == "" {
		return JudgeResponse{}, &judgeError{
			operation: "decode judge response", cause: errors.New("missing choice content"),
		}
	}
	return JudgeResponse{
		Content: envelope.Choices[0].Message.Content,
		Tokens:  envelope.Usage.TotalTokens,
	}, nil
}
