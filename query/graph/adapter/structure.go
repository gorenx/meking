package adapter

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/community"
	querygraph "github.com/memoria-space/meking/query/graph"
)

type Structures interface {
	LoadStructureState(context.Context) (community.StructureState, error)
	LoadStructure(context.Context, community.StructureID) (community.Structure, error)
}

type CommunitySets interface {
	Load(context.Context, community.CommunitySetID) (community.CommunitySet, error)
}

type StructureReader struct {
	structures    Structures
	communitySets CommunitySets
}

func NewStructureReader(structures Structures, communitySets CommunitySets) (*StructureReader, error) {
	if structures == nil {
		return nil, errors.New("create graph Structure reader: Structures are required")
	}
	if communitySets == nil {
		return nil, errors.New("create graph Structure reader: CommunitySets are required")
	}
	return &StructureReader{structures: structures, communitySets: communitySets}, nil
}

func (reader *StructureReader) Current(ctx context.Context) (querygraph.Structure, error) {
	if reader == nil || reader.structures == nil || reader.communitySets == nil {
		return querygraph.Structure{}, errors.New("graph Structure reader is not configured")
	}
	state, err := reader.structures.LoadStructureState(ctx)
	if err != nil {
		return querygraph.Structure{}, err
	}
	if state.StructureID == "" {
		return querygraph.Structure{}, querygraph.ErrNoStructure
	}
	structure, err := reader.structures.LoadStructure(ctx, state.StructureID)
	if err != nil {
		return querygraph.Structure{}, err
	}
	set, err := reader.communitySets.Load(ctx, structure.CommunitySetID)
	if err != nil {
		return querygraph.Structure{}, err
	}
	result := querygraph.Structure{
		ID:             string(structure.ID),
		CommunitySetID: string(set.ID),
		CorporaID:      structure.CorporaID,
		Memberships:    make([]querygraph.Membership, len(set.Communities)),
	}
	for index, membership := range set.Communities {
		var parentID *string
		if membership.ParentID != nil {
			value := string(*membership.ParentID)
			parentID = &value
		}
		result.Memberships[index] = querygraph.Membership{
			ID:        string(membership.ID),
			Number:    membership.Number,
			Level:     membership.Level,
			ParentID:  parentID,
			EntityIDs: append([]string(nil), membership.EntityIDs...),
		}
	}
	return result, nil
}

var _ querygraph.StructureReader = (*StructureReader)(nil)
