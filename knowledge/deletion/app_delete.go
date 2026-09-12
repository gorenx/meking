package deletion

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

func New(dependencies Dependencies) (*Application, error) {
	switch {
	case dependencies.Tx == nil:
		return nil, errors.New("create Knowledge Deletion: transaction is required")
	case dependencies.VersionHistory == nil:
		return nil, errors.New("create Knowledge Deletion: Knowledge versions are required")
	case dependencies.KnowledgeReferences == nil:
		return nil, errors.New("create Knowledge Deletion: Knowledge references are required")
	case dependencies.Provenance == nil:
		return nil, errors.New("create Knowledge Deletion: Provenance is required")
	}
	return &Application{Dependencies: dependencies}, nil
}

func (application *Application) Delete(ctx context.Context, command Command) (Result, error) {
	if err := validateCommand(command); err != nil {
		return Result{}, fmt.Errorf("delete Knowledge: %w", err)
	}
	if result, found, err := application.acceptedResult(ctx, command.Source); err != nil || found {
		return result, err
	}

	var result Result
	err := application.Tx.WithTx(ctx, func(ctx context.Context) error {
		if stored, found, err := application.Provenance.Source(ctx, command.Source.ID); err != nil {
			return err
		} else if found {
			if stored != command.Source {
				return provenance.SourceIDConflict(command.Source.ID)
			}
			var resultErr error
			result, resultErr = application.Provenance.DeletionResult(ctx, command.Source.ID)
			return resultErr
		}
		if err := application.Provenance.RecordSource(ctx, command.Source, nil); err != nil {
			return err
		}
		switch id := command.Knowledge.(type) {
		case knowledge.EntityID:
			var resultErr error
			result, resultErr = application.deleteEntity(ctx, command.Source.ID, id, command.ExpectedVersion)
			return resultErr
		case knowledge.RelationID:
			var resultErr error
			result, resultErr = application.deleteRelation(ctx, command.Source.ID, id, command.ExpectedVersion)
			return resultErr
		case knowledge.ClaimID:
			var resultErr error
			result, resultErr = application.deleteClaim(ctx, command.Source.ID, id, command.ExpectedVersion)
			return resultErr
		default:
			return fmt.Errorf("%w: unsupported Knowledge type %T", knowledge.ErrInvalidChange, command.Knowledge)
		}
	})
	if err != nil {
		return Result{}, fmt.Errorf("delete Knowledge: %w", err)
	}
	return result, nil
}

func validateCommand(command Command) error {
	if err := provenance.Validate(command.Source); err != nil {
		return err
	}
	if command.Source.Kind != provenance.Deletion {
		return fmt.Errorf("%w: Knowledge Deletion requires a Deletion Source", provenance.ErrInvalidSource)
	}
	if command.Knowledge == nil {
		return fmt.Errorf("%w: Knowledge ID is required", knowledge.ErrInvalidChange)
	}
	switch id := command.Knowledge.(type) {
	case knowledge.EntityID:
		if err := knowledge.ValidateEntityID(id); err != nil {
			return err
		}
	case knowledge.RelationID:
		if err := knowledge.ValidateRelationID(id); err != nil {
			return err
		}
	case knowledge.ClaimID:
		if err := knowledge.ValidateClaimID(id); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: unsupported Knowledge type %T", knowledge.ErrInvalidChange, command.Knowledge)
	}
	if command.ExpectedVersion == 0 {
		return fmt.Errorf("%w: Expected Version must be positive", knowledge.ErrInvalidChange)
	}
	if err := knowledge.ValidateVersion(command.ExpectedVersion); err != nil {
		return err
	}
	return nil
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
	result, err := application.Provenance.DeletionResult(ctx, source.ID)
	return result, true, err
}

func (application *Application) deleteEntity(
	ctx context.Context,
	sourceID string,
	id knowledge.EntityID,
	expected knowledge.Version,
) (Result, error) {
	current, found, err := application.VersionHistory.CurrentEntity(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return Result{}, knowledge.ErrNotFound
	}
	if current.Version != expected {
		return Result{}, &knowledge.VersionConflict{Object: id, Expected: expected, Actual: current.Version}
	}
	created := !current.Deleted
	version := current.Version
	if created {
		relations, err := application.KnowledgeReferences.ActiveRelations(ctx, id)
		if err != nil {
			return Result{}, err
		}
		subject, _ := knowledge.NewEntitySubject(id)
		claims, err := application.KnowledgeReferences.ActiveClaims(ctx, subject)
		if err != nil {
			return Result{}, err
		}
		if len(relations) > 0 || len(claims) > 0 {
			return Result{}, fmt.Errorf("%w: Entity %q still has active references", ErrKnowledgeInUse, id)
		}
		version, err = knowledge.NextVersion(current.Version)
		if err != nil {
			return Result{}, err
		}
		current.Version = version
		current.Deleted = true
		if err := application.VersionHistory.AppendEntityVersion(ctx, current, sourceID); err != nil {
			return Result{}, err
		}
		if err := application.Provenance.ConfirmEntity(ctx, provenance.EntityConfirmation{
			EntityID: id, Version: version, SourceID: sourceID,
		}); err != nil {
			return Result{}, err
		}
		return Result{Version: version, CreatedVersion: true}, nil
	}
	if err := application.Provenance.ConfirmEntity(ctx, provenance.EntityConfirmation{
		EntityID: id, Version: version, SourceID: sourceID,
	}); err != nil {
		return Result{}, err
	}
	return Result{Version: version}, nil
}

func (application *Application) deleteRelation(
	ctx context.Context,
	sourceID string,
	id knowledge.RelationID,
	expected knowledge.Version,
) (Result, error) {
	current, found, err := application.VersionHistory.CurrentRelation(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return Result{}, knowledge.ErrNotFound
	}
	if current.Version != expected {
		return Result{}, &knowledge.VersionConflict{Object: id, Expected: expected, Actual: current.Version}
	}
	created := !current.Deleted
	version := current.Version
	if created {
		subject, _ := knowledge.NewRelationSubject(id)
		claims, err := application.KnowledgeReferences.ActiveClaims(ctx, subject)
		if err != nil {
			return Result{}, err
		}
		if len(claims) > 0 {
			return Result{}, fmt.Errorf("%w: Relation %q still has active Claims", ErrKnowledgeInUse, id)
		}
		version, err = knowledge.NextVersion(current.Version)
		if err != nil {
			return Result{}, err
		}
		current.Version = version
		current.Deleted = true
		if err := application.VersionHistory.AppendRelationVersion(ctx, current, sourceID); err != nil {
			return Result{}, err
		}
		if err := application.Provenance.ConfirmRelation(ctx, provenance.RelationConfirmation{
			RelationID: id, Version: version, SourceID: sourceID,
		}); err != nil {
			return Result{}, err
		}
		return Result{Version: version, CreatedVersion: true}, nil
	}
	if err := application.Provenance.ConfirmRelation(ctx, provenance.RelationConfirmation{
		RelationID: id, Version: version, SourceID: sourceID,
	}); err != nil {
		return Result{}, err
	}
	return Result{Version: version}, nil
}

func (application *Application) deleteClaim(
	ctx context.Context,
	sourceID string,
	id knowledge.ClaimID,
	expected knowledge.Version,
) (Result, error) {
	current, found, err := application.VersionHistory.CurrentClaim(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return Result{}, knowledge.ErrNotFound
	}
	if current.Version != expected {
		return Result{}, &knowledge.VersionConflict{Object: id, Expected: expected, Actual: current.Version}
	}
	created := !current.Deleted
	version := current.Version
	if created {
		version, err = knowledge.NextVersion(current.Version)
		if err != nil {
			return Result{}, err
		}
		current.Version = version
		current.Deleted = true
		if err := application.VersionHistory.AppendClaimVersion(ctx, current, sourceID); err != nil {
			return Result{}, err
		}
		if err := application.Provenance.ConfirmClaim(ctx, provenance.ClaimConfirmation{
			ClaimID: id, Version: version, SourceID: sourceID,
		}); err != nil {
			return Result{}, err
		}
		return Result{Version: version, CreatedVersion: true}, nil
	}
	if err := application.Provenance.ConfirmClaim(ctx, provenance.ClaimConfirmation{
		ClaimID: id, Version: version, SourceID: sourceID,
	}); err != nil {
		return Result{}, err
	}
	return Result{Version: version}, nil
}
