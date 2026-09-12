package submission

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/zone"
)

type referenceState struct {
	entities  map[knowledge.EntityIdentity]referencedEntity
	relations map[string]referencedRelation
}

type referencedEntity struct {
	id     knowledge.EntityID
	bound  bool
	active bool
}

type referencedRelation struct {
	id     knowledge.RelationID
	bound  bool
	active bool
}

type referencedSubject struct {
	subject knowledge.Subject
	bound   bool
	active  bool
}

// Preflight validates one submission and every Knowledge reference without
// opening a write transaction. Submit repeats all checks at the write boundary.
func (application *Application) Preflight(ctx context.Context, command Command) error {
	if application == nil {
		return errors.New("Knowledge Submission is not configured")
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	batch, err := normalizeCommand(bindEvidenceZone(command, string(zoneID)))
	if err != nil {
		return fmt.Errorf("validate Knowledge submission: %w", err)
	}
	if stored, found, err := application.Sources.Source(ctx, batch.source.ID); err != nil {
		return err
	} else if found {
		if stored != batch.source {
			return provenance.SourceIDConflict(batch.source.ID)
		}
		return nil
	}
	if err := application.preflightReferences(ctx, batch); err != nil {
		return fmt.Errorf("validate Knowledge references: %w", err)
	}
	return nil
}

func (application *Application) preflightReferences(ctx context.Context, batch sourceBatch) error {
	state := referenceState{
		entities:  make(map[knowledge.EntityIdentity]referencedEntity, len(batch.entities)),
		relations: make(map[string]referencedRelation, len(batch.relations)),
	}
	for _, entity := range batch.entities {
		resolved, err := application.preflightEntity(ctx, entity.identity)
		if err != nil {
			return err
		}
		state.entities[entity.identity] = resolved
	}
	for _, relation := range batch.relations {
		resolved, err := application.preflightRelation(ctx, &state, relation)
		if err != nil {
			return err
		}
		state.relations[relation.key] = resolved
	}
	for _, claim := range batch.claims {
		if err := application.preflightClaim(ctx, &state, claim); err != nil {
			return err
		}
	}
	return nil
}

func (application *Application) preflightEntity(
	ctx context.Context,
	identity knowledge.EntityIdentity,
) (referencedEntity, error) {
	id, found, err := application.KnowledgeIdentities.EntityID(ctx, identity)
	if err != nil {
		return referencedEntity{}, err
	}
	if !found {
		return referencedEntity{active: true}, nil
	}
	current, hasCurrent, err := application.VersionHistory.CurrentEntity(ctx, id)
	if err != nil {
		return referencedEntity{}, err
	}
	if !hasCurrent {
		return referencedEntity{}, fmt.Errorf(
			"%w: Entity %q has no formal Version",
			knowledge.ErrInvalidChange,
			id,
		)
	}
	return referencedEntity{id: id, bound: true, active: !current.Deleted}, nil
}

func (application *Application) resolvePreflightEntity(
	ctx context.Context,
	state *referenceState,
	identity knowledge.EntityIdentity,
) (referencedEntity, error) {
	if resolved, found := state.entities[identity]; found {
		return resolved, nil
	}
	resolved, err := application.preflightEntity(ctx, identity)
	if err != nil {
		return referencedEntity{}, err
	}
	if !resolved.bound {
		return referencedEntity{}, fmt.Errorf(
			"%w: reference identifies unknown Entity %q",
			knowledge.ErrInvalidChange,
			identity.Title,
		)
	}
	state.entities[identity] = resolved
	return resolved, nil
}

func (application *Application) preflightRelation(
	ctx context.Context,
	state *referenceState,
	relation relationBatch,
) (referencedRelation, error) {
	source, err := application.resolvePreflightEntity(ctx, state, relation.source)
	if err != nil {
		return referencedRelation{}, err
	}
	target, err := application.resolvePreflightEntity(ctx, state, relation.target)
	if err != nil {
		return referencedRelation{}, err
	}
	if !source.bound || !target.bound {
		if !source.active || !target.active {
			return referencedRelation{}, fmt.Errorf(
				"%w: Relation endpoints must have active formal Versions",
				knowledge.ErrInvalidChange,
			)
		}
		return referencedRelation{active: true}, nil
	}
	identity, err := knowledge.NewRelationKey(source.id, target.id, relation.relType)
	if err != nil {
		return referencedRelation{}, err
	}
	id, found, err := application.KnowledgeIdentities.RelationID(ctx, identity)
	if err != nil {
		return referencedRelation{}, err
	}
	if !found {
		if !source.active || !target.active {
			return referencedRelation{}, fmt.Errorf(
				"%w: Relation endpoints must have active formal Versions",
				knowledge.ErrInvalidChange,
			)
		}
		return referencedRelation{active: true}, nil
	}
	current, hasCurrent, err := application.VersionHistory.CurrentRelation(ctx, id)
	if err != nil {
		return referencedRelation{}, err
	}
	if !hasCurrent {
		return referencedRelation{}, fmt.Errorf(
			"%w: Relation %q has no formal Version",
			knowledge.ErrInvalidChange,
			id,
		)
	}
	return referencedRelation{id: id, bound: true, active: !current.Deleted}, nil
}

func (application *Application) preflightClaim(
	ctx context.Context,
	state *referenceState,
	claim claimBatch,
) error {
	subject, err := application.preflightSubject(ctx, state, claim)
	if err != nil {
		return err
	}
	if !subject.bound {
		if !subject.active {
			return fmt.Errorf(
				"%w: Claim Subject must have an active formal Version",
				knowledge.ErrInvalidChange,
			)
		}
		return nil
	}
	identity, err := knowledge.NewClaimIdentity(subject.subject, claim.claimType)
	if err != nil {
		return err
	}
	id, found, err := application.KnowledgeIdentities.ClaimID(ctx, identity)
	if err != nil {
		return err
	}
	if !found {
		if !subject.active {
			return fmt.Errorf(
				"%w: Claim Subject must have an active formal Version",
				knowledge.ErrInvalidChange,
			)
		}
		return nil
	}
	if _, hasCurrent, err := application.VersionHistory.CurrentClaim(ctx, id); err != nil {
		return err
	} else if !hasCurrent {
		return fmt.Errorf(
			"%w: Claim %q has no formal Version",
			knowledge.ErrInvalidChange,
			id,
		)
	}
	return nil
}

func (application *Application) preflightSubject(
	ctx context.Context,
	state *referenceState,
	claim claimBatch,
) (referencedSubject, error) {
	switch subject := claim.subject.(type) {
	case knowledge.EntityIdentity:
		resolved, err := application.resolvePreflightEntity(ctx, state, subject)
		if err != nil {
			return referencedSubject{}, err
		}
		if !resolved.bound {
			return referencedSubject{active: resolved.active}, nil
		}
		return referencedSubject{subject: resolved.id, bound: true, active: resolved.active}, nil
	case knowledge.RelationIdentity:
		if resolved, found := state.relations[claim.subjectKey]; found {
			if !resolved.bound {
				return referencedSubject{active: resolved.active}, nil
			}
			return referencedSubject{subject: resolved.id, bound: true, active: resolved.active}, nil
		}
		source, err := application.resolvePreflightEntity(ctx, state, subject.Source)
		if err != nil {
			return referencedSubject{}, err
		}
		target, err := application.resolvePreflightEntity(ctx, state, subject.Target)
		if err != nil {
			return referencedSubject{}, err
		}
		if !source.bound || !target.bound {
			return referencedSubject{}, fmt.Errorf(
				"%w: Claim references unknown Relation",
				knowledge.ErrInvalidChange,
			)
		}
		identity, err := knowledge.NewRelationKey(source.id, target.id, subject.Type)
		if err != nil {
			return referencedSubject{}, err
		}
		id, found, err := application.KnowledgeIdentities.RelationID(ctx, identity)
		if err != nil {
			return referencedSubject{}, err
		}
		if !found {
			return referencedSubject{}, fmt.Errorf(
				"%w: Claim references unknown Relation",
				knowledge.ErrInvalidChange,
			)
		}
		current, hasCurrent, err := application.VersionHistory.CurrentRelation(ctx, id)
		if err != nil {
			return referencedSubject{}, err
		}
		if !hasCurrent {
			return referencedSubject{}, fmt.Errorf(
				"%w: Relation %q has no formal Version",
				knowledge.ErrInvalidChange,
				id,
			)
		}
		return referencedSubject{subject: id, bound: true, active: !current.Deleted}, nil
	default:
		return referencedSubject{}, fmt.Errorf(
			"%w: unsupported Claim Subject %T",
			knowledge.ErrInvalidChange,
			claim.subject,
		)
	}
}
