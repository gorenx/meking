package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/memoria-space/meking/memory/activation"
)

const recallBindingKey = "meking/recall"

type recallBinding struct {
	UserID      string                   `json:"user_id"`
	SessionID   string                   `json:"session_id"`
	Observation RecallObservationRequest `json:"observation"`
}

// NewRecallSubmission is the Host-side binding step, never a model tool.
// It snapshots the fixed observation outside arguments. A JSON string preserves
// exact version integers through MCP metadata decoders that use float64.
func NewRecallSubmission(request SubmitRecallObservationRequest) (protocol.CallToolRequest, error) {
	binding, err := json.Marshal(recallBinding{
		UserID:      request.UserID,
		SessionID:   request.SessionID,
		Observation: request.Observation,
	})
	if err != nil {
		return protocol.CallToolRequest{}, fmt.Errorf("encode recall binding: %w", err)
	}
	assessment, err := json.Marshal(request.Assessment)
	if err != nil {
		return protocol.CallToolRequest{}, fmt.Errorf("encode recall assessment: %w", err)
	}
	return protocol.CallToolRequest{Params: protocol.CallToolParams{
		Name:      "submit_recall_observation",
		Arguments: json.RawMessage(assessment),
		Meta:      &protocol.Meta{AdditionalFields: map[string]any{recallBindingKey: string(binding)}},
	}}, nil
}

func recallSubmission(call protocol.CallToolRequest, assessment RecallAssessmentResult) (SubmitRecallObservationRequest, error) {
	invalid := func() (SubmitRecallObservationRequest, error) {
		return SubmitRecallObservationRequest{}, fmt.Errorf("%w: valid Host recall binding is required", activation.ErrInvalidObservation)
	}
	if call.Params.Meta == nil {
		return invalid()
	}
	raw, ok := call.Params.Meta.AdditionalFields[recallBindingKey].(string)
	if !ok || raw == "" {
		return invalid()
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var binding recallBinding
	if err := decoder.Decode(&binding); err != nil {
		return invalid()
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return invalid()
	}
	if binding.UserID == "" || binding.SessionID == "" {
		return invalid()
	}
	return SubmitRecallObservationRequest{
		UserID:      binding.UserID,
		SessionID:   binding.SessionID,
		Observation: binding.Observation,
		Assessment:  assessment,
	}, nil
}
