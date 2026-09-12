package mcp

import (
	"encoding/json"
	"math"
	"testing"

	protocol "github.com/mark3labs/mcp-go/mcp"
)

func TestRecallBindingSurvivesWireRoundTripWithoutModelIdentity(t *testing.T) {
	grade := "Good"
	input := SubmitRecallObservationRequest{
		UserID:    "user",
		SessionID: "zone",
		Observation: RecallObservationRequest{
			ObservationID: "fixed",
			Target:        RecallTarget{Kind: "entity", ID: "target", Version: math.MaxUint64},
			Evidence:      []RecallEvidence{{Ref: "expression", Inline: &RecallInline{Text: "original"}}},
		},
		Assessment: RecallAssessmentResult{Status: "graded", Grade: &grade, Rationale: "evidence", EvidenceRefs: []string{"expression"}},
	}
	call, err := NewRecallSubmission(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Observation.Evidence[0].Inline.Text = "changed after binding"
	input.Assessment.EvidenceRefs[0] = "changed after binding"
	raw, err := json.Marshal(call)
	if err != nil {
		t.Fatal(err)
	}
	var decoded protocol.CallToolRequest
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	var assessment RecallAssessmentResult
	if err := decoded.BindArguments(&assessment); err != nil {
		t.Fatal(err)
	}
	restored, err := recallSubmission(decoded, assessment)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Observation.Target.Version != math.MaxUint64 {
		t.Fatal("binding lost version precision")
	}
	if restored.Observation.ObservationID != "fixed" || restored.Observation.Evidence[0].Inline.Text != "original" || restored.Assessment.EvidenceRefs[0] != "expression" {
		t.Fatal("caller mutation changed the bound request")
	}
}
