package community

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

const (
	setEntityA  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	setEntityB  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	setEntityC  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	setRelation = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
)

func TestNewCommunitySetBuildsStableMembershipIDs(t *testing.T) {
	hierarchy := Hierarchy{Communities: []Community{
		{
			ID: 0, Level: 0, ParentID: -1,
			Nodes: []string{setEntityC, setEntityA, setEntityB}, Final: false,
		},
		{
			ID: 1, Level: 1, ParentID: 0,
			Nodes: []string{setEntityB, setEntityA}, Final: true,
		},
		{
			ID: 2, Level: 1, ParentID: 0,
			Nodes: []string{setEntityC}, Final: true,
		},
	}}
	entities := []EntityReference{
		{ID: setEntityC, Version: 3},
		{ID: setEntityA, Version: 1},
		{ID: setEntityB, Version: 2},
	}
	relations := []RelationReference{{ID: setRelation, Version: 4}}
	set, err := newCommunitySet(
		"11111111-1111-4111-8111-111111111111",
		CurrentDetectorVersion,
		DefaultDetectConfig(),
		hierarchy,
		entities,
		relations,
		time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("newCommunitySet() error = %v", err)
	}
	if err := ValidateCommunitySet(set); err != nil {
		t.Fatalf("ValidateCommunitySet() error = %v", err)
	}
	if !reflect.DeepEqual(set.Entities, []EntityReference{
		{ID: setEntityA, Version: 1},
		{ID: setEntityB, Version: 2},
		{ID: setEntityC, Version: 3},
	}) {
		t.Fatalf("Entity references = %#v", set.Entities)
	}
	if !reflect.DeepEqual(
		set.Communities[1].EntityIDs,
		[]string{setEntityA, setEntityB},
	) {
		t.Fatalf("child Entity IDs = %#v", set.Communities[1].EntityIDs)
	}

	reordered := hierarchy
	reordered.Communities = append([]Community(nil), hierarchy.Communities...)
	reordered.Communities[0].Nodes = []string{setEntityB, setEntityC, setEntityA}
	other, err := newCommunitySet(
		"22222222-2222-4222-8222-222222222222",
		CurrentDetectorVersion,
		DefaultDetectConfig(),
		reordered,
		entities,
		relations,
		time.Date(2026, 7, 24, 13, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("newCommunitySet(reordered) error = %v", err)
	}
	if other.Communities[0].ID != set.Communities[0].ID {
		t.Fatalf(
			"root Community IDs = %q and %q for equal memberships",
			set.Communities[0].ID,
			other.Communities[0].ID,
		)
	}
}

func TestValidateCommunitySetRejectsMissingVersionAndMembershipDrift(t *testing.T) {
	valid, err := newCommunitySet(
		"11111111-1111-4111-8111-111111111111",
		CurrentDetectorVersion,
		DefaultDetectConfig(),
		Hierarchy{Communities: []Community{{
			ID: 0, Level: 0, ParentID: -1,
			Nodes: []string{setEntityA, setEntityB}, Final: true,
		}}},
		[]EntityReference{
			{ID: setEntityA, Version: 1},
			{ID: setEntityB, Version: 2},
		},
		[]RelationReference{{ID: setRelation, Version: 3}},
		time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("newCommunitySet() error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*CommunitySet)
	}{
		{
			name: "zero knowledge Version",
			mutate: func(set *CommunitySet) {
				set.Entities[0].Version = 0
			},
		},
		{
			name: "membership identity drift",
			mutate: func(set *CommunitySet) {
				set.Communities[0].EntityIDs = []string{setEntityA}
			},
		},
		{
			name: "missing Entity reference",
			mutate: func(set *CommunitySet) {
				set.Entities = set.Entities[:1]
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneCommunitySet(valid)
			test.mutate(&candidate)
			if err := ValidateCommunitySet(candidate); !errors.Is(err, ErrInvalidCommunitySet) {
				t.Fatalf("ValidateCommunitySet() error = %v", err)
			}
		})
	}
}

func cloneCommunitySet(set CommunitySet) CommunitySet {
	result := set
	result.Entities = append([]EntityReference(nil), set.Entities...)
	result.Relations = append([]RelationReference(nil), set.Relations...)
	result.Communities = make([]Membership, len(set.Communities))
	for index, current := range set.Communities {
		result.Communities[index] = current
		result.Communities[index].EntityIDs = append([]string(nil), current.EntityIDs...)
		if current.ParentID != nil {
			parent := *current.ParentID
			result.Communities[index].ParentID = &parent
		}
	}
	return result
}
