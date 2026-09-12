package zonemerger

import (
	"context"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/zone"
)

func (application *Application) EntityReview(
	ctx context.Context,
	childEntityID knowledge.EntityID,
	childBase knowledge.Version,
) (EntityReview, error) {
	assignment, found, err := application.Conflicts.EntityConflict(ctx, childEntityID, childBase)
	if err != nil {
		return EntityReview{}, err
	}
	if !found {
		return EntityReview{}, knowledge.ErrNotFound
	}
	if err := requireChildZone(ctx, assignment.ChildZoneID); err != nil {
		return EntityReview{}, err
	}
	child, err := application.Resolutions.EntityConflict(ctx, childEntityID)
	if err != nil {
		return EntityReview{}, err
	}
	if child.BaseVersion != assignment.ChildBase {
		return EntityReview{}, ErrConflictChanged
	}
	parentContext, err := zone.RouteContext(ctx, assignment.ParentZoneID)
	if err != nil {
		return EntityReview{}, err
	}
	parent, err := application.parentEntityVersion(parentContext, assignment)
	if err != nil {
		return EntityReview{}, err
	}
	return EntityReview{
		Assignment: assignment,
		Child:      child,
		Parent:     parent,
	}, nil
}

func (application *Application) RelationReview(
	ctx context.Context,
	childRelationID knowledge.RelationID,
	childBase knowledge.Version,
) (RelationReview, error) {
	assignment, found, err := application.Conflicts.RelationConflict(ctx, childRelationID, childBase)
	if err != nil {
		return RelationReview{}, err
	}
	if !found {
		return RelationReview{}, knowledge.ErrNotFound
	}
	if err := requireChildZone(ctx, assignment.ChildZoneID); err != nil {
		return RelationReview{}, err
	}
	child, err := application.Resolutions.RelationConflict(ctx, childRelationID)
	if err != nil {
		return RelationReview{}, err
	}
	if child.BaseVersion != assignment.ChildBase {
		return RelationReview{}, ErrConflictChanged
	}
	parentContext, err := zone.RouteContext(ctx, assignment.ParentZoneID)
	if err != nil {
		return RelationReview{}, err
	}
	parent, err := application.parentRelationVersion(parentContext, assignment)
	if err != nil {
		return RelationReview{}, err
	}
	return RelationReview{
		Assignment: assignment,
		Child:      child,
		Parent:     parent,
	}, nil
}

func (application *Application) ClaimReview(
	ctx context.Context,
	childClaimID knowledge.ClaimID,
	childBase knowledge.Version,
) (ClaimReview, error) {
	assignment, found, err := application.Conflicts.ClaimConflict(ctx, childClaimID, childBase)
	if err != nil {
		return ClaimReview{}, err
	}
	if !found {
		return ClaimReview{}, knowledge.ErrNotFound
	}
	if err := requireChildZone(ctx, assignment.ChildZoneID); err != nil {
		return ClaimReview{}, err
	}
	child, err := application.Resolutions.ClaimConflict(ctx, childClaimID)
	if err != nil {
		return ClaimReview{}, err
	}
	if child.BaseVersion != assignment.ChildBase {
		return ClaimReview{}, ErrConflictChanged
	}
	parentContext, err := zone.RouteContext(ctx, assignment.ParentZoneID)
	if err != nil {
		return ClaimReview{}, err
	}
	parent, err := application.parentClaimVersion(parentContext, assignment)
	if err != nil {
		return ClaimReview{}, err
	}
	return ClaimReview{
		Assignment: assignment,
		Child:      child,
		Parent:     parent,
	}, nil
}
