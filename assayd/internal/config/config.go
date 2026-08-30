// Package config parses and validates Assay's process environment.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/caarlos0/env/v11"
)

const (
	encryptionKeyBytes = 32
	maxPostgresInteger = int64(1<<31 - 1)
)

// EncryptionKey is a decoded AES-256 key.
type EncryptionKey [encryptionKeyBytes]byte

// Config contains Assay's validated runtime configuration.
type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	AdminToken         string
	EncryptionKey      EncryptionKey
	JudgeBaseURL       string
	JudgeAPIKey        string
	JudgeModel         string
	WorkerConcurrency  int
	JobMaxAttempts     int
	TraceRetentionDays int
	AutoCreateApps     bool
	UIEnabled          bool
	LogFormat          string
}

type environmentConfig struct {
	HTTPAddr           string `env:"ASSAY_HTTP_ADDR"             envDefault:":8080"`
	DatabaseURL        string `env:"ASSAY_DATABASE_URL,required"`
	AdminToken         string `env:"ASSAY_ADMIN_TOKEN,required"`
	EncryptionKey      string `env:"ASSAY_ENCRYPTION_KEY,required"`
	JudgeBaseURL       string `env:"ASSAY_JUDGE_BASE_URL"`
	JudgeAPIKey        string `env:"ASSAY_JUDGE_API_KEY"`
	JudgeModel         string `env:"ASSAY_JUDGE_MODEL"`
	WorkerConcurrency  int    `env:"ASSAY_WORKER_CONCURRENCY"`
	JobMaxAttempts     int    `env:"ASSAY_JOB_MAX_ATTEMPTS"      envDefault:"3"`
	TraceRetentionDays int    `env:"ASSAY_TRACE_RETENTION_DAYS" envDefault:"0"`
	AutoCreateApps     bool   `env:"ASSAY_AUTO_CREATE_APPS"      envDefault:"false"`
	UIEnabled          bool   `env:"ASSAY_UI_ENABLED"            envDefault:"true"`
	LogFormat          string `env:"ASSAY_LOG_FORMAT"            envDefault:"json"`
}

// Load parses and validates configuration from the process environment.
func Load() (Config, error) {
	return parse(environmentMap(os.Environ()))
}

func parse(environment map[string]string) (Config, error) {
	environment = cloneEnvironment(environment)
	workerConcurrencySet := strings.TrimSpace(environment["ASSAY_WORKER_CONCURRENCY"]) != ""
	if !workerConcurrencySet {
		delete(environment, "ASSAY_WORKER_CONCURRENCY")
	}

	raw, err := env.ParseAsWithOptions[environmentConfig](env.Options{
		Environment: environment,
	})
	if err != nil {
		return Config{}, fmt.Errorf("parse configuration: %w", err)
	}

	if !workerConcurrencySet {
		raw.WorkerConcurrency = runtime.GOMAXPROCS(0)
	}
	return validate(raw)
}

func validate(raw environmentConfig) (Config, error) {
	if err := validateValues(raw); err != nil {
		return Config{}, err
	}
	if err := validateLogFormat(raw.LogFormat); err != nil {
		return Config{}, err
	}

	key, err := decodeEncryptionKey(raw.EncryptionKey)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddr:           raw.HTTPAddr,
		DatabaseURL:        raw.DatabaseURL,
		AdminToken:         raw.AdminToken,
		EncryptionKey:      key,
		JudgeBaseURL:       raw.JudgeBaseURL,
		JudgeAPIKey:        raw.JudgeAPIKey,
		JudgeModel:         raw.JudgeModel,
		WorkerConcurrency:  raw.WorkerConcurrency,
		JobMaxAttempts:     raw.JobMaxAttempts,
		TraceRetentionDays: raw.TraceRetentionDays,
		AutoCreateApps:     raw.AutoCreateApps,
		UIEnabled:          raw.UIEnabled,
		LogFormat:          raw.LogFormat,
	}, nil
}

func validateValues(raw environmentConfig) error {
	if strings.TrimSpace(raw.HTTPAddr) == "" {
		return validationError("ASSAY_HTTP_ADDR must not be empty")
	}
	if strings.TrimSpace(raw.DatabaseURL) == "" {
		return validationError("ASSAY_DATABASE_URL must not be empty")
	}
	if strings.TrimSpace(raw.AdminToken) == "" {
		return validationError("ASSAY_ADMIN_TOKEN must not be empty")
	}
	if raw.WorkerConcurrency <= 0 {
		return validationError("ASSAY_WORKER_CONCURRENCY must be greater than zero")
	}
	if raw.JobMaxAttempts <= 0 {
		return validationError("ASSAY_JOB_MAX_ATTEMPTS must be greater than zero")
	}
	if int64(raw.JobMaxAttempts) > maxPostgresInteger {
		return validationError("ASSAY_JOB_MAX_ATTEMPTS exceeds the database integer limit")
	}
	if raw.TraceRetentionDays < 0 {
		return validationError("ASSAY_TRACE_RETENTION_DAYS must not be negative")
	}
	return nil
}

func validateLogFormat(format string) error {
	switch format {
	case "json", "text":
		return nil
	default:
		return validationError("ASSAY_LOG_FORMAT must be json or text")
	}
}

func decodeEncryptionKey(value string) (EncryptionKey, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return EncryptionKey{}, validationError("ASSAY_ENCRYPTION_KEY must be valid base64")
	}
	if len(decoded) != encryptionKeyBytes {
		return EncryptionKey{}, validationError(
			"ASSAY_ENCRYPTION_KEY must decode to exactly 32 bytes",
		)
	}

	var key EncryptionKey
	copy(key[:], decoded)
	return key, nil
}

func validationError(message string) error {
	return fmt.Errorf("validate configuration: %s", message)
}

func environmentMap(values []string) map[string]string {
	environment := make(map[string]string, len(values))
	for _, value := range values {
		key, item, found := strings.Cut(value, "=")
		if found {
			environment[key] = item
		}
	}
	return environment
}

func cloneEnvironment(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
