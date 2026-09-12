package activation_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/mas"
	"github.com/memoria-space/meking/memory/activation"
)

func observation() activation.Observation {
	at := time.Date(2026, 9, 12, 12, 0, 0, 123456789, time.UTC)
	grade := mas.GradeGood
	return activation.Observation{
		ID: "natural-recall-1", Subject: knowledge.EntityID("11111111-1111-4111-8111-111111111111"), Version: 1,
		OccurredAt: at, RecallActorID: "actor", EvaluatorID: "evaluator", ProtocolVersion: "1", ProtocolDigest: "digest",
		Evidence: []activation.Evidence{
			{Ref: "context", SpeakerID: "other", Source: activation.StoredMessage{MessageID: "message-1"}},
			{Ref: "recall", SpeakerID: "actor", CompletedAt: at, Source: activation.Inline{HostRecordID: "host-2", Role: "assistant", Text: "  原始回忆文本\n"}, Recall: &activation.AvailableInformation{Coverage: "complete", ContextRefs: []string{"context"}}},
		},
		Assessment: activation.Assessment{Status: "graded", Grade: &grade, Rationale: "Natural recall with observable ordinary effort.", EvidenceRefs: []string{"recall", "context"}},
	}
}

func TestObservationRejectsInvalidShapes(t *testing.T) {
	cases := map[string]func(*activation.Observation){
		"empty id":             func(o *activation.Observation) { o.ID = " " },
		"invalid utf8":         func(o *activation.Observation) { o.EvaluatorID = string([]byte{0xff}) },
		"missing subject":      func(o *activation.Observation) { o.Subject = nil },
		"noncanonical subject": func(o *activation.Observation) { o.Subject = knowledge.EntityID("x") },
		"zero version":         func(o *activation.Observation) { o.Version = 0 },
		"zero occurrence":      func(o *activation.Observation) { o.OccurredAt = time.Time{} },
		"outside time range":   func(o *activation.Observation) { o.OccurredAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		"missing evidence":     func(o *activation.Observation) { o.Evidence = nil },
		"duplicate ref":        func(o *activation.Observation) { o.Evidence[1].Ref = "context" },
		"duplicate source":     func(o *activation.Observation) { o.Evidence[1].Source = o.Evidence[0].Source },
		"missing source":       func(o *activation.Observation) { o.Evidence[0].Source = nil },
		"blank inline": func(o *activation.Observation) {
			o.Evidence[1].Source = activation.Inline{HostRecordID: "h", Role: "user", Text: " "}
		},
		"wrong actor":                 func(o *activation.Observation) { o.Evidence[1].SpeakerID = "other" },
		"missing completion":          func(o *activation.Observation) { o.Evidence[1].CompletedAt = time.Time{} },
		"time mismatch":               func(o *activation.Observation) { o.OccurredAt = o.OccurredAt.Add(time.Second) },
		"unknown context":             func(o *activation.Observation) { o.Evidence[1].Recall.ContextRefs = []string{"missing"} },
		"self context":                func(o *activation.Observation) { o.Evidence[1].Recall.ContextRefs = []string{"recall"} },
		"future context":              func(o *activation.Observation) { o.Evidence[0].CompletedAt = o.OccurredAt.Add(time.Second) },
		"unknown coverage":            func(o *activation.Observation) { o.Evidence[1].Recall.Coverage = "yes" },
		"partial graded":              func(o *activation.Observation) { o.Evidence[1].Recall.Coverage = "partial" },
		"unknown graded":              func(o *activation.Observation) { o.Evidence[1].Recall.Coverage = "unknown" },
		"no recall graded":            func(o *activation.Observation) { o.Evidence[1].Recall = nil },
		"no rationale":                func(o *activation.Observation) { o.Assessment.Rationale = "" },
		"no assessment evidence":      func(o *activation.Observation) { o.Assessment.EvidenceRefs = nil },
		"unknown assessment evidence": func(o *activation.Observation) { o.Assessment.EvidenceRefs = []string{"missing"} },
		"unknown status":              func(o *activation.Observation) { o.Assessment.Status = "success" },
		"missing grade":               func(o *activation.Observation) { o.Assessment.Grade = nil },
		"invalid grade":               func(o *activation.Observation) { grade := mas.Grade(0); o.Assessment.Grade = &grade },
		"mixed result":                func(o *activation.Observation) { o.Assessment.ReasonCode = "no_recall" },
		"unscorable grade": func(o *activation.Observation) {
			o.Assessment.Status = "unscorable"
			o.Assessment.ReasonCode = "no_recall"
		},
		"unknown reason": func(o *activation.Observation) {
			o.Assessment.Status = "unscorable"
			o.Assessment.Grade = nil
			o.Assessment.ReasonCode = "guess"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := observation()
			mutate(&input)
			if _, err := input.Normalize(); !errors.Is(err, activation.ErrInvalidObservation) {
				t.Fatalf("Normalize error = %v", err)
			}
		})
	}
}

func TestNormalizeOnlyChangesEquivalentRepresentations(t *testing.T) {
	input := observation()
	want, err := input.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	input.OccurredAt = input.OccurredAt.In(time.FixedZone("offset", 8*3600))
	input.Evidence[1].CompletedAt = input.OccurredAt
	input.Evidence[1].Recall.ContextRefs = []string{"context", "context"}
	input.Assessment.EvidenceRefs = []string{"recall", "context", "recall"}
	got, err := input.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("equivalent input changed: %#v", got)
	}
	input.Evidence[1].Recall.ContextRefs[0] = "mutated"
	input.Assessment.EvidenceRefs[0] = "mutated"
	*input.Assessment.Grade = mas.GradeAgain
	if !reflect.DeepEqual(got, want) {
		t.Fatal("normalized result aliases mutable input")
	}
	if got.Evidence[1].Source.(activation.Inline).Text != "  原始回忆文本\n" {
		t.Fatal("original text changed")
	}
}

func TestUnscorableDoesNotRequireCompleteRecall(t *testing.T) {
	for _, reason := range []string{"no_recall", "target_mismatch", "insufficient_evidence", "answer_exposed", "exposure_unknown", "difficulty_unknown", "interrupted", "evaluation_failed"} {
		input := observation()
		input.Assessment.Status = "unscorable"
		input.Assessment.Grade = nil
		input.Assessment.ReasonCode = reason
		input.Evidence[1].Recall = nil
		if _, err := input.Normalize(); err != nil {
			t.Fatalf("%s: %v", reason, err)
		}
	}
}

func TestModelAndProtocolIdentities(t *testing.T) {
	config := mas.DefaultConfig()
	model, err := activation.NewModel(config)
	if err != nil {
		t.Fatal(err)
	}
	config.Parameters[0] += .01
	changed, err := activation.NewModel(config)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID() == model.ID() || model.Config() != mas.DefaultConfig() {
		t.Fatal("model configuration identity is not immutable")
	}
	base, err := activation.NewProtocol("1", "en", "Prompt", `{"type":"object"}`, `{"type":"object"}`)
	if err != nil {
		t.Fatal(err)
	}
	variants := [][5]string{
		{"1", "zh-CN", "Prompt", `{"type":"object"}`, `{"type":"object"}`},
		{"1", "en", "Other", `{"type":"object"}`, `{"type":"object"}`},
		{"1", "en", "Prompt", `{"type":"array"}`, `{"type":"object"}`},
		{"1", "en", "Prompt", `{"type":"object"}`, `{"type":"array"}`},
	}
	for _, fields := range variants {
		value, err := activation.NewProtocol(fields[0], fields[1], fields[2], fields[3], fields[4])
		if err != nil {
			t.Fatal(err)
		}
		if base.Digest() == value.Digest() {
			t.Fatal("protocol digest omitted a field")
		}
	}
	if _, err := activation.NewProtocol("1", "en", "Prompt", "null", "{}"); err == nil {
		t.Fatal("null schema accepted")
	}
}
