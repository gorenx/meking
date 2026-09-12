// Package candidate owns unresolved Knowledge versions. A Candidate is
// identified by the stable Knowledge ID, the formal Base Version, and the
// canonical content Hash; it has no independent Conflict ID.
package candidate

import (
	"fmt"

	"github.com/memoria-space/meking/knowledge"
)

var emptyHash knowledge.Hash

type Key[ID knowledge.KnowledgeID] struct {
	ID          ID
	BaseVersion knowledge.Version
	ContentHash knowledge.Hash
}

type Entity struct {
	Key     Key[knowledge.EntityID]
	Number  uint64
	Content knowledge.Entity
}

type Relation struct {
	Key     Key[knowledge.RelationID]
	Number  uint64
	Content knowledge.Relation
}

type Claim struct {
	Key     Key[knowledge.ClaimID]
	Number  uint64
	Content knowledge.Claim
}

func ValidateEntity(entity Entity) error {
	if entity.Number == 0 {
		return fmt.Errorf("%w: Entity Candidate Number must be positive", knowledge.ErrInvalidChange)
	}
	if err := ValidateEntityKey(entity.Key); err != nil {
		return err
	}
	if entity.Content.ID != entity.Key.ID {
		return fmt.Errorf("%w: Entity Candidate identity does not match its key", knowledge.ErrInvalidChange)
	}
	hash, err := knowledge.HashEntity(entity.Content)
	if err != nil {
		return err
	}
	if hash != entity.Key.ContentHash {
		return fmt.Errorf("%w: Entity Candidate Hash does not match its content", knowledge.ErrInvalidChange)
	}
	return nil
}

func ValidateRelation(relation Relation) error {
	if relation.Number == 0 {
		return fmt.Errorf("%w: Relation Candidate Number must be positive", knowledge.ErrInvalidChange)
	}
	if err := ValidateRelationKey(relation.Key); err != nil {
		return err
	}
	if relation.Content.ID != relation.Key.ID {
		return fmt.Errorf("%w: Relation Candidate identity does not match its key", knowledge.ErrInvalidChange)
	}
	hash, err := knowledge.HashRelation(relation.Content)
	if err != nil {
		return err
	}
	if hash != relation.Key.ContentHash {
		return fmt.Errorf("%w: Relation Candidate Hash does not match its content", knowledge.ErrInvalidChange)
	}
	return nil
}

func ValidateClaim(claim Claim) error {
	if claim.Number == 0 {
		return fmt.Errorf("%w: Claim Candidate Number must be positive", knowledge.ErrInvalidChange)
	}
	if err := ValidateClaimKey(claim.Key); err != nil {
		return err
	}
	if claim.Content.ID != claim.Key.ID {
		return fmt.Errorf("%w: Claim Candidate identity does not match its key", knowledge.ErrInvalidChange)
	}
	hash, err := knowledge.HashClaim(claim.Content)
	if err != nil {
		return err
	}
	if hash != claim.Key.ContentHash {
		return fmt.Errorf("%w: Claim Candidate Hash does not match its content", knowledge.ErrInvalidChange)
	}
	return nil
}

func ValidateEntityKey(key Key[knowledge.EntityID]) error {
	if err := knowledge.ValidateEntityID(key.ID); err != nil {
		return err
	}
	return validateKey(key.BaseVersion, key.ContentHash)
}

func ValidateRelationKey(key Key[knowledge.RelationID]) error {
	if err := knowledge.ValidateRelationID(key.ID); err != nil {
		return err
	}
	return validateKey(key.BaseVersion, key.ContentHash)
}

func ValidateClaimKey(key Key[knowledge.ClaimID]) error {
	if err := knowledge.ValidateClaimID(key.ID); err != nil {
		return err
	}
	return validateKey(key.BaseVersion, key.ContentHash)
}

func validateKey(baseVersion knowledge.Version, contentHash knowledge.Hash) error {
	if err := knowledge.ValidateVersion(baseVersion); err != nil {
		return err
	}
	if contentHash == emptyHash {
		return fmt.Errorf("%w: Candidate Content Hash is required", knowledge.ErrInvalidChange)
	}
	return nil
}
