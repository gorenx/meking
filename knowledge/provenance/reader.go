package provenance

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/memoria-space/meking/knowledge"
)

// EvidenceReader reads the source evidence for one exact formal Knowledge Version.
type EvidenceReader interface {
	// ReadEntityEvidence reads every source record for the exact Entity reference.
	ReadEntityEvidence(ctx context.Context, reference knowledge.Reference[knowledge.EntityID]) ([]EntityConfirmation, error)
	// ReadRelationEvidence reads every source record for the exact Relation reference.
	ReadRelationEvidence(ctx context.Context, reference knowledge.Reference[knowledge.RelationID]) ([]RelationConfirmation, error)
	// ReadClaimEvidence reads every source record for the exact Claim reference.
	ReadClaimEvidence(ctx context.Context, reference knowledge.Reference[knowledge.ClaimID]) ([]ClaimConfirmation, error)
}

// Reader combines every Source supporting one formal Knowledge Version. It
// leaves the immutable formal content unchanged and returns detached metadata.
type Reader struct {
	evidence EvidenceReader
}

// ReadEntityEvidence reads the unmerged source evidence for one formal Entity Version.
func (reader *Reader) ReadEntityEvidence(
	ctx context.Context,
	reference knowledge.Reference[knowledge.EntityID],
) ([]EntityConfirmation, error) {
	if reader == nil || reader.evidence == nil {
		return nil, errors.New("read Entity Evidence: Reader is not initialized")
	}
	return reader.evidence.ReadEntityEvidence(ctx, reference)
}

// ReadRelationEvidence reads the unmerged source evidence for one formal Relation Version.
func (reader *Reader) ReadRelationEvidence(
	ctx context.Context,
	reference knowledge.Reference[knowledge.RelationID],
) ([]RelationConfirmation, error) {
	if reader == nil || reader.evidence == nil {
		return nil, errors.New("read Relation Evidence: Reader is not initialized")
	}
	return reader.evidence.ReadRelationEvidence(ctx, reference)
}

// ReadClaimEvidence reads the unmerged source evidence for one formal Claim Version.
func (reader *Reader) ReadClaimEvidence(
	ctx context.Context,
	reference knowledge.Reference[knowledge.ClaimID],
) ([]ClaimConfirmation, error) {
	if reader == nil || reader.evidence == nil {
		return nil, errors.New("read Claim Evidence: Reader is not initialized")
	}
	return reader.evidence.ReadClaimEvidence(ctx, reference)
}

func NewReader(evidence EvidenceReader) (*Reader, error) {
	if evidence == nil {
		return nil, errors.New("create Knowledge provenance Reader: formal Sources are required")
	}
	return &Reader{
		evidence: evidence,
	}, nil
}

func (reader *Reader) Entity(
	ctx context.Context,
	reference knowledge.Reference[knowledge.EntityID],
) (EntityMetadata, error) {
	if reader == nil || reader.evidence == nil {
		return EntityMetadata{}, errors.New("read Entity provenance: Reader is not initialized")
	}
	confirmations, err := reader.evidence.ReadEntityEvidence(ctx, reference)
	if err != nil {
		return EntityMetadata{}, err
	}
	result := EntityMetadata{}
	for _, confirmation := range confirmations {
		if confirmation.EntityID != reference.ID || confirmation.Version != reference.Version {
			return EntityMetadata{}, fmt.Errorf("%w: Entity Confirmation differs from requested Version", knowledge.ErrDataIntegrity)
		}
		if confirmation.Metadata.Frequency > math.MaxInt-result.Frequency {
			return EntityMetadata{}, fmt.Errorf("%w: Entity Frequency overflows int", knowledge.ErrDataIntegrity)
		}
		result.Frequency += confirmation.Metadata.Frequency
		result.Evidence = append(result.Evidence, confirmation.Metadata.Evidence...)
	}
	result.Evidence = CanonicalEvidence(result.Evidence)
	return result, ValidateEntityMetadata(result)
}

func (reader *Reader) Relation(
	ctx context.Context,
	reference knowledge.Reference[knowledge.RelationID],
) (RelationMetadata, error) {
	if reader == nil || reader.evidence == nil {
		return RelationMetadata{}, errors.New("read Relation provenance: Reader is not initialized")
	}
	confirmations, err := reader.evidence.ReadRelationEvidence(ctx, reference)
	if err != nil {
		return RelationMetadata{}, err
	}
	result := RelationMetadata{}
	for _, confirmation := range confirmations {
		if confirmation.RelationID != reference.ID || confirmation.Version != reference.Version {
			return RelationMetadata{}, fmt.Errorf("%w: Relation Confirmation differs from requested Version", knowledge.ErrDataIntegrity)
		}
		result.Weight += confirmation.Metadata.Weight
		result.Evidence = append(result.Evidence, confirmation.Metadata.Evidence...)
	}
	result.Evidence = CanonicalEvidence(result.Evidence)
	return result, ValidateRelationMetadata(result)
}

func (reader *Reader) Claim(
	ctx context.Context,
	reference knowledge.Reference[knowledge.ClaimID],
) ([]ClaimMetadata, error) {
	if reader == nil || reader.evidence == nil {
		return nil, errors.New("read Claim provenance: Reader is not initialized")
	}
	confirmations, err := reader.evidence.ReadClaimEvidence(ctx, reference)
	if err != nil {
		return nil, err
	}
	result := make([]ClaimMetadata, 0)
	for _, confirmation := range confirmations {
		if confirmation.ClaimID != reference.ID || confirmation.Version != reference.Version {
			return nil, fmt.Errorf("%w: Claim Confirmation differs from requested Version", knowledge.ErrDataIntegrity)
		}
		for _, metadata := range confirmation.Metadata {
			metadata.Evidence = CanonicalEvidence(metadata.Evidence)
			if err := ValidateClaimMetadata(metadata); err != nil {
				return nil, err
			}
			result = append(result, metadata)
		}
	}
	return result, nil
}
