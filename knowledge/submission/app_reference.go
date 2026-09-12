package submission

import (
	"fmt"

	"github.com/memoria-space/meking/knowledge"
)

func (application *Application) resolveEntity(
	state *transactionState,
	identity knowledge.EntityIdentity,
) (entityState, error) {
	if resolved, found := state.entities[identity]; found {
		return resolved, nil
	}
	id, found, err := application.KnowledgeIdentities.EntityID(state.ctx, identity)
	if err != nil {
		return entityState{}, err
	}
	if !found {
		return entityState{}, fmt.Errorf("%w: Relation references unknown Entity %q", knowledge.ErrInvalidChange, identity.Title)
	}
	current, hasCurrent, err := application.VersionHistory.CurrentEntity(state.ctx, id)
	if err != nil {
		return entityState{}, err
	}
	resolved := entityState{id: id, active: hasCurrent && !current.Deleted}
	state.entities[identity] = resolved
	return resolved, nil
}

func (application *Application) resolveSubject(
	state *transactionState,
	batch claimBatch,
) (knowledge.Subject, bool, error) {
	switch subject := batch.subject.(type) {
	case knowledge.EntityIdentity:
		resolved, err := application.resolveEntity(state, subject)
		return resolved.id, resolved.active, err
	case knowledge.RelationIdentity:
		if resolved, found := state.relations[batch.subjectKey]; found {
			return resolved.id, resolved.active, nil
		}
		source, err := application.resolveEntity(state, subject.Source)
		if err != nil {
			return nil, false, err
		}
		target, err := application.resolveEntity(state, subject.Target)
		if err != nil {
			return nil, false, err
		}
		identity, err := knowledge.NewRelationKey(source.id, target.id, subject.Type)
		if err != nil {
			return nil, false, err
		}
		id, found, err := application.KnowledgeIdentities.RelationID(state.ctx, identity)
		if err != nil {
			return nil, false, err
		}
		if !found {
			return nil, false, fmt.Errorf("%w: Claim references unknown Relation", knowledge.ErrInvalidChange)
		}
		current, hasCurrent, err := application.VersionHistory.CurrentRelation(state.ctx, id)
		if err != nil {
			return nil, false, err
		}
		resolved := relationState{id: id, active: hasCurrent && !current.Deleted}
		state.relations[batch.subjectKey] = resolved
		return id, resolved.active, nil
	default:
		return nil, false, fmt.Errorf("%w: unsupported Claim Subject %T", knowledge.ErrInvalidChange, batch.subject)
	}
}
