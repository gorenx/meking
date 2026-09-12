package zonemerger

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
)

func (application *Application) parentEntityVersion(
	ctx context.Context,
	assignment EntityConflict,
) (_ knowledge.KnowledgeVersion[knowledge.Entity], resultErr error) {
	view, err := application.CurrentKnowledge.OpenCurrent(ctx)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	versions, err := view.Entities().Read(ctx, []knowledge.Reference[knowledge.EntityID]{
		{
			ID:      assignment.ParentEntityID,
			Version: assignment.ParentVersion,
		},
	})
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, err
	}
	if len(versions) != 1 || versions[0].Deleted || versions[0].Hash != assignment.CandidateHash {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, missingParentVersion("Entity")
	}
	return versions[0], nil
}

func (application *Application) parentRelationVersion(
	ctx context.Context,
	assignment RelationConflict,
) (_ knowledge.KnowledgeVersion[knowledge.Relation], resultErr error) {
	view, err := application.CurrentKnowledge.OpenCurrent(ctx)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	versions, err := view.Relations().Read(ctx, []knowledge.Reference[knowledge.RelationID]{
		{
			ID:      assignment.ParentRelationID,
			Version: assignment.ParentVersion,
		},
	})
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, err
	}
	if len(versions) != 1 || versions[0].Deleted || versions[0].Hash != assignment.CandidateHash {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, missingParentVersion("Relation")
	}
	return versions[0], nil
}

func (application *Application) parentClaimVersion(
	ctx context.Context,
	assignment ClaimConflict,
) (_ knowledge.KnowledgeVersion[knowledge.Claim], resultErr error) {
	view, err := application.CurrentKnowledge.OpenCurrent(ctx)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	versions, err := view.Claims().Read(ctx, []knowledge.Reference[knowledge.ClaimID]{
		{
			ID:      assignment.ParentClaimID,
			Version: assignment.ParentVersion,
		},
	})
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, err
	}
	if len(versions) != 1 || versions[0].Deleted || versions[0].Hash != assignment.CandidateHash {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, missingParentVersion("Claim")
	}
	return versions[0], nil
}

func missingParentVersion(kind string) error {
	return fmt.Errorf("%w: assigned Parent %s Version is unavailable", knowledge.ErrDataIntegrity, kind)
}
