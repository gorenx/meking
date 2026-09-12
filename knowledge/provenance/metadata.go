package provenance

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Evidence is one exact Corpus-owned TextUnit occurrence supporting a Source.
type Evidence struct {
	ZoneID     string
	TextUnitID string
	Source     EvidenceSource
}

type EvidenceSource interface {
	evidenceSource()
}

type CorporaSource struct {
	CorporaID string
}

func (CorporaSource) evidenceSource() {}

type MessageSource struct {
	MessageID string
}

func (MessageSource) evidenceSource() {}

type EntityMetadata struct {
	Frequency int
	Evidence  []Evidence
}

type RelationMetadata struct {
	Weight   float64
	Evidence []Evidence
}

type ClaimMetadata struct {
	SubjectText string
	ObjectText  string
	Status      string
	StartDate   string
	EndDate     string
	SourceText  string
	Evidence    []Evidence
}

func ValidateEntityMetadata(metadata EntityMetadata) error {
	if metadata.Frequency < 0 {
		return fmt.Errorf("%w: Entity Source Frequency must be non-negative", ErrInvalidSource)
	}
	return ValidateEvidence(metadata.Evidence)
}

func ValidateRelationMetadata(metadata RelationMetadata) error {
	if math.IsNaN(metadata.Weight) || math.IsInf(metadata.Weight, 0) || metadata.Weight < 0 {
		return fmt.Errorf("%w: Relation Source Weight must be finite and non-negative", ErrInvalidSource)
	}
	return ValidateEvidence(metadata.Evidence)
}

func ValidateClaimMetadata(metadata ClaimMetadata) error {
	return ValidateEvidence(metadata.Evidence)
}

// ValidateEvidence requires a canonical, duplicate-free list. An empty list is
// valid for Source kinds such as Resolution and Deletion that have no Corpus
// evidence; submission policies decide when evidence is required.
func ValidateEvidence(evidence []Evidence) error {
	for index, item := range evidence {
		if strings.TrimSpace(item.ZoneID) == "" ||
			strings.TrimSpace(item.TextUnitID) == "" {
			return fmt.Errorf("%w: Evidence ZoneID and TextUnitID are required", ErrInvalidSource)
		}
		switch source := item.Source.(type) {
		case CorporaSource:
			if strings.TrimSpace(source.CorporaID) == "" {
				return fmt.Errorf("%w: Evidence CorporaID is required", ErrInvalidSource)
			}
		case MessageSource:
			if strings.TrimSpace(source.MessageID) == "" {
				return fmt.Errorf("%w: Evidence MessageID is required", ErrInvalidSource)
			}
		default:
			return fmt.Errorf("%w: unsupported Evidence Source %T", ErrInvalidSource, item.Source)
		}
		if index > 0 && compareEvidence(evidence[index-1], item) >= 0 {
			return fmt.Errorf("%w: Evidence must be strictly sorted and deduplicated", ErrInvalidSource)
		}
	}
	return nil
}

func CanonicalEvidence(evidence []Evidence) []Evidence {
	canonical := append([]Evidence(nil), evidence...)
	slices.SortFunc(canonical, compareEvidence)
	return slices.Compact(canonical)
}

func compareEvidence(left, right Evidence) int {
	if order := cmp.Compare(left.ZoneID, right.ZoneID); order != 0 {
		return order
	}
	leftKind, leftID := evidenceSourceKey(left.Source)
	rightKind, rightID := evidenceSourceKey(right.Source)
	if order := cmp.Compare(leftKind, rightKind); order != 0 {
		return order
	}
	if order := cmp.Compare(leftID, rightID); order != 0 {
		return order
	}
	return cmp.Compare(left.TextUnitID, right.TextUnitID)
}

func evidenceSourceKey(source EvidenceSource) (string, string) {
	switch value := source.(type) {
	case CorporaSource:
		return "corpora", value.CorporaID
	case MessageSource:
		return "message", value.MessageID
	default:
		return "", ""
	}
}
