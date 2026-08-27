package config

import (
	"runtime"
	"strings"
	"testing"
)

func TestParseUsesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := parse(validEnvironment())
	if err != nil {
		t.Fatalf("parse valid environment: %v", err)
	}

	want := Config{
		HTTPAddr:           ":8080",
		DatabaseURL:        "postgres://assay:assay@localhost/assay",
		AdminToken:         "admin-token",
		WorkerConcurrency:  runtime.GOMAXPROCS(0),
		JobMaxAttempts:     3,
		TraceRetentionDays: 0,
		UIEnabled:          true,
		LogFormat:          "json",
	}
	if cfg != want {
		t.Errorf("Config = %+v, want %+v", cfg, want)
	}
}

func TestParseReadsExplicitValues(t *testing.T) {
	t.Parallel()

	environment := validEnvironment()
	environment["ASSAY_HTTP_ADDR"] = "127.0.0.1:9000"
	environment["ASSAY_JUDGE_BASE_URL"] = "http://judge.test/v1"
	environment["ASSAY_JUDGE_API_KEY"] = "judge-secret"
	environment["ASSAY_JUDGE_MODEL"] = "judge-model"
	environment["ASSAY_WORKER_CONCURRENCY"] = "4"
	environment["ASSAY_JOB_MAX_ATTEMPTS"] = "5"
	environment["ASSAY_TRACE_RETENTION_DAYS"] = "30"
	environment["ASSAY_AUTO_CREATE_APPS"] = "true"
	environment["ASSAY_UI_ENABLED"] = "false"
	environment["ASSAY_LOG_FORMAT"] = "text"

	cfg, err := parse(environment)
	if err != nil {
		t.Fatalf("parse explicit environment: %v", err)
	}

	want := Config{
		HTTPAddr:           "127.0.0.1:9000",
		DatabaseURL:        "postgres://assay:assay@localhost/assay",
		AdminToken:         "admin-token",
		JudgeBaseURL:       "http://judge.test/v1",
		JudgeAPIKey:        "judge-secret",
		JudgeModel:         "judge-model",
		WorkerConcurrency:  4,
		JobMaxAttempts:     5,
		TraceRetentionDays: 30,
		AutoCreateApps:     true,
		UIEnabled:          false,
		LogFormat:          "text",
	}
	if cfg != want {
		t.Errorf("Config = %+v, want %+v", cfg, want)
	}
}

func TestParseRejectsMissingRequiredValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{name: "database URL", key: "ASSAY_DATABASE_URL"},
		{name: "admin token", key: "ASSAY_ADMIN_TOKEN"},
		{name: "encryption key", key: "ASSAY_ENCRYPTION_KEY"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := validEnvironment()
			delete(environment, test.key)
			_, err := parse(environment)
			if err == nil {
				t.Fatalf("parse without %s succeeded", test.key)
			}
			if !strings.Contains(err.Error(), "parse configuration") {
				t.Errorf("error = %q, want parse configuration context", err)
			}
		})
	}
}

func TestParseRejectsInvalidEncryptionKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "malformed base64", value: "not-base64"},
		{name: "too short", value: "c2hvcnQ="},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := validEnvironment()
			environment["ASSAY_ENCRYPTION_KEY"] = test.value
			_, err := parse(environment)
			if err == nil {
				t.Fatal("parse invalid encryption key succeeded")
			}
			if strings.Contains(err.Error(), test.value) {
				t.Errorf("error leaked encryption key: %q", err)
			}
		})
	}
}

func TestParseRejectsInvalidSemanticValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "blank HTTP address", key: "ASSAY_HTTP_ADDR", value: " "},
		{name: "zero concurrency", key: "ASSAY_WORKER_CONCURRENCY", value: "0"},
		{name: "negative concurrency", key: "ASSAY_WORKER_CONCURRENCY", value: "-1"},
		{name: "zero attempts", key: "ASSAY_JOB_MAX_ATTEMPTS", value: "0"},
		{name: "negative retention", key: "ASSAY_TRACE_RETENTION_DAYS", value: "-1"},
		{name: "unknown log format", key: "ASSAY_LOG_FORMAT", value: "xml"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := validEnvironment()
			environment[test.key] = test.value
			_, err := parse(environment)
			if err == nil {
				t.Fatalf("parse %s=%q succeeded", test.key, test.value)
			}
			if !strings.Contains(err.Error(), "validate configuration") {
				t.Errorf("error = %q, want validation context", err)
			}
		})
	}
}

func validEnvironment() map[string]string {
	return map[string]string{
		"ASSAY_DATABASE_URL":   "postgres://assay:assay@localhost/assay",
		"ASSAY_ADMIN_TOKEN":    "admin-token",
		"ASSAY_ENCRYPTION_KEY": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	}
}
