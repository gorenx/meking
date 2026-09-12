package submission

import (
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/provenance"
)

func (application *Application) submitEntities(state *transactionState, batches []entityBatch) error {
	for _, batch := range batches {
		id, bound, err := application.KnowledgeIdentities.EntityID(state.ctx, batch.identity)
		if err != nil {
			return err
		}
		newIdentity := !bound
		if newIdentity {
			id, err = knowledge.NewEntityID()
			if err != nil {
				return err
			}
			if err := application.KnowledgeIdentities.ReserveEntity(state.ctx, id, batch.identity); err != nil {
				return err
			}
		}
		current, hasCurrent, err := application.VersionHistory.CurrentEntity(state.ctx, id)
		if err != nil {
			return err
		}
		if newIdentity && hasCurrent {
			return fmt.Errorf("%w: newly reserved Entity %q already has Current", knowledge.ErrDataIntegrity, id)
		}
		variants := batch.variants
		if newIdentity {
			entity := entityFromVariant(id, batch.identity, variants[0])
			if err := application.createEntity(state, entity, variants[0].metadata); err != nil {
				return err
			}
			hash, err := knowledge.HashEntity(entity)
			if err != nil {
				return err
			}
			current = knowledge.KnowledgeVersion[knowledge.Entity]{Knowledge: entity, Version: 1, Hash: hash}
			hasCurrent = true
			variants = variants[1:]
		}
		for _, variant := range variants {
			entity := entityFromVariant(id, batch.identity, variant)
			if hasCurrent && !current.Deleted {
				confirmed, err := application.confirmEntity(state, current, entity, variant.metadata)
				if err != nil {
					return err
				}
				if confirmed {
					continue
				}
			}
			if err := application.submitEntityCandidate(state, current.Version, entity, variant.metadata); err != nil {
				return err
			}
		}
		state.entities[batch.identity] = entityState{id: id, active: hasCurrent && !current.Deleted}
	}
	return nil
}

func (application *Application) createEntity(
	state *transactionState,
	entity knowledge.Entity,
	metadata provenance.EntityMetadata,
) error {
	hash, err := knowledge.HashEntity(entity)
	if err != nil {
		return err
	}
	if err := application.VersionHistory.AppendEntityVersion(state.ctx, knowledge.KnowledgeVersion[knowledge.Entity]{
		Knowledge: entity, Version: 1, Hash: hash,
	}, state.sourceID); err != nil {
		return err
	}
	if err := application.Sources.ConfirmEntity(state.ctx, provenance.EntityConfirmation{
		EntityID: entity.ID, Version: 1, SourceID: state.sourceID,
		Metadata: metadata,
	}); err != nil {
		return err
	}
	state.result.CreatedVersions.Entities = append(
		state.result.CreatedVersions.Entities,
		knowledge.Reference[knowledge.EntityID]{ID: entity.ID, Version: 1},
	)
	return nil
}

func (application *Application) confirmEntity(
	state *transactionState,
	current knowledge.KnowledgeVersion[knowledge.Entity],
	entity knowledge.Entity,
	metadata provenance.EntityMetadata,
) (bool, error) {
	currentHash, err := knowledge.HashEntity(current.Knowledge)
	if err != nil {
		return false, err
	}
	if currentHash != current.Hash {
		return false, fmt.Errorf("%w: Entity %q Current Hash is invalid", knowledge.ErrDataIntegrity, entity.ID)
	}
	hash, err := knowledge.HashEntity(entity)
	if err != nil {
		return false, err
	}
	if hash != current.Hash {
		return false, nil
	}
	if !current.Knowledge.Equal(entity) {
		return false, fmt.Errorf("%w: Entity %q Hash collision", knowledge.ErrDataIntegrity, entity.ID)
	}
	if err := application.Sources.ConfirmEntity(state.ctx, provenance.EntityConfirmation{
		EntityID: entity.ID, Version: current.Version, SourceID: state.sourceID, Metadata: metadata,
	}); err != nil {
		return false, err
	}
	state.result.ConfirmedVersions.Entities = append(
		state.result.ConfirmedVersions.Entities,
		knowledge.Reference[knowledge.EntityID]{ID: entity.ID, Version: current.Version},
	)
	return true, nil
}

func (application *Application) submitEntityCandidate(
	state *transactionState,
	base knowledge.Version,
	entity knowledge.Entity,
	metadata provenance.EntityMetadata,
) error {
	hash, err := knowledge.HashEntity(entity)
	if err != nil {
		return err
	}
	key := candidate.Key[knowledge.EntityID]{
		ID:          entity.ID,
		BaseVersion: base,
		ContentHash: hash,
	}
	stored, found, err := application.Candidates.FindEntity(state.ctx, key)
	if err != nil {
		return err
	}
	opened := false
	if found {
		if !stored.Content.Equal(entity) {
			return fmt.Errorf("%w: Entity Candidate Hash collision", knowledge.ErrDataIntegrity)
		}
	} else {
		number, err := application.Candidates.NextEntityNumber(state.ctx, entity.ID, base)
		if err != nil {
			return err
		}
		stored = candidate.Entity{
			Key:     key,
			Number:  number,
			Content: entity,
		}
		if err := application.Candidates.AddEntity(state.ctx, stored, state.sourceID); err != nil {
			return err
		}
		opened = true
	}
	if err := application.Sources.SubmitEntity(state.ctx, provenance.EntitySubmission{
		Candidate: key, SourceID: state.sourceID, OpenedCandidate: opened, Metadata: metadata,
	}); err != nil {
		return err
	}
	if opened {
		state.result.OpenedCandidates.Entities = append(state.result.OpenedCandidates.Entities, key)
	} else {
		state.result.ReusedCandidates.Entities = append(state.result.ReusedCandidates.Entities, key)
	}
	return nil
}
