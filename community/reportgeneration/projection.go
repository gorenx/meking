package reportgeneration

import (
	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
)

func reportInput(
	communities []community.Membership,
	knowledge KnowledgeSnapshot,
	textUnitIDs []string,
	period string,
) communityreport.Input {
	members := make(map[string]struct{})
	for _, current := range communities {
		for _, entityID := range current.EntityIDs {
			members[entityID] = struct{}{}
		}
	}
	entities := make([]communityreport.Entity, 0, len(members))
	for _, entity := range knowledge.Entities {
		if _, selected := members[entity.ID]; selected {
			entities = append(entities, entity)
		}
	}
	relations := make([]communityreport.Relation, 0, len(knowledge.Relations))
	relationIDs := make(map[string]struct{})
	for _, relation := range knowledge.Relations {
		_, sourceSelected := members[relation.SourceEntityID]
		_, targetSelected := members[relation.TargetEntityID]
		if sourceSelected && targetSelected {
			relations = append(relations, relation)
			relationIDs[relation.ID] = struct{}{}
		}
	}
	claims := make([]communityreport.Claim, 0, len(knowledge.Claims))
	for _, claim := range knowledge.Claims {
		selected := false
		switch claim.Subject.Kind {
		case communityreport.EntityClaimSubject:
			_, selected = members[claim.Subject.ID]
		case communityreport.RelationClaimSubject:
			_, selected = relationIDs[claim.Subject.ID]
		}
		if selected {
			claims = append(claims, claim)
		}
	}
	return communityreport.Input{
		Communities: append([]community.Membership(nil), communities...),
		Entities:    entities,
		Relations:   relations,
		Claims:      claims,
		TextUnitIDs: append([]string(nil), textUnitIDs...),
		Period:      period,
	}
}
