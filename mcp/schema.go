package mcp

import (
	"encoding/json"

	protocol "github.com/mark3labs/mcp-go/mcp"
)

func inputSchema[T any]() protocol.ToolOption {
	generated := protocol.WithInputSchema[T]()
	return func(tool *protocol.Tool) {
		generated(tool)
		if len(tool.RawInputSchema) == 0 {
			return
		}
		normalized, err := normalizeSchemaTypes(tool.RawInputSchema)
		if err == nil {
			tool.RawInputSchema = normalized
		}
	}
}

func outputSchema[T any]() protocol.ToolOption {
	generated := protocol.WithOutputSchema[T]()
	return func(tool *protocol.Tool) {
		success, ok := generatedOutputSchema[T]()
		if !ok {
			generated(tool)
			return
		}
		failure, ok := generatedOutputSchema[ToolFailure]()
		if !ok {
			generated(tool)
			return
		}
		raw, err := json.Marshal(map[string]any{
			"type":  "object",
			"anyOf": []json.RawMessage{success, failure},
		})
		if err != nil {
			generated(tool)
			return
		}
		tool.OutputSchema = protocol.ToolOutputSchema{}
		tool.RawOutputSchema = raw
	}
}

func generatedOutputSchema[T any]() (json.RawMessage, bool) {
	var tool protocol.Tool
	protocol.WithOutputSchema[T]()(&tool)
	if tool.OutputSchema.Type == "" {
		return nil, false
	}
	raw, err := json.Marshal(tool.OutputSchema)
	if err != nil {
		return nil, false
	}
	normalized, err := normalizeSchemaTypes(raw)
	if err != nil {
		return nil, false
	}
	return normalized, true
}

func normalizeSchemaTypes(raw json.RawMessage) (json.RawMessage, error) {
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	normalizeSchemaType(schema)
	return json.Marshal(schema)
}

func normalizeSchemaType(value any) {
	switch current := value.(type) {
	case map[string]any:
		if types, ok := current["type"].([]any); ok {
			branches := make([]any, 0, len(types))
			for _, item := range types {
				branches = append(branches, map[string]any{"type": item})
			}
			delete(current, "type")
			if _, exists := current["anyOf"]; exists {
				constraint := map[string]any{"anyOf": branches}
				if allOf, ok := current["allOf"].([]any); ok {
					current["allOf"] = append(allOf, constraint)
				} else {
					current["allOf"] = []any{constraint}
				}
			} else {
				current["anyOf"] = branches
			}
		}
		for _, child := range current {
			normalizeSchemaType(child)
		}
	case []any:
		for _, child := range current {
			normalizeSchemaType(child)
		}
	}
}
