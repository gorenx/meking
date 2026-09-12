package merger

import (
	"context"
	"errors"
	"testing"

	"github.com/memoria-space/meking/zone"
)

const childZoneID = "22222222-2222-4222-8222-222222222222"
const parentZoneID = "11111111-1111-4111-8111-111111111111"

type resolverStub struct{ parent zone.ID }

func (resolver resolverStub) ResolveDirectParent(context.Context, zone.ID) (zone.ID, error) {
	return resolver.parent, nil
}

type corpusStub struct {
	sourceCorporaID string
	calls           int
	err             error
	validationError error
}

func (stub *corpusStub) ValidateChildCorpora(context.Context, string) error {
	return stub.validationError
}

func (stub *corpusStub) MergeChildCorpora(_ context.Context, sourceCorporaID string) error {
	stub.calls++
	stub.sourceCorporaID = sourceCorporaID
	return stub.err
}

type knowledgeStub struct {
	calls int
	err   error
}

func (stub *knowledgeStub) MergeChildKnowledge(context.Context) error {
	stub.calls++
	return stub.err
}

func TestMergeRoutesContextChildToBothDomains(t *testing.T) {
	corpora := &corpusStub{}
	knowledge := &knowledgeStub{}
	application, err := New(Dependencies{
		Zones:  resolverStub{parent: parentZoneID},
		Corpus: corpora, Knowledge: knowledge,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := zone.RouteContext(t.Context(), parentZoneID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = zone.WithChildZone(ctx, childZoneID)
	if err != nil {
		t.Fatal(err)
	}
	input := Input{SourceCorporaID: "9"}
	if err := application.Merge(ctx, input); err != nil {
		t.Fatal(err)
	}
	if corpora.sourceCorporaID != "9" {
		t.Fatalf("Corpus SourceCorporaID = %q", corpora.sourceCorporaID)
	}
	if knowledge.calls != 1 {
		t.Fatalf("Knowledge calls = %d", knowledge.calls)
	}
}

func TestMergeStopsAfterCorpusFailure(t *testing.T) {
	failure := errors.New("Corpus failed")
	corpora := &corpusStub{err: failure}
	knowledge := &knowledgeStub{}
	application, err := New(Dependencies{
		Zones:  resolverStub{parent: parentZoneID},
		Corpus: corpora, Knowledge: knowledge,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := zone.RouteContext(t.Context(), parentZoneID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = zone.WithChildZone(ctx, childZoneID)
	if err != nil {
		t.Fatal(err)
	}
	err = application.Merge(ctx, Input{SourceCorporaID: "9"})
	if !errors.Is(err, failure) || knowledge.calls != 0 {
		t.Fatalf("Merge() error = %v, Knowledge calls = %d", err, knowledge.calls)
	}
}

func TestMergeReturnsKnowledgeFailureAfterCorpusMerge(t *testing.T) {
	failure := errors.New("Child Knowledge failed")
	corpora := &corpusStub{}
	knowledge := &knowledgeStub{err: failure}
	application, err := New(Dependencies{
		Zones:  resolverStub{parent: parentZoneID},
		Corpus: corpora, Knowledge: knowledge,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := zone.RouteContext(t.Context(), parentZoneID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = zone.WithChildZone(ctx, childZoneID)
	if err != nil {
		t.Fatal(err)
	}
	err = application.Merge(ctx, Input{SourceCorporaID: "9"})
	if !errors.Is(err, failure) {
		t.Fatalf("Merge() error = %v, want %v", err, failure)
	}
	if corpora.calls != 1 || knowledge.calls != 1 {
		t.Fatalf("failed merge called Corpus %d times and Knowledge %d times", corpora.calls, knowledge.calls)
	}
}

func TestMergeRejectsNonDirectParent(t *testing.T) {
	application, err := New(Dependencies{
		Zones:  resolverStub{parent: childZoneID},
		Corpus: &corpusStub{}, Knowledge: &knowledgeStub{},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := zone.RouteContext(t.Context(), parentZoneID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = zone.WithChildZone(ctx, childZoneID)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.Merge(ctx, Input{SourceCorporaID: "9"}); !errors.Is(err, ErrNotDirectParent) {
		t.Fatalf("Merge() error = %v", err)
	}
}
