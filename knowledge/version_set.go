package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
)

// Versions is a canonical collection of exact formal Knowledge references.
// The owning field or operation decides whether the collection is complete.
type Versions struct {
	Entities  []Reference[EntityID]
	Relations []Reference[RelationID]
	Claims    []Reference[ClaimID]
}

// Validate requires the stable order emitted by Knowledge persistence. The
// order makes one exact state deterministic in storage, events, and hashes.
func (versions Versions) Validate() error {
	if err := validateReferences(versions.Entities, ValidateEntityID); err != nil {
		return fmt.Errorf("invalid Entity Version set: %w", err)
	}
	if err := validateReferences(versions.Relations, ValidateRelationID); err != nil {
		return fmt.Errorf("invalid Relation Version set: %w", err)
	}
	if err := validateReferences(versions.Claims, ValidateClaimID); err != nil {
		return fmt.Errorf("invalid Claim Version set: %w", err)
	}
	return nil
}

func (versions Versions) Equal(other Versions) bool {
	return slices.Equal(versions.Entities, other.Entities) &&
		slices.Equal(versions.Relations, other.Relations) &&
		slices.Equal(versions.Claims, other.Claims)
}

func (versions Versions) Clone() Versions {
	return Versions{
		Entities:  append([]Reference[EntityID](nil), versions.Entities...),
		Relations: append([]Reference[RelationID](nil), versions.Relations...),
		Claims:    append([]Reference[ClaimID](nil), versions.Claims...),
	}
}

func (versions Versions) Digest() (string, error) {
	if err := versions.Validate(); err != nil {
		return "", err
	}
	hash := sha256.New()
	writeReferences(hash, "entity", versions.Entities)
	writeReferences(hash, "relation", versions.Relations)
	writeReferences(hash, "claim", versions.Claims)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (reader *Reader) CurrentVersions(
	ctx context.Context,
) (_ Versions, resultErr error) {
	view, err := reader.OpenCurrent(ctx)
	if err != nil {
		return Versions{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	result := Versions{}
	for after := EntityID(""); ; {
		page, err := view.Entities().Active(ctx, after, 256)
		if err != nil {
			return Versions{}, err
		}
		result.Entities = append(result.Entities, page.Items...)
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}
	for after := RelationID(""); ; {
		page, err := view.Relations().Active(ctx, after, 256)
		if err != nil {
			return Versions{}, err
		}
		result.Relations = append(result.Relations, page.Items...)
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}
	for after := ClaimID(""); ; {
		page, err := view.Claims().Active(ctx, after, 256)
		if err != nil {
			return Versions{}, err
		}
		result.Claims = append(result.Claims, page.Items...)
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}
	return result, nil
}
