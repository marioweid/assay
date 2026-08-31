package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/app"
	"github.com/marioweid/assay/assayd/internal/config"
	"github.com/marioweid/assay/assayd/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestAppMigratesBeforeServingAndStops(t *testing.T) {
	dsn := testutil.Postgres(t)
	addr := unusedAddress(t)
	judge := newFakeJudge(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := app.New(t.Context(), config.Config{
		HTTPAddr:          addr,
		DatabaseURL:       dsn,
		AdminToken:        "admin-secret",
		EncryptionKey:     config.EncryptionKey{},
		JudgeBaseURL:      judge.server.URL,
		JudgeModel:        "fake-judge",
		WorkerConcurrency: 2,
		JobMaxAttempts:    3,
	}, logger)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})

	if version := migrationVersion(t, dsn); version != 5 {
		t.Fatalf("migration version before serving = %d, want 5", version)
	}

	serveCtx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- application.Serve(serveCtx)
	}()
	waitUntilReady(t, "http://"+addr+"/readyz")
	verifyM2Flow(t, "http://"+addr)
	verifyM3Flow(t, "http://"+addr, judge)

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve after cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("app did not stop within five seconds")
	}
}

type fakeJudge struct {
	server   *httptest.Server
	requests atomic.Int64
	invalid  atomic.Bool
}

type judgeRequestEnvelope struct {
	Temperature    float64 `json:"temperature"`
	ResponseFormat struct {
		Type string `json:"type"`
	} `json:"response_format"`
	Messages []judgeMessage `json:"messages"`
}

type judgeMessage struct {
	Content string `json:"content"`
}

func newFakeJudge(t *testing.T) *fakeJudge {
	t.Helper()
	judge := &fakeJudge{}
	judge.server = httptest.NewServer(http.HandlerFunc(judge.handle))
	t.Cleanup(judge.server.Close)
	return judge
}

func (j *fakeJudge) handle(writer http.ResponseWriter, request *http.Request) {
	var envelope judgeRequestEnvelope
	if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
		j.invalid.Store(true)
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	j.requests.Add(1)
	if envelope.Temperature != 0 || envelope.ResponseFormat.Type != "json_object" ||
		len(envelope.Messages) < 2 {
		j.invalid.Store(true)
	}
	content, found := fakeJudgeContent(envelope.Messages)
	if !found {
		j.invalid.Store(true)
		content = `{}`
	}
	response := map[string]any{
		"choices": []any{map[string]any{"message": map[string]string{"content": content}}},
		"usage":   map[string]int{"total_tokens": 5},
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(response)
}

func fakeJudgeContent(messages []judgeMessage) (string, bool) {
	if len(messages) < 2 {
		return "", false
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal([]byte(messages[1].Content), &input); err != nil {
		return "", false
	}
	switch {
	case input["answer"] != nil:
		return `{"claims":[{"id":"c1","text":"Assay evaluates AI systems."}],` +
			`"excluded":[]}`, true
	case input["context_chunks"] != nil:
		return `{"verdicts":[{"id":"c1","verdict":"supported",` +
			`"supporting_chunk_ids":["k0"],"reason":"Direct support."}]}`, true
	case input["reference"] != nil:
		return `{"reference_facts":[{"fact":"Assay evaluates AI systems.",` +
			`"status":"correct"}],"contradictions":[],"reasoning":"Matches.","score":1}`, true
	default:
		return "", false
	}
}

type m3RunResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Aggregates map[string]struct {
		Mean     float64 `json:"mean"`
		PassRate float64 `json:"pass_rate"`
		N        int     `json:"n"`
	} `json:"aggregates"`
}

func verifyM3Flow(t *testing.T, baseURL string, judge *fakeJudge) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	applicationID := createM3Application(t, client, baseURL)
	datasetID := createM3Dataset(t, client, baseURL, applicationID)
	run := createM3Run(t, client, baseURL, applicationID, datasetID)
	run = waitForRun(t, client, baseURL, run.ID)
	if run.Status != "succeeded" {
		t.Fatalf("M3 run status = %q, want succeeded", run.Status)
	}
	assertM3Scores(t, client, baseURL, run)
	if judge.invalid.Load() || judge.requests.Load() != 3 {
		t.Fatalf("fake judge invalid/requests = %v/%d", judge.invalid.Load(), judge.requests.Load())
	}
}

func createM3Application(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	project := sendAndDecode[struct {
		ID string `json:"id"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost, path: "/v1/projects",
		body: `{"name":"Evaluation"}`, token: "admin-secret",
	}, http.StatusCreated)
	application := sendAndDecode[struct {
		ID string `json:"id"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost, path: "/v1/applications",
		body:  `{"project_id":"` + project.ID + `","name":"Eval Bot","slug":"eval-bot"}`,
		token: "admin-secret",
	}, http.StatusCreated)
	return application.ID
}

func createM3Dataset(
	t *testing.T,
	client *http.Client,
	baseURL string,
	applicationID string,
) string {
	t.Helper()
	dataset := sendAndDecode[struct {
		ID string `json:"id"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost, path: "/v1/datasets",
		body:  `{"application_id":"` + applicationID + `","name":"regression"}`,
		token: "admin-secret",
	}, http.StatusCreated)
	sendAndDecode[struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost, path: "/v1/datasets/" + dataset.ID + "/items",
		body: `{"items":[{"input":{"question":"What is Assay?"},` +
			`"output":"Assay evaluates AI systems.",` +
			`"expected_output":"Assay evaluates AI systems.",` +
			`"context":[{"id":"k0","text":"Assay evaluates AI systems."}]}]}`,
		token: "admin-secret",
	}, http.StatusCreated)
	return dataset.ID
}

func createM3Run(
	t *testing.T,
	client *http.Client,
	baseURL string,
	applicationID string,
	datasetID string,
) m3RunResponse {
	t.Helper()
	return sendAndDecode[m3RunResponse](t, client, baseURL, apiRequest{
		method: http.MethodPost, path: "/v1/runs",
		body: `{"application_id":"` + applicationID + `","dataset_id":"` + datasetID +
			`","name":"baseline","mode":"score_existing",` +
			`"scorers":["groundedness","correctness"]}`,
		token: "admin-secret",
	}, http.StatusAccepted)
}

func waitForRun(
	t *testing.T,
	client *http.Client,
	baseURL string,
	runID string,
) m3RunResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run := sendAndDecode[m3RunResponse](t, client, baseURL, apiRequest{
			method: http.MethodGet, path: "/v1/runs/" + runID, token: "admin-secret",
		}, http.StatusOK)
		if run.Status == "succeeded" || run.Status == "failed" {
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("evaluation run did not reach a terminal state")
	return m3RunResponse{}
}

func assertM3Scores(
	t *testing.T,
	client *http.Client,
	baseURL string,
	run m3RunResponse,
) {
	t.Helper()
	scores := sendAndDecode[struct {
		Items []struct {
			Scorer           string  `json:"scorer"`
			Value            float64 `json:"value"`
			Rationale        string  `json:"rationale"`
			PromptTemplateID string  `json:"prompt_template_id"`
			JudgeModel       string  `json:"judge_model"`
		} `json:"items"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodGet, path: "/v1/runs/" + run.ID + "/scores", token: "admin-secret",
	}, http.StatusOK)
	if len(scores.Items) != 2 {
		t.Fatalf("M3 scores = %#v, want two", scores.Items)
	}
	for _, score := range scores.Items {
		assertM3Score(t, score.Value, score.Rationale, score.PromptTemplateID, score.JudgeModel)
		assertM3Aggregate(t, score.Scorer, run.Aggregates[score.Scorer])
	}
}

func assertM3Score(
	t *testing.T,
	value float64,
	rationale string,
	promptID string,
	judgeModel string,
) {
	t.Helper()
	if value != 1 || rationale == "" || promptID == "" || judgeModel != "fake-judge" {
		t.Fatalf(
			"M3 score value/rationale/prompt/model = %v/%q/%q/%q",
			value, rationale, promptID, judgeModel,
		)
	}
}

func assertM3Aggregate(
	t *testing.T,
	scorer string,
	aggregate struct {
		Mean     float64 `json:"mean"`
		PassRate float64 `json:"pass_rate"`
		N        int     `json:"n"`
	},
) {
	t.Helper()
	if aggregate.Mean != 1 || aggregate.PassRate != 1 || aggregate.N != 1 {
		t.Fatalf("M3 aggregate %s = %#v", scorer, aggregate)
	}
}

type apiRequest struct {
	method string
	path   string
	body   string
	token  string
}

type traceListResponse struct {
	Items []struct {
		ID        string `json:"id"`
		SpanCount int    `json:"span_count"`
	} `json:"items"`
}

type traceDetailResponse struct {
	Spans []struct {
		Name     string `json:"name"`
		Children []struct {
			Name string `json:"name"`
		} `json:"children"`
	} `json:"spans"`
}

func verifyM2Flow(t *testing.T, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	assertManagementRequiresAuth(t, client, baseURL)
	key := createM2Resources(t, client, baseURL)
	verifyTraceRoundTrip(t, client, baseURL, key)
}

func assertManagementRequiresAuth(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	response := sendAPIRequest(t, client, baseURL, apiRequest{
		method: http.MethodPost,
		path:   "/v1/projects",
		body:   `{"name":"Support"}`,
	})
	status := response.StatusCode
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close unauthenticated response: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated project status = %d, want 401", status)
	}
}

func createM2Resources(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	project := sendAndDecode[struct {
		ID string `json:"id"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost,
		path:   "/v1/projects",
		body:   `{"name":"Support"}`,
		token:  "admin-secret",
	}, http.StatusCreated)
	createdKey := sendAndDecode[struct {
		Key string `json:"key"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost,
		path:   "/v1/projects/" + project.ID + "/keys",
		body:   `{"name":"CI"}`,
		token:  "admin-secret",
	}, http.StatusCreated)
	if !strings.HasPrefix(createdKey.Key, "asy_") {
		t.Fatalf("created key = %q, want asy_ prefix", createdKey.Key)
	}
	application := sendAndDecode[struct {
		ID string `json:"id"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost,
		path:   "/v1/applications",
		body: `{"project_id":"` + project.ID + `","name":"Support Bot",` +
			`"slug":"support-bot"}`,
		token: "admin-secret",
	}, http.StatusCreated)
	if application.ID == "" {
		t.Fatal("created application ID is empty")
	}
	return createdKey.Key
}

func verifyTraceRoundTrip(t *testing.T, client *http.Client, baseURL string, key string) {
	t.Helper()
	response := sendAPIRequest(t, client, baseURL, apiRequest{
		method: http.MethodPost,
		path:   "/v1/traces",
		body:   otlpTraceJSON,
		token:  key,
	})
	status := response.StatusCode
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close OTLP response: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("OTLP ingest status = %d, want 200", status)
	}
	list := sendAndDecode[traceListResponse](t, client, baseURL, apiRequest{
		method: http.MethodGet,
		path:   "/v1/traces",
		token:  key,
	}, http.StatusOK)
	if len(list.Items) != 1 {
		t.Fatalf("trace list = %#v", list.Items)
	}
	if list.Items[0].SpanCount != 2 {
		t.Fatalf("trace span count = %d, want 2", list.Items[0].SpanCount)
	}
	detail := sendAndDecode[traceDetailResponse](t, client, baseURL, apiRequest{
		method: http.MethodGet,
		path:   "/v1/traces/" + list.Items[0].ID,
		token:  key,
	}, http.StatusOK)
	assertTraceTree(t, detail)
}

func assertTraceTree(t *testing.T, detail traceDetailResponse) {
	t.Helper()
	if len(detail.Spans) != 1 {
		t.Fatalf("trace roots = %#v, want one", detail.Spans)
	}
	if detail.Spans[0].Name != "answer" {
		t.Fatalf("root name = %q, want answer", detail.Spans[0].Name)
	}
	if len(detail.Spans[0].Children) != 1 {
		t.Fatalf("root children = %#v, want one", detail.Spans[0].Children)
	}
	if detail.Spans[0].Children[0].Name != "generation" {
		t.Fatalf("child name = %q, want generation", detail.Spans[0].Children[0].Name)
	}
}

const otlpTraceJSON = `{
  "resourceSpans": [{
    "resource": {"attributes": [
      {"key":"assay.application.slug","value":{"stringValue":"support-bot"}}
    ]},
    "scopeSpans": [{"spans": [
      {
        "traceId":"00112233445566778899aabbccddeeff",
        "spanId":"0102030405060708",
        "name":"answer",
        "startTimeUnixNano":"1787911200000000000",
        "endTimeUnixNano":"1787911202000000000"
      },
      {
        "traceId":"00112233445566778899aabbccddeeff",
        "spanId":"1112131415161718",
        "parentSpanId":"0102030405060708",
        "name":"generation",
        "startTimeUnixNano":"1787911200100000000",
        "endTimeUnixNano":"1787911201000000000",
        "attributes":[
          {"key":"gen_ai.operation.name","value":{"stringValue":"chat"}}
        ]
      }
    ]}]
  }]
}`

func sendAndDecode[T any](
	t *testing.T,
	client *http.Client,
	baseURL string,
	request apiRequest,
	wantStatus int,
) T {
	t.Helper()
	response := sendAPIRequest(t, client, baseURL, request)
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close %s %s response: %v", request.method, request.path, err)
		}
	}()
	if response.StatusCode != wantStatus {
		t.Fatalf(
			"%s %s status = %d, want %d",
			request.method,
			request.path,
			response.StatusCode,
			wantStatus,
		)
	}
	var output T
	if err := json.NewDecoder(response.Body).Decode(&output); err != nil {
		t.Fatalf("decode %s %s response: %v", request.method, request.path, err)
	}
	return output
}

func sendAPIRequest(
	t *testing.T,
	client *http.Client,
	baseURL string,
	spec apiRequest,
) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(),
		spec.method,
		baseURL+spec.path,
		strings.NewReader(spec.body),
	)
	if err != nil {
		t.Fatalf("create %s %s request: %v", spec.method, spec.path, err)
	}
	if spec.body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if spec.token != "" {
		request.Header.Set("Authorization", "Bearer "+spec.token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("send %s %s request: %v", spec.method, spec.path, err)
	}
	return response
}

func migrationVersion(t *testing.T, dsn string) int64 {
	t.Helper()
	connectionConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse migration verification DSN: %v", err)
	}
	database := stdlib.OpenDB(*connectionConfig)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close migration verification database: %v", err)
		}
	})
	return readMigrationVersion(t, database)
}

func readMigrationVersion(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	const versionQuery = `SELECT max(version_id) FROM goose_db_version WHERE is_applied`
	var version int64
	if err := database.QueryRowContext(t.Context(), versionQuery).Scan(&version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	return version
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve HTTP address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release HTTP address: %v", err)
	}
	return address
}

func waitUntilReady(t *testing.T, endpoint string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}

	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			t.Fatalf("create readiness request: %v", err)
		}
		response, err := client.Do(request)
		if err == nil {
			closeErr := response.Body.Close()
			if response.StatusCode == http.StatusOK && closeErr == nil {
				return
			}
		}

		select {
		case <-ctx.Done():
			t.Fatalf("wait for app readiness: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}
