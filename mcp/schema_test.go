package mcp

import (
	"encoding/json"
	"testing"

	protocol "github.com/mark3labs/mcp-go/mcp"
)

func TestSchemaUsesSingleTypeBranches(t *testing.T) {
	type collection struct {
		Items []string `json:"items"`
	}
	type document struct {
		Collection *collection `json:"collection"`
	}

	tool := protocol.NewTool(
		"schema_test",
		inputSchema[document](),
		outputSchema[document](),
	)
	raw, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal Tool: %v", err)
	}
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal Tool: %v", err)
	}
	assertSingleSchemaTypes(t, schema)
	if countNullableSchemaTypes(schema, "array") != 2 {
		t.Fatalf("nullable array branches were not preserved: %s", raw)
	}
	if countNullableSchemaTypes(schema, "object") != 2 {
		t.Fatalf("nullable object branches were not preserved: %s", raw)
	}
}

func assertSingleSchemaTypes(t *testing.T, value any) {
	t.Helper()
	switch current := value.(type) {
	case map[string]any:
		if _, ok := current["type"].([]any); ok {
			t.Fatalf("schema contains an array-valued type: %#v", current["type"])
		}
		for _, child := range current {
			assertSingleSchemaTypes(t, child)
		}
	case []any:
		for _, child := range current {
			assertSingleSchemaTypes(t, child)
		}
	}
}

func countNullableSchemaTypes(value any, expected string) int {
	count := 0
	switch current := value.(type) {
	case map[string]any:
		if alternatives, ok := current["anyOf"].([]any); ok {
			types := make(map[string]bool, len(alternatives))
			for _, alternative := range alternatives {
				branch, ok := alternative.(map[string]any)
				if !ok {
					continue
				}
				kind, ok := branch["type"].(string)
				if ok {
					types[kind] = true
				}
			}
			if types["null"] && types[expected] {
				count++
			}
		}
		for _, child := range current {
			count += countNullableSchemaTypes(child, expected)
		}
	case []any:
		for _, child := range current {
			count += countNullableSchemaTypes(child, expected)
		}
	}
	return count
}
