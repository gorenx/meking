package community

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestDetectorFindsTwoDisconnectedTriangles(t *testing.T) {
	hierarchy, err := testHierarchy(context.Background(), twoTriangleEdges(), DetectConfig{
		MaxClusterSize: 10,
		Seed:           42,
	})
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(hierarchy.Communities) != 2 {
		t.Fatalf("communities = %#v, want two roots", hierarchy.Communities)
	}

	got := make([][]string, 0, 2)
	for _, current := range hierarchy.Communities {
		if current.Level != 0 || current.ParentID != -1 || !current.Final || current.Unsplittable {
			t.Fatalf("unexpected root: %+v", current)
		}
		nodes := append([]string(nil), current.Nodes...)
		sort.Strings(nodes)
		got = append(got, nodes)
	}
	sort.Slice(got, func(i, j int) bool { return got[i][0] < got[j][0] })
	want := [][]string{{"A", "B", "C"}, {"D", "E", "F"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("community nodes = %v, want %v", got, want)
	}
}

func TestDetectorUsesLargestConnectedComponent(t *testing.T) {
	hierarchy, err := testHierarchy(context.Background(), twoTriangleEdges(), DetectConfig{
		MaxClusterSize:               10,
		UseLargestConnectedComponent: true,
		Seed:                         42,
	})
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(hierarchy.Communities) != 1 {
		t.Fatalf("communities = %#v, want one LCC root", hierarchy.Communities)
	}
	nodes := append([]string(nil), hierarchy.Communities[0].Nodes...)
	sort.Strings(nodes)
	if want := []string{"A", "B", "C"}; !reflect.DeepEqual(nodes, want) {
		t.Fatalf("LCC nodes = %v, want %v", nodes, want)
	}
}

func TestDetectorEntityIDsPreservesCanonicalLowercaseNodes(t *testing.T) {
	edges := []weightedEdge{
		{
			Source: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			Target: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			Weight: 1,
		},
	}
	hierarchy, err := testHierarchy(context.Background(), edges, DetectConfig{
		MaxClusterSize:               10,
		UseLargestConnectedComponent: true,
		Seed:                         42,
	})
	if err != nil {
		t.Fatalf("detectMemberships() error = %v", err)
	}
	want := []string{
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
	}
	if len(hierarchy.Communities) != 1 ||
		!reflect.DeepEqual(hierarchy.Communities[0].Nodes, want) {
		t.Fatalf("Community nodes = %#v, want %v", hierarchy.Communities, want)
	}
}

func TestDetectorMarksUnsplittableThresholdCommunity(t *testing.T) {
	edges := twoTriangleEdges()[:3]
	hierarchy, err := testHierarchy(context.Background(), edges, DetectConfig{
		MaxClusterSize: 3,
		Seed:           42,
	})
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(hierarchy.Communities) != 1 || !hierarchy.Communities[0].Unsplittable {
		t.Fatalf("communities = %#v, want one unsplittable root", hierarchy.Communities)
	}
}

func TestDetectorProducesDeterministicHierarchy(t *testing.T) {
	edges := ringEdges(24)
	cfg := DetectConfig{MaxClusterSize: 4, Seed: 42}
	want, err := testHierarchy(context.Background(), edges, cfg)
	if err != nil {
		t.Fatalf("first Detect() error = %v", err)
	}
	for run := 0; run < 25; run++ {
		got, err := testHierarchy(context.Background(), edges, cfg)
		if err != nil {
			t.Fatalf("Detect() run %d error = %v", run, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Detect() run %d drifted\n got: %#v\nwant: %#v", run, got, want)
		}
	}
	if len(want.Communities) <= 1 {
		t.Fatalf("ring hierarchy has no useful communities: %#v", want)
	}
}

func TestDetectorRejectsInvalidInputAndHonorsCancellation(t *testing.T) {
	_, err := testHierarchy(context.Background(), nil, DetectConfig{MaxClusterSize: 10})
	if !errors.Is(err, ErrEmptyGraph) {
		t.Fatalf("empty graph error = %v, want ErrEmptyGraph", err)
	}
	_, err = testHierarchy(context.Background(), twoTriangleEdges(), DetectConfig{})
	if !errors.Is(err, ErrInvalidMaxClusterSize) {
		t.Fatalf("invalid size error = %v, want ErrInvalidMaxClusterSize", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = testHierarchy(ctx, twoTriangleEdges(), DetectConfig{MaxClusterSize: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want context canceled", err)
	}
}

func twoTriangleEdges() []weightedEdge {
	return []weightedEdge{
		{Source: "A", Target: "B", Weight: 1},
		{Source: "A", Target: "C", Weight: 1},
		{Source: "B", Target: "C", Weight: 1},
		{Source: "D", Target: "E", Weight: 1},
		{Source: "D", Target: "F", Weight: 1},
		{Source: "E", Target: "F", Weight: 1},
	}
}

func testHierarchy(ctx context.Context, edges []weightedEdge, cfg DetectConfig) (Hierarchy, error) {
	memberships, nodes, err := detectMemberships(ctx, edges, cfg)
	if err != nil {
		return Hierarchy{}, err
	}
	hierarchy := Hierarchy{Communities: collectCommunities(memberships, nodes, cfg.MaxClusterSize)}
	if err := hierarchy.Validate(); err != nil {
		return Hierarchy{}, err
	}
	return hierarchy, nil
}

func ringEdges(size int) []weightedEdge {
	edges := make([]weightedEdge, 0, size)
	for node := 0; node < size; node++ {
		edges = append(edges, weightedEdge{
			Source: string(rune('A' + node)),
			Target: string(rune('A' + (node+1)%size)),
			Weight: 1,
		})
	}
	return edges
}
