// Command openapi writes Assay's deterministic OpenAPI document.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/marioweid/assay/assayd/internal/api"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "openapi: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("openapi", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", "", "path to the generated OpenAPI document")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse arguments: %w", err)
	}
	if strings.TrimSpace(*output) == "" {
		return errors.New("output path must not be blank")
	}
	document, err := generateOpenAPI()
	if err != nil {
		return err
	}
	if err := os.WriteFile(*output, document, 0o644); err != nil {
		return fmt.Errorf("write OpenAPI document %q: %w", *output, err)
	}
	return nil
}

func generateOpenAPI() ([]byte, error) {
	router := http.NewServeMux()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	humaAPI := api.Register(router, api.Dependencies{Logger: logger})
	document, err := json.MarshalIndent(humaAPI.OpenAPI(), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAPI document: %w", err)
	}
	return append(document, '\n'), nil
}
