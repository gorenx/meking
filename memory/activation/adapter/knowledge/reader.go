package knowledge

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/memory/activation"
)

type Reader struct{ versions knowledge.CurrentVersions }

func New(versions knowledge.CurrentVersions) (*Reader, error) {
	if versions == nil {
		return nil, errors.New("activation: current Knowledge versions are required")
	}
	return &Reader{versions: versions}, nil
}

func (reader *Reader) ReadCurrent(ctx context.Context, subject knowledge.ObjectRef) (activation.CurrentTarget, bool, error) {
	if err := activation.ValidateSubject(subject); err != nil {
		return activation.CurrentTarget{}, false, err
	}
	switch id := subject.(type) {
	case knowledge.EntityID:
		value, found, err := reader.versions.CurrentEntity(ctx, id)
		return activation.CurrentTarget{Version: value.Version, Deleted: value.Deleted}, found, err
	case knowledge.RelationID:
		value, found, err := reader.versions.CurrentRelation(ctx, id)
		return activation.CurrentTarget{Version: value.Version, Deleted: value.Deleted}, found, err
	case knowledge.ClaimID:
		value, found, err := reader.versions.CurrentClaim(ctx, id)
		return activation.CurrentTarget{Version: value.Version, Deleted: value.Deleted}, found, err
	default:
		return activation.CurrentTarget{}, false, activation.ErrInvalidObservation
	}
}
