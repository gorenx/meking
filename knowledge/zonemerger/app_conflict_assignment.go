package zonemerger

import (
	"context"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/zone"
)

type entityDifference struct {
	ChildZoneID  zone.ID
	ParentZoneID zone.ID
	Child        childEntity
	Parent       knowledge.KnowledgeVersion[knowledge.Entity]
}

type relationDifference struct {
	ChildZoneID  zone.ID
	ParentZoneID zone.ID
	Child        childRelation
	Parent       knowledge.KnowledgeVersion[knowledge.Relation]
}

type claimDifference struct {
	ChildZoneID  zone.ID
	ParentZoneID zone.ID
	Child        childClaim
	Parent       knowledge.KnowledgeVersion[knowledge.Claim]
}

func (application *Application) assignEntityConflict(
	childContext context.Context,
	difference entityDifference,
) error {
	assignment := EntityConflict{
		ChildZoneID:    difference.ChildZoneID,
		ChildEntityID:  difference.Child.Version.Knowledge.ID,
		ChildBase:      difference.Child.Version.Version,
		ParentZoneID:   difference.ParentZoneID,
		ParentEntityID: difference.Parent.Knowledge.ID,
		ParentVersion:  difference.Parent.Version,
		CandidateHash:  difference.Parent.Hash,
	}
	source, err := entityAssignmentSource(assignment)
	if err != nil {
		return err
	}
	result, err := application.Submissions.Submit(childContext, submission.Command{
		Source: source,
		Entities: []submission.Entity{
			{
				Content: knowledge.EntityContent{
					Identity: difference.Child.Content.Content.Identity,
					Aliases: append(
						[]string(nil),
						difference.Parent.Knowledge.Aliases...,
					),
					Description: difference.Parent.Knowledge.Description,
				},
			},
		},
	})
	if err != nil {
		return err
	}
	if err := requireEntityCandidate(result, assignment); err != nil {
		return err
	}
	return application.Conflicts.SaveEntityConflict(childContext, assignment)
}

func (application *Application) assignRelationConflict(
	childContext context.Context,
	difference relationDifference,
) error {
	assignment := RelationConflict{
		ChildZoneID:      difference.ChildZoneID,
		ChildRelationID:  difference.Child.Version.Knowledge.ID,
		ChildBase:        difference.Child.Version.Version,
		ParentZoneID:     difference.ParentZoneID,
		ParentRelationID: difference.Parent.Knowledge.ID,
		ParentVersion:    difference.Parent.Version,
		CandidateHash:    difference.Parent.Hash,
	}
	source, err := relationAssignmentSource(assignment)
	if err != nil {
		return err
	}
	result, err := application.Submissions.Submit(childContext, submission.Command{
		Source: source,
		Relations: []submission.Relation{
			{
				Content: knowledge.RelationContent{
					Source:      difference.Child.Content.Content.Source,
					Target:      difference.Child.Content.Content.Target,
					Type:        difference.Child.Content.Content.Type,
					Description: difference.Parent.Knowledge.Description,
				},
			},
		},
	})
	if err != nil {
		return err
	}
	if err := requireRelationCandidate(result, assignment); err != nil {
		return err
	}
	return application.Conflicts.SaveRelationConflict(childContext, assignment)
}

func (application *Application) assignClaimConflict(
	childContext context.Context,
	difference claimDifference,
) error {
	assignment := ClaimConflict{
		ChildZoneID:   difference.ChildZoneID,
		ChildClaimID:  difference.Child.Version.Knowledge.ID,
		ChildBase:     difference.Child.Version.Version,
		ParentZoneID:  difference.ParentZoneID,
		ParentClaimID: difference.Parent.Knowledge.ID,
		ParentVersion: difference.Parent.Version,
		CandidateHash: difference.Parent.Hash,
	}
	source, err := claimAssignmentSource(assignment)
	if err != nil {
		return err
	}
	result, err := application.Submissions.Submit(childContext, submission.Command{
		Source: source,
		Claims: []submission.Claim{
			{
				Content: knowledge.ClaimContent{
					Subject:     difference.Child.Content.Content.Subject,
					Type:        difference.Child.Content.Content.Type,
					Description: difference.Parent.Knowledge.Description,
				},
			},
		},
	})
	if err != nil {
		return err
	}
	if err := requireClaimCandidate(result, assignment); err != nil {
		return err
	}
	return application.Conflicts.SaveClaimConflict(childContext, assignment)
}

func requireEntityCandidate(result submission.Result, assignment EntityConflict) error {
	for _, candidates := range [][]candidate.Key[knowledge.EntityID]{
		result.OpenedCandidates.Entities,
		result.ReusedCandidates.Entities,
	} {
		for _, candidate := range candidates {
			if candidate.ID == assignment.ChildEntityID &&
				candidate.BaseVersion == assignment.ChildBase &&
				candidate.ContentHash == assignment.CandidateHash {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: Parent Entity content did not create the expected Child candidate", knowledge.ErrDataIntegrity)
}

func requireRelationCandidate(result submission.Result, assignment RelationConflict) error {
	for _, candidates := range [][]candidate.Key[knowledge.RelationID]{
		result.OpenedCandidates.Relations,
		result.ReusedCandidates.Relations,
	} {
		for _, candidate := range candidates {
			if candidate.ID == assignment.ChildRelationID &&
				candidate.BaseVersion == assignment.ChildBase &&
				candidate.ContentHash == assignment.CandidateHash {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: Parent Relation content did not create the expected Child candidate", knowledge.ErrDataIntegrity)
}

func requireClaimCandidate(result submission.Result, assignment ClaimConflict) error {
	for _, candidates := range [][]candidate.Key[knowledge.ClaimID]{
		result.OpenedCandidates.Claims,
		result.ReusedCandidates.Claims,
	} {
		for _, candidate := range candidates {
			if candidate.ID == assignment.ChildClaimID &&
				candidate.BaseVersion == assignment.ChildBase &&
				candidate.ContentHash == assignment.CandidateHash {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: Parent Claim content did not create the expected Child candidate", knowledge.ErrDataIntegrity)
}
