package zonemerger

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/zone"
)

type mergeScope struct {
	ParentContext context.Context
	ChildContext  context.Context
	ParentZoneID  zone.ID
	ChildZoneID   zone.ID
}

// MergeChildKnowledge compares one fixed Child formal view with the Parent.
// Different content is submitted back to the Child for review; it is never
// opened as a Parent conflict before the Child decides.
func (application *Application) MergeChildKnowledge(ctx context.Context) error {
	if application == nil {
		return errors.New("merge Child Knowledge: application is not configured")
	}
	parentZoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	childZoneID, err := zone.RequireChildID(ctx)
	if err != nil {
		return err
	}
	if childZoneID == parentZoneID {
		return ErrInvalidMerge
	}
	childContext, err := zone.RouteContext(ctx, childZoneID)
	if err != nil {
		return err
	}
	child, err := application.readChildKnowledge(childContext)
	if err != nil {
		return fmt.Errorf("read Child Zone %q Knowledge: %w", childZoneID, err)
	}

	return application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		childTransactionContext, err := zone.RouteContext(transactionContext, childZoneID)
		if err != nil {
			return err
		}
		scope := mergeScope{
			ParentContext: transactionContext,
			ChildContext:  childTransactionContext,
			ParentZoneID:  parentZoneID,
			ChildZoneID:   childZoneID,
		}
		parentEntities := make(map[knowledge.EntityID]knowledge.EntityID, len(child.Entities))
		for _, entity := range child.Entities {
			parentID, err := application.mergeEntity(
				scope,
				entity,
			)
			if err != nil {
				return fmt.Errorf("merge Child Entity %q: %w", entity.Version.Knowledge.ID, err)
			}
			parentEntities[entity.Version.Knowledge.ID] = parentID
		}

		parentRelations := make(map[knowledge.RelationID]knowledge.RelationID, len(child.Relations))
		for _, relation := range child.Relations {
			parentID, err := application.mergeRelation(
				scope,
				relation,
				parentEntities,
			)
			if err != nil {
				return fmt.Errorf("merge Child Relation %q: %w", relation.Version.Knowledge.ID, err)
			}
			parentRelations[relation.Version.Knowledge.ID] = parentID
		}

		for _, claim := range child.Claims {
			if err := application.mergeClaim(
				scope,
				claim,
				parentEntities,
				parentRelations,
			); err != nil {
				return fmt.Errorf("merge Child Claim %q: %w", claim.Version.Knowledge.ID, err)
			}
		}
		return nil
	})
}

func (application *Application) mergeEntity(
	scope mergeScope,
	child childEntity,
) (knowledge.EntityID, error) {
	if err := application.requireChildEntity(scope.ChildContext, child.Version); err != nil {
		return "", err
	}
	identity := child.Content.Content.Identity
	parentID, found, err := application.Identities.EntityID(scope.ParentContext, identity)
	if err != nil {
		return "", err
	}
	if !found {
		source, err := childMergeSource(mergeSourceIdentity{
			ChildZoneID:  scope.ChildZoneID,
			ObjectType:   "entity",
			ChildID:      string(child.Version.Knowledge.ID),
			ChildVersion: child.Version.Version,
		})
		if err != nil {
			return "", err
		}
		if _, err := application.Submissions.Submit(scope.ParentContext, submission.Command{
			Source:   source,
			Entities: []submission.Entity{child.Content},
		}); err != nil {
			return "", err
		}
		parentID, found, err = application.Identities.EntityID(scope.ParentContext, identity)
		if err != nil || !found {
			return "", missingParentIdentity("Entity", err)
		}
		return parentID, nil
	}
	parent, hasCurrent, err := application.CurrentVersions.CurrentEntity(scope.ParentContext, parentID)
	if err != nil {
		return "", err
	}
	if !hasCurrent || parent.Deleted {
		return "", fmt.Errorf("%w: Parent Entity %q has no active formal value", ErrConflictChanged, parentID)
	}
	if equalEntities(parent.Knowledge, child.Version.Knowledge) {
		source, err := childMergeSource(mergeSourceIdentity{
			ChildZoneID:   scope.ChildZoneID,
			ObjectType:    "entity",
			ChildID:       string(child.Version.Knowledge.ID),
			ChildVersion:  child.Version.Version,
			ParentID:      string(parentID),
			ParentVersion: parent.Version,
			ParentHash:    parent.Hash,
		})
		if err != nil {
			return "", err
		}
		_, err = application.Submissions.Submit(scope.ParentContext, submission.Command{
			Source:   source,
			Entities: []submission.Entity{child.Content},
		})
		return parentID, err
	}
	return parentID, application.assignEntityConflict(
		scope.ChildContext,
		entityDifference{
			ChildZoneID:  scope.ChildZoneID,
			ParentZoneID: scope.ParentZoneID,
			Child:        child,
			Parent:       parent,
		},
	)
}

func (application *Application) mergeRelation(
	scope mergeScope,
	child childRelation,
	parentEntities map[knowledge.EntityID]knowledge.EntityID,
) (knowledge.RelationID, error) {
	if err := application.requireChildRelation(scope.ChildContext, child.Version); err != nil {
		return "", err
	}
	relation := child.Version.Knowledge
	parentSourceID, sourceFound := parentEntities[relation.SourceEntityID]
	parentTargetID, targetFound := parentEntities[relation.TargetEntityID]
	if !sourceFound || !targetFound {
		return "", fmt.Errorf("%w: Parent Relation endpoint mapping is missing", knowledge.ErrDataIntegrity)
	}
	identity, err := knowledge.NewRelationKey(parentSourceID, parentTargetID, relation.Type)
	if err != nil {
		return "", err
	}
	parentID, found, err := application.Identities.RelationID(scope.ParentContext, identity)
	if err != nil {
		return "", err
	}
	if !found {
		source, err := childMergeSource(mergeSourceIdentity{
			ChildZoneID:  scope.ChildZoneID,
			ObjectType:   "relation",
			ChildID:      string(relation.ID),
			ChildVersion: child.Version.Version,
		})
		if err != nil {
			return "", err
		}
		if _, err := application.Submissions.Submit(scope.ParentContext, submission.Command{
			Source:    source,
			Relations: []submission.Relation{child.Content},
		}); err != nil {
			return "", err
		}
		parentID, found, err = application.Identities.RelationID(scope.ParentContext, identity)
		if err != nil || !found {
			return "", missingParentIdentity("Relation", err)
		}
		return parentID, nil
	}
	parent, hasCurrent, err := application.CurrentVersions.CurrentRelation(scope.ParentContext, parentID)
	if err != nil {
		return "", err
	}
	if !hasCurrent || parent.Deleted {
		return "", fmt.Errorf("%w: Parent Relation %q has no active formal value", ErrConflictChanged, parentID)
	}
	if parent.Knowledge.Description == relation.Description {
		source, err := childMergeSource(mergeSourceIdentity{
			ChildZoneID:   scope.ChildZoneID,
			ObjectType:    "relation",
			ChildID:       string(relation.ID),
			ChildVersion:  child.Version.Version,
			ParentID:      string(parentID),
			ParentVersion: parent.Version,
			ParentHash:    parent.Hash,
		})
		if err != nil {
			return "", err
		}
		_, err = application.Submissions.Submit(scope.ParentContext, submission.Command{
			Source:    source,
			Relations: []submission.Relation{child.Content},
		})
		return parentID, err
	}
	return parentID, application.assignRelationConflict(
		scope.ChildContext,
		relationDifference{
			ChildZoneID:  scope.ChildZoneID,
			ParentZoneID: scope.ParentZoneID,
			Child:        child,
			Parent:       parent,
		},
	)
}

func (application *Application) mergeClaim(
	scope mergeScope,
	child childClaim,
	parentEntities map[knowledge.EntityID]knowledge.EntityID,
	parentRelations map[knowledge.RelationID]knowledge.RelationID,
) error {
	if err := application.requireChildClaim(scope.ChildContext, child.Version); err != nil {
		return err
	}
	claim := child.Version.Knowledge
	var parentSubject knowledge.Subject
	switch subject := claim.Subject.(type) {
	case knowledge.EntityID:
		parentSubject = parentEntities[subject]
	case knowledge.RelationID:
		parentSubject = parentRelations[subject]
	}
	if parentSubject == nil || parentSubject.ObjectID() == "" {
		return fmt.Errorf("%w: Parent Claim subject mapping is missing", knowledge.ErrDataIntegrity)
	}
	identity, err := knowledge.NewClaimIdentity(parentSubject, claim.Type)
	if err != nil {
		return err
	}
	parentID, found, err := application.Identities.ClaimID(scope.ParentContext, identity)
	if err != nil {
		return err
	}
	if !found {
		source, err := childMergeSource(mergeSourceIdentity{
			ChildZoneID:  scope.ChildZoneID,
			ObjectType:   "claim",
			ChildID:      string(claim.ID),
			ChildVersion: child.Version.Version,
		})
		if err != nil {
			return err
		}
		_, err = application.Submissions.Submit(scope.ParentContext, submission.Command{
			Source: source,
			Claims: []submission.Claim{child.Content},
		})
		return err
	}
	parent, hasCurrent, err := application.CurrentVersions.CurrentClaim(scope.ParentContext, parentID)
	if err != nil {
		return err
	}
	if !hasCurrent || parent.Deleted {
		return fmt.Errorf("%w: Parent Claim %q has no active formal value", ErrConflictChanged, parentID)
	}
	if parent.Knowledge.Description == claim.Description {
		source, err := childMergeSource(mergeSourceIdentity{
			ChildZoneID:   scope.ChildZoneID,
			ObjectType:    "claim",
			ChildID:       string(claim.ID),
			ChildVersion:  child.Version.Version,
			ParentID:      string(parentID),
			ParentVersion: parent.Version,
			ParentHash:    parent.Hash,
		})
		if err != nil {
			return err
		}
		_, err = application.Submissions.Submit(scope.ParentContext, submission.Command{
			Source: source,
			Claims: []submission.Claim{child.Content},
		})
		return err
	}
	return application.assignClaimConflict(
		scope.ChildContext,
		claimDifference{
			ChildZoneID:  scope.ChildZoneID,
			ParentZoneID: scope.ParentZoneID,
			Child:        child,
			Parent:       parent,
		},
	)
}

func (application *Application) requireChildEntity(ctx context.Context, expected knowledge.KnowledgeVersion[knowledge.Entity]) error {
	current, found, err := application.CurrentVersions.CurrentEntity(ctx, expected.Knowledge.ID)
	if err != nil {
		return err
	}
	if !found || current.Version != expected.Version || current.Deleted || current.Hash != expected.Hash || !equalEntities(current.Knowledge, expected.Knowledge) {
		return ErrConflictChanged
	}
	return nil
}

func (application *Application) requireChildRelation(ctx context.Context, expected knowledge.KnowledgeVersion[knowledge.Relation]) error {
	current, found, err := application.CurrentVersions.CurrentRelation(ctx, expected.Knowledge.ID)
	if err != nil {
		return err
	}
	if !found || current != expected || current.Deleted {
		return ErrConflictChanged
	}
	return nil
}

func (application *Application) requireChildClaim(ctx context.Context, expected knowledge.KnowledgeVersion[knowledge.Claim]) error {
	current, found, err := application.CurrentVersions.CurrentClaim(ctx, expected.Knowledge.ID)
	if err != nil {
		return err
	}
	if !found || current.Version != expected.Version || current.Deleted || current.Hash != expected.Hash || !knowledge.SameSubject(current.Knowledge.Subject, expected.Knowledge.Subject) || current.Knowledge.Type != expected.Knowledge.Type || current.Knowledge.Description != expected.Knowledge.Description {
		return ErrConflictChanged
	}
	return nil
}

func equalEntities(left knowledge.Entity, right knowledge.Entity) bool {
	return left.Title == right.Title &&
		left.Type == right.Type &&
		slices.Equal(left.Aliases, right.Aliases) &&
		left.Description == right.Description
}

func missingParentIdentity(kind string, err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: merged Parent %s identity is missing", knowledge.ErrDataIntegrity, kind)
}
