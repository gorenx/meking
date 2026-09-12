package community

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/memoria-space/meking/internal/uuid"
	"github.com/memoria-space/meking/knowledge"
)

type StructureID string

func RestoreStructureID(id StructureID) (StructureID, error) {
	if !uuid.IsCanonicalV4(string(id)) {
		return "", ErrInvalidStructure
	}
	return id, nil
}

type Structure struct {
	ID             StructureID
	CommunitySetID CommunitySetID
	CorporaID      string
	Knowledge      knowledge.Manifest
	CreatedAt      time.Time
}

func NewStructure(
	communitySetID CommunitySetID,
	corporaID string,
	versions knowledge.Manifest,
) (Structure, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return Structure{}, fmt.Errorf("create Community Structure ID: %w", err)
	}
	return RestoreStructure(Structure{
		ID:             StructureID(id),
		CommunitySetID: communitySetID,
		CorporaID:      corporaID,
		Knowledge:      versions.Clone(),
		CreatedAt:      time.Now().UTC(),
	})
}

func RestoreStructure(value Structure) (Structure, error) {
	if _, err := RestoreStructureID(value.ID); err != nil {
		return Structure{}, fmt.Errorf("%w: invalid StructureID", ErrInvalidStructure)
	}
	if err := ValidateCommunitySetID(value.CommunitySetID); err != nil {
		return Structure{}, fmt.Errorf("%w: %v", ErrInvalidStructure, err)
	}
	if strings.TrimSpace(value.CorporaID) == "" || value.CorporaID != strings.TrimSpace(value.CorporaID) {
		return Structure{}, fmt.Errorf("%w: invalid CorporaID", ErrInvalidStructure)
	}
	if err := value.Knowledge.Validate(); err != nil {
		return Structure{}, fmt.Errorf("%w: invalid Knowledge Versions: %v", ErrInvalidStructure, err)
	}
	if value.CreatedAt.IsZero() {
		return Structure{}, fmt.Errorf("%w: invalid Structure boundary", ErrInvalidStructure)
	}
	value.CreatedAt = value.CreatedAt.UTC()
	value.Knowledge = value.Knowledge.Clone()
	return value, nil
}

type StructureStore interface {
	SaveStructure(ctx context.Context, value Structure) error
	LoadStructure(ctx context.Context, id StructureID) (Structure, error)
	LoadStructureAt(
		ctx context.Context,
		corporaID string,
	versions knowledge.Manifest,
	) (Structure, error)
}
