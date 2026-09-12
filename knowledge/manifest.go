package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"slices"
)

// References identifies any number of exact formal Knowledge versions. It does
// not imply that the references describe the complete current Knowledge.
type References struct {
	Entities  []Reference[EntityID]
	Relations []Reference[RelationID]
	Claims    []Reference[ClaimID]
}

// Manifest identifies every active formal Knowledge version at one fixed
// boundary. Conflict alternatives are not formal versions and cannot appear in
// a Manifest.
type Manifest References

// Validate requires the stable order emitted by Knowledge persistence. The
// order makes one exact state deterministic in storage, events, and hashes.
func (references References) Validate() error {
	if err := validateReferences(references.Entities, ValidateEntityID); err != nil {
		return fmt.Errorf("invalid Entity Version set: %w", err)
	}
	if err := validateReferences(references.Relations, ValidateRelationID); err != nil {
		return fmt.Errorf("invalid Relation Version set: %w", err)
	}
	if err := validateReferences(references.Claims, ValidateClaimID); err != nil {
		return fmt.Errorf("invalid Claim Version set: %w", err)
	}
	return nil
}

func (manifest Manifest) Validate() error {
	return References(manifest).Validate()
}

func (manifest Manifest) Equal(other Manifest) bool {
	return slices.Equal(manifest.Entities, other.Entities) &&
		slices.Equal(manifest.Relations, other.Relations) &&
		slices.Equal(manifest.Claims, other.Claims)
}

func (manifest Manifest) Clone() Manifest {
	return Manifest{
		Entities:  append([]Reference[EntityID](nil), manifest.Entities...),
		Relations: append([]Reference[RelationID](nil), manifest.Relations...),
		Claims:    append([]Reference[ClaimID](nil), manifest.Claims...),
	}
}

func (manifest Manifest) Digest() (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	hash := sha256.New()
	writeReferences(hash, "entity", manifest.Entities)
	writeReferences(hash, "relation", manifest.Relations)
	writeReferences(hash, "claim", manifest.Claims)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateReferences[ID KnowledgeID](
	references []Reference[ID],
	validateID func(ID) error,
) error {
	var previous ID
	for index, reference := range references {
		if err := validateID(reference.ID); err != nil {
			return err
		}
		if reference.Version == 0 {
			return fmt.Errorf("Knowledge reference Version must be positive")
		}
		if err := ValidateVersion(reference.Version); err != nil {
			return err
		}
		if index > 0 && reference.ID <= previous {
			return fmt.Errorf("Knowledge references must be strictly ordered by ID")
		}
		previous = reference.ID
	}
	return nil
}

func writeReferences[ID KnowledgeID](writer hash.Hash, kind string, references []Reference[ID]) {
	_, _ = writer.Write([]byte(kind))
	_, _ = writer.Write([]byte{0})
	for _, reference := range references {
		_, _ = writer.Write([]byte(string(reference.ID)))
		_, _ = writer.Write([]byte{0})
		_, _ = fmt.Fprintf(writer, "%d", reference.Version)
		_, _ = writer.Write([]byte{0})
	}
}

func (reader *Reader) CurrentManifest(
	ctx context.Context,
) (_ Manifest, resultErr error) {
	view, err := reader.OpenCurrent(ctx)
	if err != nil {
		return Manifest{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	result := Manifest{}
	for after := EntityID(""); ; {
		page, err := view.Entities().Active(ctx, after, 256)
		if err != nil {
			return Manifest{}, err
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
			return Manifest{}, err
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
			return Manifest{}, err
		}
		result.Claims = append(result.Claims, page.Items...)
		if !page.HasMore {
			break
		}
		after = page.NextAfter
	}
	return result, nil
}
