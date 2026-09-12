// Package activation accepts externally evaluated conversational recall and
// atomically records its effect on the current Zone's memory estimates.
package activation

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/mas"
)

type Observation struct {
	ID              string
	Subject         knowledge.ObjectRef
	Version         knowledge.Version
	OccurredAt      time.Time
	RecallActorID   string
	EvaluatorID     string
	ProtocolVersion string
	ProtocolDigest  string
	// Evidence preserves dialogue order; visibility is recorded per recall expression.
	Evidence   []Evidence
	Assessment Assessment
}

type Evidence struct {
	Ref         string
	SpeakerID   string
	CompletedAt time.Time
	Source      EvidenceSource
	// Recall is nil for background material, otherwise it records what the
	// observed actor could see while producing this expression.
	Recall *AvailableInformation
}

type AvailableInformation struct {
	Coverage    string
	ContextRefs []string
}

type EvidenceSource interface{ evidenceSource() }

type StoredMessage struct{ MessageID string }

func (StoredMessage) evidenceSource() {}

type Inline struct {
	HostRecordID string
	Role         string
	Text         string
}

func (Inline) evidenceSource() {}

type Assessment struct {
	Status       string
	Grade        *mas.Grade
	ReasonCode   string
	Rationale    string
	EvidenceRefs []string
}

// Normalize validates the complete input and copies mutable fields. It only
// normalizes time zones and reference sets, never dialogue order or original text.
func (input Observation) Normalize() (Observation, error) {
	invalid := func(message string) (Observation, error) {
		return Observation{}, fmt.Errorf("%w: %s", ErrInvalidObservation, message)
	}
	for _, value := range []string{input.ID, input.RecallActorID, input.EvaluatorID, input.ProtocolVersion, input.ProtocolDigest} {
		if !validText(value) {
			return invalid("identity and protocol fields are required")
		}
	}
	if err := ValidateSubject(input.Subject); err != nil {
		return Observation{}, err
	}
	if input.Version == 0 || !validTime(input.OccurredAt) {
		return invalid("positive version and occurrence time are required")
	}
	input.OccurredAt = input.OccurredAt.UTC()
	if len(input.Evidence) == 0 {
		return invalid("evidence is required")
	}
	input.Evidence = slices.Clone(input.Evidence)
	refs := make(map[string]int, len(input.Evidence))
	sources := make(map[string]bool, len(input.Evidence))
	var lastRecall time.Time
	complete := true
	for index, evidence := range input.Evidence {
		if !validText(evidence.Ref) || !validText(evidence.SpeakerID) {
			return invalid("evidence reference and speaker are required")
		}
		if _, exists := refs[evidence.Ref]; exists {
			return invalid("duplicate evidence reference")
		}
		refs[evidence.Ref] = index
		var sourceKey string
		switch source := evidence.Source.(type) {
		case StoredMessage:
			if !validText(source.MessageID) {
				return invalid("message identity is required")
			}
			sourceKey = "message:" + source.MessageID
		case Inline:
			if !validText(source.HostRecordID) || !validText(source.Role) || !validText(source.Text) {
				return invalid("inline identity, role and original text are required")
			}
			sourceKey = "inline:" + source.HostRecordID
		default:
			return invalid("evidence must have exactly one supported source")
		}
		if sources[sourceKey] {
			return invalid("duplicate evidence source")
		}
		sources[sourceKey] = true
		if !evidence.CompletedAt.IsZero() {
			if !validTime(evidence.CompletedAt) {
				return invalid("invalid evidence completion time")
			}
			evidence.CompletedAt = evidence.CompletedAt.UTC()
		}
		if evidence.Recall != nil {
			if evidence.SpeakerID != input.RecallActorID || !validTime(evidence.CompletedAt) {
				return invalid("recall expression requires the observed actor and completion time")
			}
			if !lastRecall.IsZero() && evidence.CompletedAt.Before(lastRecall) {
				return invalid("recall expressions must be chronological")
			}
			lastRecall = evidence.CompletedAt
			available := *evidence.Recall
			switch available.Coverage {
			case "complete":
			case "partial", "unknown":
				complete = false
			default:
				return invalid("invalid visibility coverage")
			}
			available.ContextRefs = referenceSet(available.ContextRefs)
			evidence.Recall = &available
		}
		input.Evidence[index] = evidence
	}
	for index, evidence := range input.Evidence {
		if evidence.Recall == nil {
			continue
		}
		for _, ref := range evidence.Recall.ContextRefs {
			position, exists := refs[ref]
			if !exists || position >= index {
				return invalid("visible context must reference preceding evidence")
			}
			at := input.Evidence[position].CompletedAt
			if !at.IsZero() && at.After(evidence.CompletedAt) {
				return invalid("visible context cannot be completed after recall")
			}
		}
	}
	assessment := input.Assessment
	if !validText(assessment.Rationale) {
		return invalid("assessment rationale is required")
	}
	assessment.EvidenceRefs = referenceSet(assessment.EvidenceRefs)
	if len(assessment.EvidenceRefs) == 0 {
		return invalid("assessment evidence references are required")
	}
	for _, ref := range assessment.EvidenceRefs {
		if _, exists := refs[ref]; !exists {
			return invalid("assessment references unknown evidence")
		}
	}
	switch assessment.Status {
	case "graded":
		if assessment.Grade == nil || assessment.ReasonCode != "" {
			return invalid("graded result requires only a grade")
		}
		grade, err := mas.NewGrade(assessment.Grade.Int())
		if err != nil {
			return invalid("invalid grade")
		}
		assessment.Grade = &grade
		if lastRecall.IsZero() || !complete {
			return invalid("graded result requires completed recall with complete visibility")
		}
	case "unscorable":
		if assessment.Grade != nil || !validReason(assessment.ReasonCode) {
			return invalid("unscorable result requires only a supported reason")
		}
	default:
		return invalid("invalid assessment status")
	}
	if !lastRecall.IsZero() && !input.OccurredAt.Equal(lastRecall) {
		return invalid("occurrence time must equal the last recall expression completion")
	}
	input.Assessment = assessment
	return input, nil
}

func ValidateSubject(subject knowledge.ObjectRef) error {
	var err error
	switch id := subject.(type) {
	case knowledge.EntityID:
		err = knowledge.ValidateEntityID(id)
	case knowledge.RelationID:
		err = knowledge.ValidateRelationID(id)
	case knowledge.ClaimID:
		err = knowledge.ValidateClaimID(id)
	default:
		return fmt.Errorf("%w: unsupported subject", ErrInvalidObservation)
	}
	if err != nil {
		return fmt.Errorf("%w: invalid subject: %v", ErrInvalidObservation, err)
	}
	return nil
}

func validText(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && !strings.ContainsRune(value, 0)
}
func validTime(at time.Time) bool {
	return !at.IsZero() && at.UTC().Year() >= 1 && at.UTC().Year() <= 9999
}
func referenceSet(values []string) []string {
	result := append([]string{}, values...)
	slices.Sort(result)
	return slices.Compact(result)
}
func validReason(reason string) bool {
	switch reason {
	case "no_recall", "target_mismatch", "insufficient_evidence", "answer_exposed", "exposure_unknown", "difficulty_unknown", "interrupted", "evaluation_failed":
		return true
	default:
		return false
	}
}
