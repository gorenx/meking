package assembly_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/mas"
	mekingmcp "github.com/memoria-space/meking/mcp"
	"github.com/memoria-space/meking/memory"
	"github.com/memoria-space/meking/memory/activation"
	"github.com/memoria-space/meking/zone"
)

func recallRequest(t *testing.T, fixture mcpFixture) mekingmcp.SubmitRecallObservationRequest {
	t.Helper()
	sessionID := string(fixture.sessionID)
	callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", memoryRequest(sessionID, "recall-knowledge", "original-message", "Alice is an engineer."))
	matched := callTool[mekingmcp.MemoryMatches](t, fixture.client, "search_memory", mekingmcp.SearchMemoryRequest{UserID: mcpTestUserID, SessionID: sessionID, Query: memory.Query{Title: "ALICE", Type: "PERSON"}})
	if len(matched.Entities) != 1 {
		t.Fatalf("entities: %#v", matched.Entities)
	}
	entity := matched.Entities[0].Version
	current := readResource[mekingmcp.EntityResource](t, fixture.client, fmt.Sprintf("meking://users/%s/sessions/%s/knowledge/entities/%s", mcpTestUserID, sessionID, entity.Content.ID))
	policy := readResource[mekingmcp.RecallProtocolResource](t, fixture.client, "meking://recall/protocols/1/en")
	at := time.Date(2026, 9, 10, 12, 0, 0, 123456789, time.UTC)
	grade := "Good"
	return mekingmcp.SubmitRecallObservationRequest{UserID: mcpTestUserID, SessionID: sessionID,
		Observation: mekingmcp.RecallObservationRequest{ObservationID: "natural-recall", Target: mekingmcp.RecallTarget{Kind: "entity", ID: string(entity.Content.ID), Version: current.Version.Number}, OccurredAt: at, RecallActorID: "actor", EvaluatorID: "external-agent", ProtocolVersion: policy.Version, ProtocolDigest: policy.Digest,
			Evidence: []mekingmcp.RecallEvidence{{Ref: "expression", SpeakerID: "actor", CompletedAt: &at, Inline: &mekingmcp.RecallInline{HostRecordID: "host-1", Role: "assistant", Text: "I recall Alice works as an engineer."}, Recall: &mekingmcp.RecallVisibility{Coverage: "complete", ContextRefs: []string{}}}}},
		Assessment: mekingmcp.RecallAssessmentResult{Status: "graded", Grade: &grade, Rationale: "Fixture assessment supplied by the external caller.", EvidenceRefs: []string{"expression"}}}
}

func TestActivationMCPToSharedDatabase(t *testing.T) {
	f := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	request := recallRequest(t, f)
	resources, err := f.client.ListResources(t.Context(), protocol.ListResourcesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources.Resources) != 2 {
		t.Fatalf("protocol resources: %#v", resources.Resources)
	}
	var digests []string
	for _, language := range []string{"zh-CN", "en"} {
		resource := readResource[mekingmcp.RecallProtocolResource](t, f.client, "meking://recall/protocols/1/"+language)
		fromTool := callTool[mekingmcp.RecallProtocolResource](t, f.client, "get_recall_evaluation_protocol", mekingmcp.RecallProtocolRequest{Language: language})
		if fromTool.Prompt == "" || fromTool.Prompt != resource.Prompt || fromTool.Digest != resource.Digest || string(fromTool.InputSchema) != string(resource.InputSchema) || string(fromTool.OutputSchema) != string(resource.OutputSchema) {
			t.Fatalf("evaluation tool differs from protocol resource: %#v", fromTool)
		}
		examples := strings.Split(fromTool.Prompt, "```json\n")
		if len(examples) != 3 {
			t.Fatal("evaluation instructions must provide both JSON result examples")
		}
		for _, block := range examples[1:] {
			raw := strings.SplitN(block, "```", 2)[0]
			var example json.RawMessage
			if err := json.Unmarshal([]byte(raw), &example); err != nil {
				t.Fatal(err)
			}
			// The example passes argument validation but cannot write without Host binding.
			assertToolFailure(t, rawToolCall(t, f.client, "submit_recall_observation", example), "invalid_observation")
		}
		if resource.Language != language || !json.Valid(resource.InputSchema) || !json.Valid(resource.OutputSchema) {
			t.Fatalf("protocol resource: %#v", resource)
		}
		archived, err := activation.NewProtocol(resource.Version, resource.Language, resource.Prompt, string(resource.InputSchema), string(resource.OutputSchema))
		if err != nil {
			t.Fatal(err)
		}
		if archived.Digest() != resource.Digest {
			t.Fatal("published protocol digest does not match exact contents")
		}
		digests = append(digests, resource.Digest)
	}
	if digests[0] == digests[1] {
		t.Fatal("bilingual protocols have the same identity")
	}
	receipt := callRecall(t, f, request)
	if receipt.Outcome != "applied" || receipt.AppliedSequence != 1 || receipt.Duplicate {
		t.Fatalf("receipt: %#v", receipt)
	}
	ctx, err := zone.NewContext(t.Context(), f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	state, found, err := f.service.Observations().ReadState(ctx, knowledge.EntityID(request.Observation.Target.ID))
	if err != nil || !found {
		t.Fatalf("state: %v %v", found, err)
	}
	scheduler, _ := mas.NewScheduler(mas.DefaultConfig())
	want, err := scheduler.UpdateLiveness(mas.State{}, mas.GradeGood, request.Observation.OccurredAt)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != want || state.Sequence != 1 {
		t.Fatalf("incorrect persisted calculation: %#v", state)
	}
	retry := callRecall(t, f, request)
	if !retry.Duplicate {
		t.Fatal("not duplicate")
	}
	retry.Duplicate = false
	if retry != receipt {
		t.Fatalf("retry receipt changed: %#v", retry)
	}
	request.Assessment.Rationale = "changed"
	assertToolFailure(t, rawRecall(t, f, request), "observation_conflict")
	request.Observation.ObservationID = "unscorable"
	request.Assessment.Status = "unscorable"
	request.Assessment.Grade = nil
	reason := "difficulty_unknown"
	request.Assessment.ReasonCode = &reason
	unscorable := callRecall(t, f, request)
	if unscorable.Outcome != "recorded_unscorable" || unscorable.AppliedSequence != 0 {
		t.Fatalf("unscorable: %#v", unscorable)
	}
	after, _, err := f.service.Observations().ReadState(ctx, knowledge.EntityID(request.Observation.Target.ID))
	if err != nil {
		t.Fatal(err)
	}
	if after != state {
		t.Fatal("unscorable updated state")
	}
}

func TestActivationMCPRejectsInvalidShapeAndReferences(t *testing.T) {
	f := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	request := recallRequest(t, f)
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"mixed-result", func(value map[string]any) {
			value["assessment"].(map[string]any)["reason_code"] = "no_recall"
		}},
		{"null-grade", func(value map[string]any) {
			value["assessment"].(map[string]any)["grade"] = nil
		}},
		{"assessment-identity", func(value map[string]any) {
			value["assessment"].(map[string]any)["observation_id"] = "invented"
		}},
		{"assessment-protocol", func(value map[string]any) {
			value["assessment"].(map[string]any)["protocol_digest"] = "invented"
		}},
		{"old-assessment-envelope", func(value map[string]any) {
			value["assessment"] = map[string]any{
				"observation_id":   request.Observation.ObservationID,
				"protocol_version": request.Observation.ProtocolVersion,
				"protocol_digest":  request.Observation.ProtocolDigest,
				"result":           value["assessment"],
			}
		}},
		{"unknown-field", func(value map[string]any) { value["observation"].(map[string]any)["session_id"] = "invented" }},
		{"double-source", func(value map[string]any) {
			value["observation"].(map[string]any)["evidence"].([]any)[0].(map[string]any)["stored_message"] = map[string]any{"message_id": "original-message"}
		}},
		{"missing-time", func(value map[string]any) {
			delete(value["observation"].(map[string]any)["evidence"].([]any)[0].(map[string]any), "completed_at")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(encoded, &value); err != nil {
				t.Fatal(err)
			}
			test.mutate(value)
			binding, err := json.Marshal(map[string]any{
				"user_id":     value["user_id"],
				"session_id":  value["session_id"],
				"observation": value["observation"],
			})
			if err != nil {
				t.Fatal(err)
			}
			call := protocol.CallToolRequest{Params: protocol.CallToolParams{
				Name:      "submit_recall_observation",
				Arguments: value["assessment"],
				Meta:      &protocol.Meta{AdditionalFields: map[string]any{"meking/recall": string(binding)}},
			}}
			result, err := f.client.CallTool(t.Context(), call)
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatal("invalid shape accepted")
			}
		})
	}
	wrong := request
	wrong.Assessment.EvidenceRefs = []string{"invented-reference"}
	assertToolFailure(t, rawRecall(t, f, wrong), "invalid_observation")
	wrong = request
	wrong.Observation.Target.Version++
	assertToolFailure(t, rawRecall(t, f, wrong), "version_changed")
	wrong = request
	wrong.Observation.Evidence = append([]mekingmcp.RecallEvidence{}, request.Observation.Evidence...)
	wrong.Observation.Evidence[0].Inline = nil
	wrong.Observation.Evidence[0].StoredMessage = &mekingmcp.RecallStoredMessage{MessageID: "missing"}
	assertToolFailure(t, rawRecall(t, f, wrong), "message_not_found")
	wrong.Observation.Evidence[0].StoredMessage.MessageID = "original-message"
	callRecall(t, f, wrong)
	wrong.UserID = "different-user"
	assertToolFailure(t, rawRecall(t, f, wrong), "forbidden")
	ctx, err := zone.NewContext(t.Context(), f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	state, found, err := f.service.Observations().ReadState(ctx, knowledge.EntityID(request.Observation.Target.ID))
	if err != nil || !found || state.Sequence != 1 {
		t.Fatalf("invalid calls changed state: %#v %v %v", state, found, err)
	}
	if strings.Contains(fmt.Sprint(state), "Session") {
		t.Fatal("unexpected Session state")
	}
}

func rawRecall(t *testing.T, f mcpFixture, input mekingmcp.SubmitRecallObservationRequest) *protocol.CallToolResult {
	t.Helper()
	request, err := mekingmcp.NewRecallSubmission(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.client.CallTool(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func callRecall(t *testing.T, f mcpFixture, input mekingmcp.SubmitRecallObservationRequest) mekingmcp.RecallReceipt {
	t.Helper()
	result := rawRecall(t, f, input)
	if result.IsError {
		t.Fatalf("recall submission failed: %#v", result)
	}
	return structured[mekingmcp.RecallReceipt](t, result.StructuredContent)
}

func TestRecallToolArgumentsExcludeHostBinding(t *testing.T) {
	f := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	input := recallRequest(t, f)
	list, err := f.client.ListTools(t.Context(), protocol.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range list.Tools {
		if tool.Name != "submit_recall_observation" {
			continue
		}
		found = true
		encoded, err := json.Marshal(tool)
		if err != nil {
			t.Fatal(err)
		}
		var wire struct {
			Input json.RawMessage `json:"inputSchema"`
		}
		if err := json.Unmarshal(encoded, &wire); err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Type       string                     `json:"type"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(wire.Input, &schema); err != nil {
			t.Fatal(err)
		}
		if schema.Type != "object" {
			t.Fatalf("MCP inputSchema.type = %q, want object", schema.Type)
		}
		if len(schema.Properties) != 5 {
			t.Fatalf("tools/list hides evaluation fields: %#v", schema.Properties)
		}
		for _, field := range []string{"status", "grade", "reason_code", "rationale", "evidence_refs"} {
			if _, found := schema.Properties[field]; !found {
				t.Errorf("tools/list missing %s", field)
			}
		}
		for _, forbidden := range []string{"user_id", "session_id", "observation_id", "target", "occurred_at", "protocol_digest", "evaluator_id", "speaker_id", "host_record_id", "message_id"} {
			if strings.Contains(string(wire.Input), `"`+forbidden+`"`) {
				t.Errorf("tools/list exposes Host field %s", forbidden)
			}
		}
	}
	if !found {
		t.Fatal("recall tool missing")
	}
	// Valid judgments cannot execute without an independent Host binding.
	assertToolFailure(t, rawToolCall(t, f.client, "submit_recall_observation", input.Assessment), "invalid_observation")
	// The old full envelope is not an accepted model argument shape.
	if result := rawToolCall(t, f.client, "submit_recall_observation", input); !result.IsError {
		t.Fatal("full Host envelope accepted as tool arguments")
	}
	for _, binding := range []any{"", "{}", "{", "null", "{} {}", map[string]any{"observation_id": "invented"}} {
		request, err := mekingmcp.NewRecallSubmission(input)
		if err != nil {
			t.Fatal(err)
		}
		request.Params.Meta.AdditionalFields["meking/recall"] = binding
		result, err := f.client.CallTool(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		assertToolFailure(t, result, "invalid_observation")
	}
	ctx, err := zone.NewContext(t.Context(), f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists, err := f.service.Observations().ReadState(ctx, knowledge.EntityID(input.Observation.Target.ID)); err != nil || exists {
		t.Fatalf("unbound submission wrote state: %v %v", exists, err)
	}
	callRecall(t, f, input)
}
