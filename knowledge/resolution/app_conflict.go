package resolution

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
)

func New(dependencies Dependencies) (*Application, error) {
	switch {
	case dependencies.Tx == nil:
		return nil, errors.New("create Conflict Resolution: transaction is required")
	case dependencies.KnowledgeIdentities == nil:
		return nil, errors.New("create Conflict Resolution: Knowledge identities are required")
	case dependencies.VersionHistory == nil:
		return nil, errors.New("create Conflict Resolution: Knowledge versions are required")
	case dependencies.Candidates == nil:
		return nil, errors.New("create Conflict Resolution: Candidates are required")
	case dependencies.Provenance == nil:
		return nil, errors.New("create Conflict Resolution: Provenance is required")
	}
	return &Application{Dependencies: dependencies}, nil
}

func (application *Application) EntityConflict(
	ctx context.Context,
	entityID knowledge.EntityID,
) (EntityConflict, error) {
	if application == nil {
		return EntityConflict{}, errors.New("read Entity conflict: application is not configured")
	}
	if err := knowledge.ValidateEntityID(entityID); err != nil {
		return EntityConflict{}, err
	}
	if _, found, err := application.KnowledgeIdentities.EntityIdentity(ctx, entityID); err != nil {
		return EntityConflict{}, err
	} else if !found {
		return EntityConflict{}, conflictNotFound("Entity", entityID)
	}
	current, found, err := application.VersionHistory.CurrentEntity(ctx, entityID)
	if err != nil {
		return EntityConflict{}, err
	}
	result := EntityConflict{Current: current, HasCurrent: found}
	if found {
		result.BaseVersion = current.Version
		result.Confirmations, err = application.Provenance.ReadEntityEvidence(ctx, knowledge.Reference[knowledge.EntityID]{
			ID:      entityID,
			Version: current.Version,
		})
		if err != nil {
			return EntityConflict{}, err
		}
	}
	candidates, err := application.Candidates.ListEntities(ctx, entityID, result.BaseVersion)
	if err != nil {
		return EntityConflict{}, err
	}
	result.Candidates = make([]EntityCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		sources, err := application.Provenance.EntitySubmissions(ctx, candidate.Key)
		if err != nil {
			return EntityConflict{}, err
		}
		result.Candidates = append(result.Candidates, EntityCandidate{Candidate: candidate, Sources: sources})
	}
	if !hasPendingEntityCandidate(result.Candidates) {
		return EntityConflict{}, conflictNotFound("Entity", entityID)
	}
	return result, nil
}

func (application *Application) RelationConflict(
	ctx context.Context,
	relationID knowledge.RelationID,
) (RelationConflict, error) {
	if application == nil {
		return RelationConflict{}, errors.New("read Relation conflict: application is not configured")
	}
	if err := knowledge.ValidateRelationID(relationID); err != nil {
		return RelationConflict{}, err
	}
	if _, found, err := application.KnowledgeIdentities.RelationKey(ctx, relationID); err != nil {
		return RelationConflict{}, err
	} else if !found {
		return RelationConflict{}, conflictNotFound("Relation", relationID)
	}
	current, found, err := application.VersionHistory.CurrentRelation(ctx, relationID)
	if err != nil {
		return RelationConflict{}, err
	}
	result := RelationConflict{Current: current, HasCurrent: found}
	if found {
		result.BaseVersion = current.Version
		result.Confirmations, err = application.Provenance.ReadRelationEvidence(ctx,
			knowledge.Reference[knowledge.RelationID]{
				ID:      relationID,
				Version: current.Version,
			},
		)
		if err != nil {
			return RelationConflict{}, err
		}
	}
	candidates, err := application.Candidates.ListRelations(ctx, relationID, result.BaseVersion)
	if err != nil {
		return RelationConflict{}, err
	}
	result.Candidates = make([]RelationCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		sources, err := application.Provenance.RelationSubmissions(ctx, candidate.Key)
		if err != nil {
			return RelationConflict{}, err
		}
		result.Candidates = append(result.Candidates, RelationCandidate{Candidate: candidate, Sources: sources})
	}
	if !hasPendingRelationCandidate(result.Candidates) {
		return RelationConflict{}, conflictNotFound("Relation", relationID)
	}
	return result, nil
}

func (application *Application) ClaimConflict(
	ctx context.Context,
	claimID knowledge.ClaimID,
) (ClaimConflict, error) {
	if application == nil {
		return ClaimConflict{}, errors.New("read Claim conflict: application is not configured")
	}
	if err := knowledge.ValidateClaimID(claimID); err != nil {
		return ClaimConflict{}, err
	}
	if _, found, err := application.KnowledgeIdentities.ClaimIdentity(ctx, claimID); err != nil {
		return ClaimConflict{}, err
	} else if !found {
		return ClaimConflict{}, conflictNotFound("Claim", claimID)
	}
	current, found, err := application.VersionHistory.CurrentClaim(ctx, claimID)
	if err != nil {
		return ClaimConflict{}, err
	}
	result := ClaimConflict{Current: current, HasCurrent: found}
	if found {
		result.BaseVersion = current.Version
		result.Confirmations, err = application.Provenance.ReadClaimEvidence(ctx, knowledge.Reference[knowledge.ClaimID]{
			ID:      claimID,
			Version: current.Version,
		})
		if err != nil {
			return ClaimConflict{}, err
		}
	}
	candidates, err := application.Candidates.ListClaims(ctx, claimID, result.BaseVersion)
	if err != nil {
		return ClaimConflict{}, err
	}
	result.Candidates = make([]ClaimCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		sources, err := application.Provenance.ClaimSubmissions(ctx, candidate.Key)
		if err != nil {
			return ClaimConflict{}, err
		}
		result.Candidates = append(result.Candidates, ClaimCandidate{Candidate: candidate, Sources: sources})
	}
	if !hasPendingClaimCandidate(result.Candidates) {
		return ClaimConflict{}, conflictNotFound("Claim", claimID)
	}
	return result, nil
}

func conflictNotFound(kind string, id knowledge.ObjectRef) error {
	return fmt.Errorf("%w: %w: %s %q has no pending conflict", ErrConflictNotFound, knowledge.ErrNotFound, kind, id.ObjectID())
}

func hasPendingEntityCandidate(candidates []EntityCandidate) bool {
	for _, candidate := range candidates {
		if len(candidate.Sources) > 0 {
			return true
		}
	}
	return false
}

func hasPendingRelationCandidate(candidates []RelationCandidate) bool {
	for _, candidate := range candidates {
		if len(candidate.Sources) > 0 {
			return true
		}
	}
	return false
}

func hasPendingClaimCandidate(candidates []ClaimCandidate) bool {
	for _, candidate := range candidates {
		if len(candidate.Sources) > 0 {
			return true
		}
	}
	return false
}
