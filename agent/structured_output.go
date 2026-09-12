package agent

import "fmt"

// StructuredOutputMode identifies the OpenAI-compatible response protocol
// used for a structured consumer contract. JSON Schema asks the provider to
// enforce the schema; JSON Object asks only for valid JSON and relies on the
// existing consumer-side decoder to enforce required fields and types.
type StructuredOutputMode string

const (
	StructuredOutputJSONSchema StructuredOutputMode = "json_schema"
	StructuredOutputJSONObject StructuredOutputMode = "json_object"
)

// Validate rejects configuration that would silently weaken or disable the
// structured response contract.
func (mode StructuredOutputMode) Validate() error {
	switch mode {
	case StructuredOutputJSONSchema, StructuredOutputJSONObject:
		return nil
	default:
		return fmt.Errorf("unsupported structured output mode %q", mode)
	}
}
