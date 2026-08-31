// Package target calls application endpoints for generated evaluation runs.
package target

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/marioweid/assay/assayd/internal/domain"
)

// Render expands configured string leaves without changing other JSON value types.
func Render(
	endpoint domain.ResolvedTargetEndpoint,
	item domain.DatasetItem,
) (map[string]string, map[string]any, error) {
	data := map[string]any{
		"item": map[string]any{
			"input": item.Input, "expected_output": item.ExpectedOutput,
			"metadata": item.Metadata, "external_id": item.ExternalID,
		},
		"secret": endpoint.Secret,
	}
	headers := make(map[string]string, len(endpoint.Headers))
	for name, value := range endpoint.Headers {
		rendered, err := renderString("header "+name, value, data)
		if err != nil {
			return nil, nil, err
		}
		headers[name] = rendered
	}
	rendered, err := renderValue("request", endpoint.RequestTemplate, data)
	if err != nil {
		return nil, nil, err
	}
	body, ok := rendered.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf(
			"render target request: %w: root must be an object", domain.ErrInvalid,
		)
	}
	return headers, body, nil
}

func renderValue(name string, value any, data map[string]any) (any, error) {
	switch typed := value.(type) {
	case string:
		return renderString(name, typed, data)
	case map[string]any:
		return renderMap(name, typed, data)
	case []any:
		return renderSlice(name, typed, data)
	default:
		return typed, nil
	}
}

func renderMap(name string, value map[string]any, data map[string]any) (map[string]any, error) {
	result := make(map[string]any, len(value))
	for key, item := range value {
		rendered, err := renderValue(name+"."+key, item, data)
		if err != nil {
			return nil, err
		}
		result[key] = rendered
	}
	return result, nil
}

func renderSlice(name string, value []any, data map[string]any) ([]any, error) {
	result := make([]any, 0, len(value))
	for index, item := range value {
		rendered, err := renderValue(fmt.Sprintf("%s[%d]", name, index), item, data)
		if err != nil {
			return nil, err
		}
		result = append(result, rendered)
	}
	return result, nil
}

func renderString(name string, value string, data map[string]any) (string, error) {
	parsed, err := template.New(name).Option("missingkey=error").Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse target template %s: %w", name, err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return "", fmt.Errorf("execute target template %s: %w", name, err)
	}
	return output.String(), nil
}
