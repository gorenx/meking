package submission

import "github.com/memoria-space/meking/knowledge"

func entityFromVariant(
	id knowledge.EntityID,
	identity knowledge.EntityIdentity,
	variant entityVariant,
) knowledge.Entity {
	return knowledge.Entity{
		ID: id, Title: identity.Title, Type: identity.Type,
		Aliases: append([]string(nil), variant.aliases...), Description: variant.description,
	}
}

func relationFromVariant(
	id knowledge.RelationID,
	identity knowledge.RelationKey,
	variant relationVariant,
) knowledge.Relation {
	return knowledge.Relation{
		ID: id, SourceEntityID: identity.SourceEntityID, TargetEntityID: identity.TargetEntityID,
		Type: identity.Type, Description: variant.description,
	}
}

func claimFromVariant(
	id knowledge.ClaimID,
	identity knowledge.ClaimIdentity,
	variant claimVariant,
) knowledge.Claim {
	return knowledge.Claim{
		ID: id, Subject: identity.Subject, Type: identity.Type, Description: variant.description,
	}
}
