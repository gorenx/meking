package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
)

// Pin the Host submission and judgment-only output contracts independently of
// descriptions. The observation contract remains unchanged by the output split.
func TestRecallSchemaCompatibility(t *testing.T) {
	protocols, err := RecallProtocols("中文评估提示词", "English evaluation prompt")
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(recallSubmissionSchema)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		schema string
		digest string
	}{
		{
			name:   "submission",
			schema: compact.String(),
			digest: "2004990e7e3d175309892ef51d19eed781c5c28c4f81cb31baba216b881669ae",
		},
		{
			name:   "observation",
			schema: protocols[0].InputSchema(),
			digest: "1a0562a68a832f3cde3d041918427259e2e92dfdc67574481498ecadfa73564d",
		},
		{
			name:   "assessment",
			schema: protocols[0].OutputSchema(),
			digest: "2004990e7e3d175309892ef51d19eed781c5c28c4f81cb31baba216b881669ae",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var schema any
			if err := json.Unmarshal([]byte(test.schema), &schema); err != nil {
				t.Fatal(err)
			}
			removeRecallDescriptions(t, schema)
			raw, err := json.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != test.digest {
				t.Fatalf("schema validation contract changed: got %s, want %s", got, test.digest)
			}
		})
	}
	if protocols[0].InputSchema() != protocols[1].InputSchema() || protocols[0].OutputSchema() != protocols[1].OutputSchema() {
		t.Fatal("bilingual protocols must share schemas")
	}
}

func removeRecallDescriptions(t *testing.T, value any) {
	t.Helper()
	switch current := value.(type) {
	case map[string]any:
		if properties, ok := current["properties"].(map[string]any); ok {
			for name, field := range properties {
				schema, ok := field.(map[string]any)
				if !ok {
					t.Fatalf("field %s has no schema", name)
				}
				if description, ok := schema["description"].(string); schema["const"] == nil && (!ok || description == "") {
					t.Errorf("field %s has no description", name)
				}
			}
		}
		delete(current, "description")
		for _, child := range current {
			removeRecallDescriptions(t, child)
		}
	case []any:
		for _, child := range current {
			removeRecallDescriptions(t, child)
		}
	}
}

func TestRecallEvaluationOutputIsJudgmentOnly(t *testing.T) {
	protocols, err := RecallProtocols("评估提示词", "Evaluation prompt")
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range protocols {
		var schema struct {
			Type                 string                     `json:"type"`
			Properties           map[string]json.RawMessage `json:"properties"`
			AdditionalProperties *bool                      `json:"additionalProperties"`
			OneOf                []json.RawMessage          `json:"oneOf"`
		}
		if err := json.Unmarshal([]byte(protocol.OutputSchema()), &schema); err != nil {
			t.Fatal(err)
		}
		if schema.Type != "object" || schema.AdditionalProperties == nil || *schema.AdditionalProperties {
			t.Fatal("evaluation output must be a closed object")
		}
		if len(schema.OneOf) != 2 || len(schema.Properties) != 5 {
			t.Fatal("evaluation output must expose five fields and two exclusive result branches")
		}
		for _, field := range []string{"status", "grade", "reason_code", "rationale", "evidence_refs"} {
			if _, found := schema.Properties[field]; !found {
				t.Errorf("missing top-level field %s", field)
			}
		}
	}
}
