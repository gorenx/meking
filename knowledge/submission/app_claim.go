package submission

import (
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/provenance"
)

func (application *Application) submitClaims(state *transactionState, batches []claimBatch) error {
	for _, batch := range batches {
		subject, active, err := application.resolveSubject(state, batch)
		if err != nil {
			return err
		}
		identity, err := knowledge.NewClaimIdentity(subject, batch.claimType)
		if err != nil {
			return err
		}
		id, bound, err := application.KnowledgeIdentities.ClaimID(state.ctx, identity)
		if err != nil {
			return err
		}
		newIdentity := !bound
		if newIdentity {
			id, err = knowledge.NewClaimID()
			if err != nil {
				return err
			}
			if err := application.KnowledgeIdentities.ReserveClaim(state.ctx, id, identity); err != nil {
				return err
			}
		}
		current, hasCurrent, err := application.VersionHistory.CurrentClaim(state.ctx, id)
		if err != nil {
			return err
		}
		if newIdentity && hasCurrent {
			return fmt.Errorf("%w: newly reserved Claim %q already has Current", knowledge.ErrDataIntegrity, id)
		}
		variants := batch.variants
		if newIdentity && active {
			claim := claimFromVariant(id, identity, variants[0])
			if err := application.createClaim(state, claim, variants[0].metadata); err != nil {
				return err
			}
			hash, err := knowledge.HashClaim(claim)
			if err != nil {
				return err
			}
			current = knowledge.KnowledgeVersion[knowledge.Claim]{Knowledge: claim, Version: 1, Hash: hash}
			hasCurrent = true
			variants = variants[1:]
		}
		for _, variant := range variants {
			claim := claimFromVariant(id, identity, variant)
			if hasCurrent && !current.Deleted {
				confirmed, err := application.confirmClaim(state, current, claim, variant.metadata)
				if err != nil {
					return err
				}
				if confirmed {
					continue
				}
			}
			if !hasCurrent {
				return fmt.Errorf("%w: Claim Subject must have an active formal Version", knowledge.ErrInvalidChange)
			}
			if err := application.submitClaimCandidate(state, current.Version, claim, variant.metadata); err != nil {
				return err
			}
		}
	}
	return nil
}

func (application *Application) createClaim(
	state *transactionState,
	claim knowledge.Claim,
	metadata []provenance.ClaimMetadata,
) error {
	hash, err := knowledge.HashClaim(claim)
	if err != nil {
		return err
	}
	if err := application.VersionHistory.AppendClaimVersion(state.ctx, knowledge.KnowledgeVersion[knowledge.Claim]{
		Knowledge: claim, Version: 1, Hash: hash,
	}, state.sourceID); err != nil {
		return err
	}
	if err := application.Sources.ConfirmClaim(state.ctx, provenance.ClaimConfirmation{
		ClaimID: claim.ID, Version: 1, SourceID: state.sourceID,
		Metadata: metadata,
	}); err != nil {
		return err
	}
	state.result.CreatedVersions.Claims = append(
		state.result.CreatedVersions.Claims,
		knowledge.Reference[knowledge.ClaimID]{ID: claim.ID, Version: 1},
	)
	return nil
}

func (application *Application) confirmClaim(
	state *transactionState,
	current knowledge.KnowledgeVersion[knowledge.Claim],
	claim knowledge.Claim,
	metadata []provenance.ClaimMetadata,
) (bool, error) {
	currentHash, err := knowledge.HashClaim(current.Knowledge)
	if err != nil {
		return false, err
	}
	if currentHash != current.Hash {
		return false, fmt.Errorf("%w: Claim %q Current Hash is invalid", knowledge.ErrDataIntegrity, claim.ID)
	}
	hash, err := knowledge.HashClaim(claim)
	if err != nil {
		return false, err
	}
	if hash != current.Hash {
		return false, nil
	}
	if !current.Knowledge.Equal(claim) {
		return false, fmt.Errorf("%w: Claim %q Hash collision", knowledge.ErrDataIntegrity, claim.ID)
	}
	if err := application.Sources.ConfirmClaim(state.ctx, provenance.ClaimConfirmation{
		ClaimID: claim.ID, Version: current.Version, SourceID: state.sourceID, Metadata: metadata,
	}); err != nil {
		return false, err
	}
	state.result.ConfirmedVersions.Claims = append(
		state.result.ConfirmedVersions.Claims,
		knowledge.Reference[knowledge.ClaimID]{ID: claim.ID, Version: current.Version},
	)
	return true, nil
}

func (application *Application) submitClaimCandidate(
	state *transactionState,
	base knowledge.Version,
	claim knowledge.Claim,
	metadata []provenance.ClaimMetadata,
) error {
	hash, err := knowledge.HashClaim(claim)
	if err != nil {
		return err
	}
	key := candidate.Key[knowledge.ClaimID]{
		ID:          claim.ID,
		BaseVersion: base,
		ContentHash: hash,
	}
	stored, found, err := application.Candidates.FindClaim(state.ctx, key)
	if err != nil {
		return err
	}
	opened := false
	if found {
		if !stored.Content.Equal(claim) {
			return fmt.Errorf("%w: Claim Candidate Hash collision", knowledge.ErrDataIntegrity)
		}
	} else {
		number, err := application.Candidates.NextClaimNumber(state.ctx, claim.ID, base)
		if err != nil {
			return err
		}
		stored = candidate.Claim{
			Key:     key,
			Number:  number,
			Content: claim,
		}
		if err := application.Candidates.AddClaim(state.ctx, stored, state.sourceID); err != nil {
			return err
		}
		opened = true
	}
	if err := application.Sources.SubmitClaim(state.ctx, provenance.ClaimSubmission{
		Candidate: key, SourceID: state.sourceID, OpenedCandidate: opened, Metadata: metadata,
	}); err != nil {
		return err
	}
	if opened {
		state.result.OpenedCandidates.Claims = append(state.result.OpenedCandidates.Claims, key)
	} else {
		state.result.ReusedCandidates.Claims = append(state.result.ReusedCandidates.Claims, key)
	}
	return nil
}
