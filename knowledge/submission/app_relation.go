package submission

import (
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/provenance"
)

func (application *Application) submitRelations(state *transactionState, batches []relationBatch) error {
	for _, batch := range batches {
		source, err := application.resolveEntity(state, batch.source)
		if err != nil {
			return err
		}
		target, err := application.resolveEntity(state, batch.target)
		if err != nil {
			return err
		}
		identity, err := knowledge.NewRelationKey(source.id, target.id, batch.relType)
		if err != nil {
			return err
		}
		id, bound, err := application.KnowledgeIdentities.RelationID(state.ctx, identity)
		if err != nil {
			return err
		}
		newIdentity := !bound
		if newIdentity {
			id, err = knowledge.NewRelationID()
			if err != nil {
				return err
			}
			if err := application.KnowledgeIdentities.ReserveRelation(state.ctx, id, identity); err != nil {
				return err
			}
		}
		current, hasCurrent, err := application.VersionHistory.CurrentRelation(state.ctx, id)
		if err != nil {
			return err
		}
		if newIdentity && hasCurrent {
			return fmt.Errorf("%w: newly reserved Relation %q already has Current", knowledge.ErrDataIntegrity, id)
		}
		variants := batch.variants
		if newIdentity && source.active && target.active {
			relation := relationFromVariant(id, identity, variants[0])
			if err := application.createRelation(state, relation, variants[0].metadata); err != nil {
				return err
			}
			hash, err := knowledge.HashRelation(relation)
			if err != nil {
				return err
			}
			current = knowledge.KnowledgeVersion[knowledge.Relation]{Knowledge: relation, Version: 1, Hash: hash}
			hasCurrent = true
			variants = variants[1:]
		}
		for _, variant := range variants {
			relation := relationFromVariant(id, identity, variant)
			if hasCurrent && !current.Deleted {
				confirmed, err := application.confirmRelation(state, current, relation, variant.metadata)
				if err != nil {
					return err
				}
				if confirmed {
					continue
				}
			}
			if !hasCurrent {
				return fmt.Errorf("%w: Relation endpoints must have active formal Versions", knowledge.ErrInvalidChange)
			}
			if err := application.submitRelationCandidate(state, current.Version, relation, variant.metadata); err != nil {
				return err
			}
		}
		state.relations[batch.key] = relationState{id: id, active: hasCurrent && !current.Deleted}
	}
	return nil
}

func (application *Application) createRelation(
	state *transactionState,
	relation knowledge.Relation,
	metadata provenance.RelationMetadata,
) error {
	hash, err := knowledge.HashRelation(relation)
	if err != nil {
		return err
	}
	if err := application.VersionHistory.AppendRelationVersion(state.ctx, knowledge.KnowledgeVersion[knowledge.Relation]{
		Knowledge: relation, Version: 1, Hash: hash,
	}, state.sourceID); err != nil {
		return err
	}
	if err := application.Sources.ConfirmRelation(state.ctx, provenance.RelationConfirmation{
		RelationID: relation.ID, Version: 1, SourceID: state.sourceID,
		Metadata: metadata,
	}); err != nil {
		return err
	}
	state.result.CreatedVersions.Relations = append(
		state.result.CreatedVersions.Relations,
		knowledge.Reference[knowledge.RelationID]{ID: relation.ID, Version: 1},
	)
	return nil
}

func (application *Application) confirmRelation(
	state *transactionState,
	current knowledge.KnowledgeVersion[knowledge.Relation],
	relation knowledge.Relation,
	metadata provenance.RelationMetadata,
) (bool, error) {
	currentHash, err := knowledge.HashRelation(current.Knowledge)
	if err != nil {
		return false, err
	}
	if currentHash != current.Hash {
		return false, fmt.Errorf("%w: Relation %q Current Hash is invalid", knowledge.ErrDataIntegrity, relation.ID)
	}
	hash, err := knowledge.HashRelation(relation)
	if err != nil {
		return false, err
	}
	if hash != current.Hash {
		return false, nil
	}
	if current.Knowledge != relation {
		return false, fmt.Errorf("%w: Relation %q Hash collision", knowledge.ErrDataIntegrity, relation.ID)
	}
	if err := application.Sources.ConfirmRelation(state.ctx, provenance.RelationConfirmation{
		RelationID: relation.ID, Version: current.Version, SourceID: state.sourceID, Metadata: metadata,
	}); err != nil {
		return false, err
	}
	state.result.ConfirmedVersions.Relations = append(
		state.result.ConfirmedVersions.Relations,
		knowledge.Reference[knowledge.RelationID]{ID: relation.ID, Version: current.Version},
	)
	return true, nil
}

func (application *Application) submitRelationCandidate(
	state *transactionState,
	base knowledge.Version,
	relation knowledge.Relation,
	metadata provenance.RelationMetadata,
) error {
	hash, err := knowledge.HashRelation(relation)
	if err != nil {
		return err
	}
	key := candidate.Key[knowledge.RelationID]{
		ID:          relation.ID,
		BaseVersion: base,
		ContentHash: hash,
	}
	stored, found, err := application.Candidates.FindRelation(state.ctx, key)
	if err != nil {
		return err
	}
	opened := false
	if found {
		if stored.Content != relation {
			return fmt.Errorf("%w: Relation Candidate Hash collision", knowledge.ErrDataIntegrity)
		}
	} else {
		number, err := application.Candidates.NextRelationNumber(state.ctx, relation.ID, base)
		if err != nil {
			return err
		}
		stored = candidate.Relation{
			Key:     key,
			Number:  number,
			Content: relation,
		}
		if err := application.Candidates.AddRelation(state.ctx, stored, state.sourceID); err != nil {
			return err
		}
		opened = true
	}
	if err := application.Sources.SubmitRelation(state.ctx, provenance.RelationSubmission{
		Candidate: key, SourceID: state.sourceID, OpenedCandidate: opened, Metadata: metadata,
	}); err != nil {
		return err
	}
	if opened {
		state.result.OpenedCandidates.Relations = append(state.result.OpenedCandidates.Relations, key)
	} else {
		state.result.ReusedCandidates.Relations = append(state.result.ReusedCandidates.Relations, key)
	}
	return nil
}
