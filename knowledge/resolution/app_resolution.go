package resolution

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

var (
	ErrConflictChanged    = errors.New("Knowledge conflict changed after it was read")
	ErrConflictNotFound   = errors.New("Knowledge conflict was not found")
	ErrResolutionNotFound = errors.New("Knowledge conflict resolution was not found")
)

func (application *Application) EntityResult(
	ctx context.Context,
	command EntityCommand,
) (Result, error) {
	value, err := normalizeEntity(command)
	if err != nil {
		return Result{}, fmt.Errorf("read Entity conflict resolution: %w", err)
	}
	result, found, err := application.acceptedResult(ctx, value.source)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return Result{}, ErrResolutionNotFound
	}
	return result, nil
}

func (application *Application) RelationResult(
	ctx context.Context,
	command RelationCommand,
) (Result, error) {
	value, err := normalizeRelation(command)
	if err != nil {
		return Result{}, fmt.Errorf("read Relation conflict resolution: %w", err)
	}
	result, found, err := application.acceptedResult(ctx, value.source)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return Result{}, ErrResolutionNotFound
	}
	return result, nil
}

func (application *Application) ClaimResult(
	ctx context.Context,
	command ClaimCommand,
) (Result, error) {
	value, err := normalizeClaim(command)
	if err != nil {
		return Result{}, fmt.Errorf("read Claim conflict resolution: %w", err)
	}
	result, found, err := application.acceptedResult(ctx, value.source)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return Result{}, ErrResolutionNotFound
	}
	return result, nil
}

func (application *Application) ResolveEntity(ctx context.Context, command EntityCommand) (Result, error) {
	resolution, err := normalizeEntity(command)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Entity conflict: %w", err)
	}
	if result, found, err := application.acceptedResult(ctx, resolution.source); err != nil || found {
		return result, err
	}

	var result Result
	err = application.Tx.WithTx(ctx, func(ctx context.Context) error {
		if stored, found, err := application.Provenance.Source(ctx, resolution.source.ID); err != nil {
			return err
		} else if found {
			if stored != resolution.source {
				return provenance.SourceIDConflict(resolution.source.ID)
			}
			result, err = application.Provenance.ResolutionResult(ctx, resolution.source.ID)
			return err
		}

		identity, found, err := application.KnowledgeIdentities.EntityIdentity(ctx, resolution.final.ID)
		if err != nil {
			return err
		}
		if !found {
			return knowledge.ErrNotFound
		}
		if identity.Title != resolution.final.Title || identity.Type != resolution.final.Type {
			return fmt.Errorf("%w: Entity Resolution changes the stable identity", knowledge.ErrIdentityConflict)
		}
		conflicts, err := application.EntityConflict(ctx, resolution.final.ID)
		if err != nil {
			return err
		}
		if conflicts.BaseVersion != resolution.baseVersion ||
			!equalExpectations(resolution.candidates, actualEntityExpectations(conflicts)) {
			return ErrConflictChanged
		}
		if err := application.Provenance.RecordSource(ctx, resolution.source, nil); err != nil {
			return err
		}

		version := knowledge.Version(1)
		created := true
		if conflicts.HasCurrent {
			version = conflicts.Current.Version
			created = conflicts.Current.Deleted || !conflicts.Current.Knowledge.Equal(resolution.final)
			if created {
				version, err = knowledge.NextVersion(version)
				if err != nil {
					return err
				}
			}
		}
		if created {
			hash, err := knowledge.HashEntity(resolution.final)
			if err != nil {
				return err
			}
			if err := application.VersionHistory.AppendEntityVersion(ctx, knowledge.KnowledgeVersion[knowledge.Entity]{
				Knowledge: resolution.final, Version: version, Hash: hash,
			}, resolution.source.ID); err != nil {
				return err
			}
		}
		if err := application.Provenance.ConfirmEntity(ctx, provenance.EntityConfirmation{
			EntityID: resolution.final.ID, Version: version, SourceID: resolution.source.ID,
		}); err != nil {
			return err
		}
		for _, expected := range resolution.candidates {
			if err := application.Provenance.ResolveEntityCandidate(ctx, provenance.EntityCandidateResolution{
				Candidate: expected.Candidate, SourceID: resolution.source.ID,
			}); err != nil {
				return err
			}
		}
		result = Result{Version: version, CreatedVersion: created}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("resolve Entity conflict: %w", err)
	}
	return result, nil
}

func (application *Application) ResolveRelation(ctx context.Context, command RelationCommand) (Result, error) {
	resolution, err := normalizeRelation(command)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Relation conflict: %w", err)
	}
	if result, found, err := application.acceptedResult(ctx, resolution.source); err != nil || found {
		return result, err
	}

	var result Result
	err = application.Tx.WithTx(ctx, func(ctx context.Context) error {
		if stored, found, err := application.Provenance.Source(ctx, resolution.source.ID); err != nil {
			return err
		} else if found {
			if stored != resolution.source {
				return provenance.SourceIDConflict(resolution.source.ID)
			}
			result, err = application.Provenance.ResolutionResult(ctx, resolution.source.ID)
			return err
		}
		identity, found, err := application.KnowledgeIdentities.RelationKey(ctx, resolution.final.ID)
		if err != nil {
			return err
		}
		if !found {
			return knowledge.ErrNotFound
		}
		if identity.SourceEntityID != resolution.final.SourceEntityID ||
			identity.TargetEntityID != resolution.final.TargetEntityID || identity.Type != resolution.final.Type {
			return fmt.Errorf("%w: Relation Resolution changes the stable identity", knowledge.ErrIdentityConflict)
		}
		if err := application.requireActiveRelationEndpoints(ctx, resolution.final); err != nil {
			return err
		}
		conflicts, err := application.RelationConflict(ctx, resolution.final.ID)
		if err != nil {
			return err
		}
		if conflicts.BaseVersion != resolution.baseVersion ||
			!equalExpectations(resolution.candidates, actualRelationExpectations(conflicts)) {
			return ErrConflictChanged
		}
		if err := application.Provenance.RecordSource(ctx, resolution.source, nil); err != nil {
			return err
		}
		version := knowledge.Version(1)
		created := true
		if conflicts.HasCurrent {
			version = conflicts.Current.Version
			created = conflicts.Current.Deleted || conflicts.Current.Knowledge != resolution.final
			if created {
				version, err = knowledge.NextVersion(version)
				if err != nil {
					return err
				}
			}
		}
		if created {
			hash, err := knowledge.HashRelation(resolution.final)
			if err != nil {
				return err
			}
			if err := application.VersionHistory.AppendRelationVersion(ctx, knowledge.KnowledgeVersion[knowledge.Relation]{
				Knowledge: resolution.final, Version: version, Hash: hash,
			}, resolution.source.ID); err != nil {
				return err
			}
		}
		if err := application.Provenance.ConfirmRelation(ctx, provenance.RelationConfirmation{
			RelationID: resolution.final.ID, Version: version, SourceID: resolution.source.ID,
		}); err != nil {
			return err
		}
		for _, expected := range resolution.candidates {
			if err := application.Provenance.ResolveRelationCandidate(ctx, provenance.RelationCandidateResolution{
				Candidate: expected.Candidate, SourceID: resolution.source.ID,
			}); err != nil {
				return err
			}
		}
		result = Result{Version: version, CreatedVersion: created}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("resolve Relation conflict: %w", err)
	}
	return result, nil
}

func (application *Application) ResolveClaim(ctx context.Context, command ClaimCommand) (Result, error) {
	resolution, err := normalizeClaim(command)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Claim conflict: %w", err)
	}
	if result, found, err := application.acceptedResult(ctx, resolution.source); err != nil || found {
		return result, err
	}

	var result Result
	err = application.Tx.WithTx(ctx, func(ctx context.Context) error {
		if stored, found, err := application.Provenance.Source(ctx, resolution.source.ID); err != nil {
			return err
		} else if found {
			if stored != resolution.source {
				return provenance.SourceIDConflict(resolution.source.ID)
			}
			result, err = application.Provenance.ResolutionResult(ctx, resolution.source.ID)
			return err
		}
		identity, found, err := application.KnowledgeIdentities.ClaimIdentity(ctx, resolution.final.ID)
		if err != nil {
			return err
		}
		if !found {
			return knowledge.ErrNotFound
		}
		if identity.Type != resolution.final.Type || !knowledge.SameSubject(identity.Subject, resolution.final.Subject) {
			return fmt.Errorf("%w: Claim Resolution changes the stable identity", knowledge.ErrIdentityConflict)
		}
		if err := application.requireActiveSubject(ctx, resolution.final.Subject); err != nil {
			return err
		}
		conflicts, err := application.ClaimConflict(ctx, resolution.final.ID)
		if err != nil {
			return err
		}
		if conflicts.BaseVersion != resolution.baseVersion ||
			!equalExpectations(resolution.candidates, actualClaimExpectations(conflicts)) {
			return ErrConflictChanged
		}
		if err := application.Provenance.RecordSource(ctx, resolution.source, nil); err != nil {
			return err
		}
		version := knowledge.Version(1)
		created := true
		if conflicts.HasCurrent {
			version = conflicts.Current.Version
			created = conflicts.Current.Deleted || !conflicts.Current.Knowledge.Equal(resolution.final)
			if created {
				version, err = knowledge.NextVersion(version)
				if err != nil {
					return err
				}
			}
		}
		if created {
			hash, err := knowledge.HashClaim(resolution.final)
			if err != nil {
				return err
			}
			if err := application.VersionHistory.AppendClaimVersion(ctx, knowledge.KnowledgeVersion[knowledge.Claim]{
				Knowledge: resolution.final, Version: version, Hash: hash,
			}, resolution.source.ID); err != nil {
				return err
			}
		}
		if err := application.Provenance.ConfirmClaim(ctx, provenance.ClaimConfirmation{
			ClaimID: resolution.final.ID, Version: version, SourceID: resolution.source.ID,
		}); err != nil {
			return err
		}
		for _, expected := range resolution.candidates {
			if err := application.Provenance.ResolveClaimCandidate(ctx, provenance.ClaimCandidateResolution{
				Candidate: expected.Candidate, SourceID: resolution.source.ID,
			}); err != nil {
				return err
			}
		}
		result = Result{Version: version, CreatedVersion: created}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("resolve Claim conflict: %w", err)
	}
	return result, nil
}

func (application *Application) acceptedResult(
	ctx context.Context,
	source provenance.Source,
) (Result, bool, error) {
	stored, found, err := application.Provenance.Source(ctx, source.ID)
	if err != nil || !found {
		return Result{}, false, err
	}
	if stored != source {
		return Result{}, true, provenance.SourceIDConflict(source.ID)
	}
	result, err := application.Provenance.ResolutionResult(ctx, source.ID)
	return result, true, err
}

func (application *Application) requireActiveRelationEndpoints(ctx context.Context, relation knowledge.Relation) error {
	for _, id := range []knowledge.EntityID{relation.SourceEntityID, relation.TargetEntityID} {
		current, found, err := application.VersionHistory.CurrentEntity(ctx, id)
		if err != nil {
			return err
		}
		if !found || current.Deleted {
			return fmt.Errorf("%w: Relation endpoint Entity %q is not active", knowledge.ErrInvalidChange, id)
		}
	}
	return nil
}

func (application *Application) requireActiveSubject(ctx context.Context, subject knowledge.Subject) error {
	switch subject := subject.(type) {
	case knowledge.EntityID:
		current, found, err := application.VersionHistory.CurrentEntity(ctx, subject)
		if err != nil {
			return err
		}
		if !found || current.Deleted {
			return fmt.Errorf("%w: Claim Subject Entity %q is not active", knowledge.ErrInvalidChange, subject)
		}
	case knowledge.RelationID:
		current, found, err := application.VersionHistory.CurrentRelation(ctx, subject)
		if err != nil {
			return err
		}
		if !found || current.Deleted {
			return fmt.Errorf("%w: Claim Subject Relation %q is not active", knowledge.ErrInvalidChange, subject)
		}
	default:
		return fmt.Errorf("%w: unsupported Claim Subject %T", knowledge.ErrInvalidChange, subject)
	}
	return nil
}
