package api

import (
	"encoding/json"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestPendingRunSerializesEmptyAggregatesAsObject(t *testing.T) {
	payload, err := json.Marshal(evalRunOutput(domain.EvalRun{Status: "pending"}))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatal(err)
	}
	if string(body["aggregates"]) != "{}" {
		t.Fatalf("pending aggregates = %s, want an empty object", body["aggregates"])
	}
}
