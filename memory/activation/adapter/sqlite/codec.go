package sqlite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/memory/activation"
)

type observationRecord struct {
	ID              string
	SubjectKind     string
	SubjectID       string
	Version         knowledge.Version
	OccurredAt      time.Time
	RecallActorID   string
	EvaluatorID     string
	ProtocolVersion string
	ProtocolDigest  string
	Evidence        []evidenceRecord
	Assessment      activation.Assessment
}

type evidenceRecord struct {
	Ref           string
	SpeakerID     string
	CompletedAt   time.Time
	StoredMessage *activation.StoredMessage
	Inline        *activation.Inline
	Recall        *activation.AvailableInformation
}

func subjectKind(subject knowledge.ObjectRef) string {
	switch subject.(type) {
	case knowledge.EntityID:
		return "entity"
	case knowledge.RelationID:
		return "relation"
	case knowledge.ClaimID:
		return "claim"
	default:
		return ""
	}
}

func restoreSubject(kind, id string) (knowledge.ObjectRef, error) {
	var subject knowledge.ObjectRef
	switch kind {
	case "entity":
		subject = knowledge.EntityID(id)
	case "relation":
		subject = knowledge.RelationID(id)
	case "claim":
		subject = knowledge.ClaimID(id)
	default:
		return nil, activation.ErrDataIntegrity
	}
	if err := activation.ValidateSubject(subject); err != nil {
		return nil, fmt.Errorf("%w: %v", activation.ErrDataIntegrity, err)
	}
	return subject, nil
}

func encodeInput(input activation.Observation) ([]byte, error) {
	value := observationRecord{ID: input.ID, SubjectKind: subjectKind(input.Subject), SubjectID: input.Subject.ObjectID(), Version: input.Version,
		OccurredAt: input.OccurredAt, RecallActorID: input.RecallActorID, EvaluatorID: input.EvaluatorID, ProtocolVersion: input.ProtocolVersion,
		ProtocolDigest: input.ProtocolDigest, Assessment: input.Assessment, Evidence: make([]evidenceRecord, len(input.Evidence))}
	for index, item := range input.Evidence {
		evidence := evidenceRecord{Ref: item.Ref, SpeakerID: item.SpeakerID, CompletedAt: item.CompletedAt, Recall: item.Recall}
		switch source := item.Source.(type) {
		case activation.StoredMessage:
			evidence.StoredMessage = &source
		case activation.Inline:
			evidence.Inline = &source
		default:
			return nil, activation.ErrDataIntegrity
		}
		value.Evidence[index] = evidence
	}
	return json.Marshal(value)
}

func decodeInput(encoded string) (activation.Observation, error) {
	var value observationRecord
	if err := decode(encoded, &value); err != nil {
		return activation.Observation{}, err
	}
	subject, err := restoreSubject(value.SubjectKind, value.SubjectID)
	if err != nil {
		return activation.Observation{}, err
	}
	input := activation.Observation{ID: value.ID, Subject: subject, Version: value.Version, OccurredAt: value.OccurredAt,
		RecallActorID: value.RecallActorID, EvaluatorID: value.EvaluatorID, ProtocolVersion: value.ProtocolVersion, ProtocolDigest: value.ProtocolDigest,
		Assessment: value.Assessment, Evidence: make([]activation.Evidence, len(value.Evidence))}
	for index, item := range value.Evidence {
		evidence := activation.Evidence{Ref: item.Ref, SpeakerID: item.SpeakerID, CompletedAt: item.CompletedAt, Recall: item.Recall}
		switch {
		case item.StoredMessage != nil && item.Inline == nil:
			evidence.Source = *item.StoredMessage
		case item.StoredMessage == nil && item.Inline != nil:
			evidence.Source = *item.Inline
		default:
			return activation.Observation{}, activation.ErrDataIntegrity
		}
		input.Evidence[index] = evidence
	}
	return input, nil
}

func decode(encoded string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: decode record: %v", activation.ErrDataIntegrity, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("%w: trailing record content", activation.ErrDataIntegrity)
	}
	return nil
}
