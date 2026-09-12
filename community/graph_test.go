package community

import (
	"reflect"
	"testing"
)

func TestPrepareEdgesNormalizesDirectionKeepsLastAndSorts(t *testing.T) {
	input := []weightedEdge{
		{Source: "B", Target: "A", Weight: 1},
		{Source: "C", Target: "D", Weight: 3},
		{Source: "A", Target: "B", Weight: 2},
	}
	want := []weightedEdge{
		{Source: "A", Target: "B", Weight: 2},
		{Source: "C", Target: "D", Weight: 3},
	}
	if got := prepareEdges(input, false); !reflect.DeepEqual(got, want) {
		t.Fatalf("prepareEdges() = %#v, want %#v", got, want)
	}
}

func TestPrepareEdgesStableLCCPreservesNodeIdentities(t *testing.T) {
	input := []weightedEdge{
		{Source: "  alice  ", Target: "bob", Weight: 1},
		{Source: "bob", Target: "carol &amp; dave", Weight: 2},
		{Source: "X", Target: "Y", Weight: 3},
	}
	want := []weightedEdge{
		{Source: "  alice  ", Target: "bob", Weight: 1},
		{Source: "bob", Target: "carol &amp; dave", Weight: 2},
	}
	if got := prepareEdges(input, true); !reflect.DeepEqual(got, want) {
		t.Fatalf("prepareEdges() = %#v, want %#v", got, want)
	}
}

func TestPrepareEdgesLCCTieUsesFirstEncounteredComponent(t *testing.T) {
	input := []weightedEdge{
		{Source: "X", Target: "Y", Weight: 1},
		{Source: "A", Target: "B", Weight: 1},
	}
	want := []weightedEdge{{Source: "X", Target: "Y", Weight: 1}}
	if got := prepareEdges(input, true); !reflect.DeepEqual(got, want) {
		t.Fatalf("prepareEdges() = %#v, want first component %#v", got, want)
	}
}

func TestPrepareEdgesStableLCCPreservesEntityIDs(t *testing.T) {
	input := []weightedEdge{
		{
			Source: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			Target: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			Weight: 1,
		},
		{
			Source: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			Target: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
			Weight: 2,
		},
		{
			Source: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
			Target: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
			Weight: 3,
		},
	}
	want := append([]weightedEdge(nil), input[:2]...)
	if got := prepareEdges(input, true); !reflect.DeepEqual(got, want) {
		t.Fatalf("prepareEdges() = %#v, want canonical lowercase IDs %#v", got, want)
	}
}
