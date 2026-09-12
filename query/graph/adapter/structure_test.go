package adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memoria-space/meking/community"
	querygraph "github.com/memoria-space/meking/query/graph"
)

func TestStructureReaderProjectsCurrentCommunityStructure(t *testing.T) {
	structureID := community.StructureID("10000000-0000-4000-8000-000000000001")
	communitySetID := community.CommunitySetID("20000000-0000-4000-8000-000000000001")
	parent := community.CommunityID("parent")
	provider := &graphStructureProvider{
		state: community.StructureState{StructureID: structureID},
		structure: community.Structure{
			ID: structureID, CommunitySetID: communitySetID, CorporaID: "corpora-1",
			CreatedAt: time.Now().UTC(),
		},
		set: community.CommunitySet{
			ID: communitySetID,
			Communities: []community.Membership{{
				ID: "child", Number: 2, Level: 1, ParentID: &parent,
				EntityIDs: []string{"entity-1"},
			}},
		},
	}
	reader, err := NewStructureReader(provider, provider)
	if err != nil {
		t.Fatalf("NewStructureReader() error = %v", err)
	}
	result, err := reader.Current(t.Context())
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if result.ID != string(structureID) || result.CommunitySetID != string(communitySetID) ||
		result.CorporaID != "corpora-1" || len(result.Memberships) != 1 ||
		result.Memberships[0].ParentID == nil || *result.Memberships[0].ParentID != string(parent) {
		t.Fatalf("Structure = %#v", result)
	}
	provider.set.Communities[0].EntityIDs[0] = "changed"
	if result.Memberships[0].EntityIDs[0] != "entity-1" {
		t.Fatalf("Structure retained provider slice: %#v", result)
	}
}

func TestStructureReaderReportsMissingCurrentStructure(t *testing.T) {
	provider := &graphStructureProvider{}
	reader, err := NewStructureReader(provider, provider)
	if err != nil {
		t.Fatalf("NewStructureReader() error = %v", err)
	}
	if _, err := reader.Current(t.Context()); !errors.Is(err, querygraph.ErrNoStructure) {
		t.Fatalf("Current() error = %v", err)
	}
}

type graphStructureProvider struct {
	state     community.StructureState
	structure community.Structure
	set       community.CommunitySet
}

func (provider *graphStructureProvider) LoadStructureState(context.Context) (community.StructureState, error) {
	return provider.state, nil
}

func (provider *graphStructureProvider) LoadStructure(context.Context, community.StructureID) (community.Structure, error) {
	return provider.structure, nil
}

func (provider *graphStructureProvider) Load(context.Context, community.CommunitySetID) (community.CommunitySet, error) {
	return provider.set, nil
}
