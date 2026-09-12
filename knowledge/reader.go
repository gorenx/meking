package knowledge

import (
	"context"
	"errors"
)

type IdentityReader interface {
	Entity(ctx context.Context, identity EntityIdentity) (KnowledgeVersion[Entity], bool, error)
	FindEntities(
		ctx context.Context,
		title string,
		entityType string,
		limit int,
	) (Page[Reference[EntityID], EntityID], error)
	Relation(ctx context.Context, key RelationKey) (KnowledgeVersion[Relation], bool, error)
	Claim(ctx context.Context, identity ClaimIdentity) (KnowledgeVersion[Claim], bool, error)
}

type VersionReader[T Knowledge, ID KnowledgeID] interface {
	Active(ctx context.Context, after ID, limit int) (Page[Reference[ID], ID], error)
	Current(ctx context.Context, after ID, limit int) (Page[KnowledgeVersion[T], ID], error)
	History(ctx context.Context, id ID, after Version, limit int) (Page[KnowledgeVersion[T], Version], error)
	Read(ctx context.Context, references []Reference[ID]) ([]KnowledgeVersion[T], error)
}

// View fixes one current database read transaction while a consumer enumerates
// formal Knowledge. Exact historical Versions remain readable by Reference.
type View interface {
	Identities() IdentityReader
	Entities() VersionReader[Entity, EntityID]
	Relations() VersionReader[Relation, RelationID]
	Claims() VersionReader[Claim, ClaimID]
	Close() error
}

type ViewSource interface {
	OpenCurrent(ctx context.Context) (View, error)
}

type Reader struct {
	source ViewSource
}

func NewReader(source ViewSource) (*Reader, error) {
	if source == nil {
		return nil, errors.New("create Knowledge Reader: View source is required")
	}
	return &Reader{source: source}, nil
}

func (reader *Reader) OpenCurrent(ctx context.Context) (View, error) {
	if reader == nil || reader.source == nil {
		return nil, errors.New("open current Knowledge View: Reader is not initialized")
	}
	return reader.source.OpenCurrent(ctx)
}
