package zonemerger

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/submission"
)

const currentPageSize = 256

type childEntity struct {
	Version knowledge.KnowledgeVersion[knowledge.Entity]
	Content submission.Entity
}

type childRelation struct {
	Version knowledge.KnowledgeVersion[knowledge.Relation]
	Content submission.Relation
}

type childClaim struct {
	Version knowledge.KnowledgeVersion[knowledge.Claim]
	Content submission.Claim
}

type childKnowledge struct {
	Entities  []childEntity
	Relations []childRelation
	Claims    []childClaim
}

func (application *Application) readChildKnowledge(
	ctx context.Context,
) (_ childKnowledge, resultErr error) {
	view, err := application.CurrentKnowledge.OpenCurrent(ctx)
	if err != nil {
		return childKnowledge{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()

	result := childKnowledge{}
	entityIdentities := make(map[knowledge.EntityID]knowledge.EntityIdentity)
	for after := knowledge.EntityID(""); ; {
		page, err := view.Entities().Current(ctx, after, currentPageSize)
		if err != nil {
			return childKnowledge{}, err
		}
		for _, version := range page.Items {
			if version.Deleted {
				continue
			}
			entity := version.Knowledge
			identity := knowledge.EntityIdentity{
				Title: entity.Title,
				Type:  entity.Type,
			}
			entityIdentities[entity.ID] = identity
			metadata, err := application.Provenance.Entity(
				ctx,
				knowledge.Reference[knowledge.EntityID]{
					ID:      entity.ID,
					Version: version.Version,
				},
			)
			if err != nil {
				return childKnowledge{}, err
			}
			result.Entities = append(result.Entities, childEntity{
				Version: version,
				Content: submission.Entity{
					Content: knowledge.EntityContent{
						Identity:    identity,
						Aliases:     append([]string(nil), entity.Aliases...),
						Description: entity.Description,
					},
					Metadata: metadata,
				},
			})
		}
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}

	relationIdentities := make(map[knowledge.RelationID]knowledge.RelationIdentity)
	for after := knowledge.RelationID(""); ; {
		page, err := view.Relations().Current(ctx, after, currentPageSize)
		if err != nil {
			return childKnowledge{}, err
		}
		for _, version := range page.Items {
			if version.Deleted {
				continue
			}
			relation := version.Knowledge
			source, sourceFound := entityIdentities[relation.SourceEntityID]
			target, targetFound := entityIdentities[relation.TargetEntityID]
			if !sourceFound || !targetFound {
				return childKnowledge{}, fmt.Errorf(
					"%w: Child Relation %q has an inactive endpoint",
					knowledge.ErrDataIntegrity,
					relation.ID,
				)
			}
			identity := knowledge.RelationIdentity{
				Source: source,
				Target: target,
				Type:   relation.Type,
			}
			relationIdentities[relation.ID] = identity
			metadata, err := application.Provenance.Relation(
				ctx,
				knowledge.Reference[knowledge.RelationID]{
					ID:      relation.ID,
					Version: version.Version,
				},
			)
			if err != nil {
				return childKnowledge{}, err
			}
			result.Relations = append(result.Relations, childRelation{
				Version: version,
				Content: submission.Relation{
					Content: knowledge.RelationContent{
						Source:      source,
						Target:      target,
						Type:        relation.Type,
						Description: relation.Description,
					},
					Metadata: metadata,
				},
			})
		}
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}

	for after := knowledge.ClaimID(""); ; {
		page, err := view.Claims().Current(ctx, after, currentPageSize)
		if err != nil {
			return childKnowledge{}, err
		}
		for _, version := range page.Items {
			if version.Deleted {
				continue
			}
			claim := version.Knowledge
			var subject knowledge.SubjectIdentity
			switch value := claim.Subject.(type) {
			case knowledge.EntityID:
				identity, found := entityIdentities[value]
				if !found {
					return childKnowledge{}, fmt.Errorf(
						"%w: Child Claim %q has an inactive Entity subject",
						knowledge.ErrDataIntegrity,
						claim.ID,
					)
				}
				subject = identity
			case knowledge.RelationID:
				identity, found := relationIdentities[value]
				if !found {
					return childKnowledge{}, fmt.Errorf(
						"%w: Child Claim %q has an inactive Relation subject",
						knowledge.ErrDataIntegrity,
						claim.ID,
					)
				}
				subject = identity
			default:
				return childKnowledge{}, fmt.Errorf(
					"%w: Child Claim %q has unsupported subject %T",
					knowledge.ErrDataIntegrity,
					claim.ID,
					claim.Subject,
				)
			}
			metadata, err := application.Provenance.Claim(
				ctx,
				knowledge.Reference[knowledge.ClaimID]{
					ID:      claim.ID,
					Version: version.Version,
				},
			)
			if err != nil {
				return childKnowledge{}, err
			}
			for _, source := range metadata {
				result.Claims = append(result.Claims, childClaim{
					Version: version,
					Content: submission.Claim{
						Content: knowledge.ClaimContent{
							Subject:     subject,
							Type:        claim.Type,
							Description: claim.Description,
						},
						Metadata: source,
					},
				})
			}
		}
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}
	return result, nil
}
