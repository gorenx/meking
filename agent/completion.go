package agent

import "encoding/json"

// CompletionRole identifies a participant in a provider completion request.
type CompletionRole string

const (
	CompletionRoleSystem    CompletionRole = "system"
	CompletionRoleUser      CompletionRole = "user"
	CompletionRoleAssistant CompletionRole = "assistant"
)

// CompletionMessage is one ordered provider conversation turn.
type CompletionMessage struct {
	Role    CompletionRole
	Content string
}

// CompletionResponseFormat selects the provider response representation needed
// by an adapter mapping a consumer-owned model port.
type CompletionResponseFormat string

const (
	CompletionResponseText       CompletionResponseFormat = ""
	CompletionResponseJSONObject CompletionResponseFormat = "json_object"
	CompletionResponseJSONSchema CompletionResponseFormat = "json_schema"
)

// JSONSchema is the structured result contract supplied by the calling
// domain. Agent transports it without interpreting the domain fields.
type JSONSchema struct {
	Name       string
	Definition json.RawMessage
}

// CompletionRequest is the Agent invocation DTO. Domain adapters map
// their own requests into this provider capability at the outbound boundary.
type CompletionRequest struct {
	Messages            []CompletionMessage
	ResponseFormat      CompletionResponseFormat
	JSONSchema          *JSONSchema
	MaxCompletionTokens int
}

type CompletionResponse struct {
	Content string
}

type CompletionDeltaHandler func(delta string) error

// CompletionStreamResponse retains the exact emitted prefix.
type CompletionStreamResponse struct {
	Content string
}
