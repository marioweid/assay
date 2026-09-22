// Command acceptance-fixtures serves deterministic judge and target responses for acceptance tests.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	fixtureQuestion          = "What is Assay?"
	fixtureFailingQuestion   = "What is Assay incorrectly?"
	fixtureTransientQuestion = "fixture-transient: What is Assay?"
	fixtureTerminalQuestion  = "fixture-terminal: fail generation"
	fixturePassingAnswer     = "Assay evaluates AI systems."
	fixtureFailingAnswer     = "Assay is a database."
	fixtureReference         = "Assay evaluates AI systems."
	fixtureContext           = "Assay evaluates AI systems."
	maxRequestBytes          = 1 << 20
)

type fixtureHandler struct {
	mux       *http.ServeMux
	mutex     sync.Mutex
	transient map[[sha256.Size]byte]bool
}

type judgeEnvelope struct {
	Messages       []judgeMessage `json:"messages"`
	ResponseFormat struct {
		Type string `json:"type"`
	} `json:"response_format"`
}

type judgeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		return runHealthcheck()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr: ":18090", Handler: newFixtureHandler(), ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "acceptance fixtures: %v\n", err)
		return 1
	}
	return 0
}

func newFixtureHandler() http.Handler {
	handler := &fixtureHandler{mux: http.NewServeMux(), transient: make(map[[sha256.Size]byte]bool)}
	handler.mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(writer, "ok\n")
	})
	handler.mux.HandleFunc("POST /v1/chat/completions", handler.judge)
	handler.mux.HandleFunc("POST /answer", handler.answer)
	return handler
}

func (h *fixtureHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	h.mux.ServeHTTP(writer, request)
}

func (h *fixtureHandler) judge(writer http.ResponseWriter, request *http.Request) {
	payload, err := readBody(request)
	if err != nil {
		http.Error(writer, "invalid synthetic judge request", http.StatusBadRequest)
		return
	}
	if bytesContain(payload, fixtureTerminalQuestion) {
		http.Error(writer, "synthetic terminal failure", http.StatusUnprocessableEntity)
		return
	}
	if h.failTransient(writer, payload) {
		return
	}
	input, err := decodeJudgeInput(payload)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	content, found := judgeContent(input)
	if !found {
		http.Error(writer, "unknown synthetic judge input", http.StatusUnprocessableEntity)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"choices": []any{map[string]any{"message": map[string]string{"content": content}}},
		"usage":   map[string]int{"total_tokens": 5},
	})
}

func decodeJudgeInput(payload []byte) (map[string]json.RawMessage, error) {
	var envelope judgeEnvelope
	if json.Unmarshal(payload, &envelope) != nil || envelope.ResponseFormat.Type != "json_object" {
		return nil, errors.New("invalid synthetic judge contract")
	}
	if len(envelope.Messages) < 2 || envelope.Messages[1].Role != "user" {
		return nil, errors.New("invalid synthetic judge contract")
	}
	var input map[string]json.RawMessage
	if json.Unmarshal([]byte(envelope.Messages[1].Content), &input) != nil {
		return nil, errors.New("invalid synthetic judge input")
	}
	return input, nil
}

func judgeContent(input map[string]json.RawMessage) (string, bool) {
	if raw, found := input["answer"]; found {
		return extractionContent(raw)
	}
	if _, found := input["context_chunks"]; found {
		return verificationContent(input)
	}
	if raw, found := input["generated"]; found {
		return correctnessContent(input, raw)
	}
	return "", false
}

func extractionContent(raw json.RawMessage) (string, bool) {
	var answer string
	if json.Unmarshal(raw, &answer) != nil {
		return "", false
	}
	switch answer {
	case fixturePassingAnswer, fixtureFailingAnswer:
		content, _ := json.Marshal(map[string]any{
			"claims":   []map[string]string{{"id": "c1", "text": answer}},
			"excluded": []any{},
		})
		return string(content), true
	default:
		return "", false
	}
}

func verificationContent(input map[string]json.RawMessage) (string, bool) {
	var claims []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if json.Unmarshal(input["claims"], &claims) != nil || len(claims) != 1 || claims[0].ID != "c1" {
		return "", false
	}
	verdict := "unsupported"
	reason := "Synthetic context does not support the claim."
	evidence := []string{}
	supportsClaim := claims[0].Text == fixturePassingAnswer &&
		strings.Contains(string(input["context_chunks"]), fixtureContext)
	if supportsClaim {
		verdict = "supported"
		reason = "Synthetic context directly supports the claim."
		evidence = []string{"k0"}
	} else if claims[0].Text != fixtureFailingAnswer {
		return "", false
	}
	content, _ := json.Marshal(map[string]any{"verdicts": []any{map[string]any{
		"id": "c1", "verdict": verdict, "supporting_chunk_ids": evidence, "reason": reason,
	}}})
	return string(content), true
}

func correctnessContent(input map[string]json.RawMessage, raw json.RawMessage) (string, bool) {
	var generated, reference string
	generatedErr := json.Unmarshal(raw, &generated)
	referenceErr := json.Unmarshal(input["reference"], &reference)
	if generatedErr != nil || referenceErr != nil || reference != fixtureReference {
		return "", false
	}
	status, reasoning, score := "contradicted", "Synthetic answer contradicts the reference.", 0
	contradictions := []string{generated}
	if generated == fixturePassingAnswer {
		status, reasoning, score, contradictions = "correct", "Synthetic answer matches.", 1, []string{}
	} else if generated != fixtureFailingAnswer {
		return "", false
	}
	content, _ := json.Marshal(map[string]any{
		"reference_facts": []any{map[string]string{"fact": fixtureReference, "status": status}},
		"contradictions":  contradictions, "reasoning": reasoning, "score": score,
	})
	return string(content), true
}

func (h *fixtureHandler) answer(writer http.ResponseWriter, request *http.Request) {
	payload, err := readBody(request)
	if err != nil {
		http.Error(writer, "invalid synthetic target request", http.StatusBadRequest)
		return
	}
	if h.failTransient(writer, payload) {
		return
	}
	var input struct {
		Query string `json:"query"`
	}
	if json.Unmarshal(payload, &input) != nil {
		http.Error(writer, "invalid synthetic target request", http.StatusBadRequest)
		return
	}
	switch input.Query {
	case fixtureQuestion, fixtureTransientQuestion:
		writeJSON(writer, http.StatusOK, targetResponse(fixturePassingAnswer))
	case fixtureFailingQuestion:
		writeJSON(writer, http.StatusOK, targetResponse(fixtureFailingAnswer))
	case fixtureTerminalQuestion:
		http.Error(writer, "synthetic terminal failure", http.StatusUnprocessableEntity)
	default:
		http.Error(writer, "unknown synthetic target input", http.StatusUnprocessableEntity)
	}
}

func targetResponse(answer string) map[string]any {
	return map[string]any{
		"answer":  answer,
		"sources": []map[string]string{{"id": "k0", "text": fixtureContext}},
	}
}

func (h *fixtureHandler) failTransient(writer http.ResponseWriter, payload []byte) bool {
	if !bytesContain(payload, fixtureTransientQuestion) {
		return false
	}
	hash := sha256.Sum256(payload)
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if h.transient[hash] {
		return false
	}
	h.transient[hash] = true
	http.Error(writer, "synthetic transient failure", http.StatusServiceUnavailable)
	return true
}

func bytesContain(payload []byte, value string) bool {
	return strings.Contains(string(payload), value)
}

func readBody(request *http.Request) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(request.Body, maxRequestBytes+1))
	if err != nil || len(payload) > maxRequestBytes {
		return nil, errors.New("request body is invalid")
	}
	return payload, nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func runHealthcheck() int {
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://127.0.0.1:18090/healthz")
	if err != nil {
		return 1
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
