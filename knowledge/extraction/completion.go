package extraction

import (
	"context"
	"encoding/json"
)

// CompletionRole identifies one participant in an extraction conversation.
type CompletionRole string

const (
	CompletionRoleSystem    CompletionRole = "system"
	CompletionRoleUser      CompletionRole = "user"
	CompletionRoleAssistant CompletionRole = "assistant"
)

// CompletionMessage is one ordered extraction conversation turn.
type CompletionMessage struct {
	Role    CompletionRole
	Content string
}

// CompletionRequest contains only the model controls used by extraction.
type CompletionRequest struct {
	Messages            []CompletionMessage
	SchemaName          string
	Schema              json.RawMessage
	MaxCompletionTokens int
}

// CompletionResponse is the selected assistant text.
type CompletionResponse struct {
	Content string
}

// CompletionModel is the extraction-owned model port.
type CompletionModel interface {
	Complete(context.Context, CompletionRequest) (CompletionResponse, error)
}
