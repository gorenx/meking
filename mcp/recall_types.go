package mcp

import (
	"fmt"
	"time"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/mas"
	"github.com/memoria-space/meking/memory/activation"
)

// SubmitRecallObservationRequest is assembled by the Host, not the evaluator.
// The Host binds a judgment to the observation fixed for that evaluation call.
type SubmitRecallObservationRequest struct {
	UserID      string                   `json:"user_id"`
	SessionID   string                   `json:"session_id"`
	Observation RecallObservationRequest `json:"observation"`
	Assessment  RecallAssessmentResult   `json:"assessment"`
}

type RecallObservationRequest struct {
	ObservationID   string           `json:"observation_id"`
	Target          RecallTarget     `json:"target"`
	OccurredAt      time.Time        `json:"occurred_at"`
	RecallActorID   string           `json:"recall_actor_id"`
	EvaluatorID     string           `json:"evaluator_id"`
	ProtocolVersion string           `json:"protocol_version"`
	ProtocolDigest  string           `json:"protocol_digest"`
	Evidence        []RecallEvidence `json:"evidence"`
}

type RecallTarget struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version uint64 `json:"version"`
}

type RecallEvidence struct {
	Ref           string               `json:"ref"`
	SpeakerID     string               `json:"speaker_id"`
	CompletedAt   *time.Time           `json:"completed_at,omitempty"`
	StoredMessage *RecallStoredMessage `json:"stored_message,omitempty"`
	Inline        *RecallInline        `json:"inline,omitempty"`
	Recall        *RecallVisibility    `json:"recall,omitempty"`
}

type RecallStoredMessage struct {
	MessageID string `json:"message_id"`
}
type RecallInline struct {
	HostRecordID string `json:"host_record_id"`
	Role         string `json:"role"`
	Text         string `json:"text"`
}
type RecallVisibility struct {
	Coverage    string   `json:"coverage"`
	ContextRefs []string `json:"context_refs"`
}

// RecallAssessmentResult is the complete model output. EvidenceRefs may select
// existing material references; identity and protocol fields belong to the Host.
type RecallAssessmentResult struct {
	Status       string   `json:"status"`
	Grade        *string  `json:"grade,omitempty"`
	ReasonCode   *string  `json:"reason_code,omitempty"`
	Rationale    string   `json:"rationale"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type RecallReceipt struct {
	ObservationID   string    `json:"observation_id"`
	Outcome         string    `json:"outcome"`
	RecordedAt      time.Time `json:"recorded_at"`
	ProtocolDigest  string    `json:"protocol_digest"`
	ModelID         string    `json:"model_id"`
	AppliedSequence uint64    `json:"applied_sequence,omitempty"`
	Duplicate       bool      `json:"duplicate"`
}

func recallValue(request SubmitRecallObservationRequest) (activation.Observation, error) {
	value, assessment := request.Observation, request.Assessment
	invalid := func(message string) (activation.Observation, error) {
		return activation.Observation{}, fmt.Errorf("%w: %s", activation.ErrInvalidObservation, message)
	}
	var subject knowledge.ObjectRef
	switch value.Target.Kind {
	case "entity":
		subject = knowledge.EntityID(value.Target.ID)
	case "relation":
		subject = knowledge.RelationID(value.Target.ID)
	case "claim":
		subject = knowledge.ClaimID(value.Target.ID)
	default:
		return invalid("unsupported target kind")
	}
	input := activation.Observation{
		ID:              value.ObservationID,
		Subject:         subject,
		Version:         knowledge.Version(value.Target.Version),
		OccurredAt:      value.OccurredAt,
		RecallActorID:   value.RecallActorID,
		EvaluatorID:     value.EvaluatorID,
		ProtocolVersion: value.ProtocolVersion,
		ProtocolDigest:  value.ProtocolDigest,
		Evidence:        make([]activation.Evidence, len(value.Evidence)),
		Assessment: activation.Assessment{
			Status:       assessment.Status,
			Rationale:    assessment.Rationale,
			EvidenceRefs: assessment.EvidenceRefs,
		},
	}
	if assessment.Grade != nil {
		var grade mas.Grade
		switch *assessment.Grade {
		case "Again":
			grade = mas.GradeAgain
		case "Hard":
			grade = mas.GradeHard
		case "Good":
			grade = mas.GradeGood
		case "Easy":
			grade = mas.GradeEasy
		default:
			return invalid("invalid grade")
		}
		input.Assessment.Grade = &grade
	}
	if assessment.ReasonCode != nil {
		if *assessment.ReasonCode == "" {
			return invalid("empty reason code")
		}
		input.Assessment.ReasonCode = *assessment.ReasonCode
	}
	for index, item := range value.Evidence {
		evidence := activation.Evidence{Ref: item.Ref, SpeakerID: item.SpeakerID}
		if item.CompletedAt != nil {
			evidence.CompletedAt = *item.CompletedAt
		}
		if item.Recall != nil {
			evidence.Recall = &activation.AvailableInformation{
				Coverage:    item.Recall.Coverage,
				ContextRefs: item.Recall.ContextRefs,
			}
		}
		switch {
		case item.StoredMessage != nil && item.Inline == nil:
			evidence.Source = activation.StoredMessage{MessageID: item.StoredMessage.MessageID}
		case item.StoredMessage == nil && item.Inline != nil:
			evidence.Source = activation.Inline{
				HostRecordID: item.Inline.HostRecordID,
				Role:         item.Inline.Role,
				Text:         item.Inline.Text,
			}
		default:
			return invalid("evidence requires exactly one source")
		}
		input.Evidence[index] = evidence
	}
	return input.Normalize()
}

func recallReceipt(receipt activation.Receipt) RecallReceipt {
	return RecallReceipt{
		ObservationID:   receipt.ObservationID,
		Outcome:         receipt.Outcome,
		RecordedAt:      receipt.RecordedAt,
		ProtocolDigest:  receipt.ProtocolDigest,
		ModelID:         receipt.ModelID,
		AppliedSequence: receipt.AppliedSequence,
		Duplicate:       receipt.Duplicate,
	}
}
