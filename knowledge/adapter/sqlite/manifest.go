package sqlite

import (
	"context"

	"github.com/memoria-space/meking/knowledge"
)

// CurrentManifest reads the exact formal Knowledge manifest from the caller's
// transaction when one is active. A state-changing operation uses the same
// transaction to publish the manifest atomically.
func (database *Database) CurrentManifest(ctx context.Context) (knowledge.Manifest, error) {
	statements, err := database.reader(ctx)
	if err != nil {
		return knowledge.Manifest{}, err
	}
	result := knowledge.Manifest{}
	entities := entityReader{statements: statements}
	for after := knowledge.EntityID(""); ; {
		page, err := entities.Active(ctx, after, 256)
		if err != nil {
			return knowledge.Manifest{}, err
		}
		result.Entities = append(result.Entities, page.Items...)
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}
	relations := relationReader{statements: statements}
	for after := knowledge.RelationID(""); ; {
		page, err := relations.Active(ctx, after, 256)
		if err != nil {
			return knowledge.Manifest{}, err
		}
		result.Relations = append(result.Relations, page.Items...)
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}
	claims := claimReader{statements: statements}
	for after := knowledge.ClaimID(""); ; {
		page, err := claims.Active(ctx, after, 256)
		if err != nil {
			return knowledge.Manifest{}, err
		}
		result.Claims = append(result.Claims, page.Items...)
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}
	return result, nil
}
