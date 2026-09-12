package global

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

type ratingModelFunc func(context.Context, RatingRequest) (string, error)

func (f ratingModelFunc) RateCommunity(
	ctx context.Context,
	request RatingRequest,
) (string, error) {
	return f(ctx, request)
}

func TestDynamicSelectorDescendsAndReplacesParentsInStableOrder(t *testing.T) {
	selector := newTestDynamicSelector(t, ratingModelFunc(func(
		_ context.Context,
		request RatingRequest,
	) (string, error) {
		switch {
		case strings.Contains(request.SystemPrompt, "root-a"),
			strings.Contains(request.SystemPrompt, "root-b"),
			strings.Contains(request.SystemPrompt, "child-a"),
			strings.Contains(request.SystemPrompt, "child-b"):
			return `{"rating":5}`, nil
		default:
			return `{"rating":0}`, nil
		}
	}), DynamicSelectionConfig{
		Threshold: 1, Repeats: 1, MaxLevel: 2, MaxConcurrency: 2,
	})
	rootA := candidate("community-a", 0, nil, "root-a")
	rootB := candidate("community-b", 0, nil, "root-b")
	childA := candidate("community-a1", 1, stringPointer("community-a"), "child-a")
	childB := candidate("community-b1", 1, stringPointer("community-b"), "child-b")

	selected, err := selector.SelectCommunities(t.Context(), SelectionRequest{
		Question: "question", Reports: []ReportCandidate{rootA, rootB, childA, childB},
	})
	if err != nil {
		t.Fatalf("SelectCommunities() error = %v", err)
	}
	if !reflect.DeepEqual(selected, []string{"community-a1", "community-b1"}) {
		t.Fatalf("selected Communities = %v", selected)
	}
}

func TestDynamicSelectorFallsBackWhenRootReportsAreIrrelevant(t *testing.T) {
	selector := newTestDynamicSelector(t, ratingModelFunc(func(
		_ context.Context,
		request RatingRequest,
	) (string, error) {
		if strings.Contains(request.SystemPrompt, "match") {
			return `{"rating":3}`, nil
		}
		return `{"rating":0}`, nil
	}), DynamicSelectionConfig{
		Threshold: 1, Repeats: 1, MaxLevel: 2, MaxConcurrency: 2,
	})
	selected, err := selector.SelectCommunities(t.Context(), SelectionRequest{
		Question: "question",
		Reports: []ReportCandidate{
			candidate("community-root", 0, nil, "irrelevant root"),
			candidate("community-a", 1, stringPointer("community-root"), "irrelevant child"),
			candidate("community-b", 1, stringPointer("community-root"), "match"),
		},
	})
	if err != nil {
		t.Fatalf("SelectCommunities() error = %v", err)
	}
	if !reflect.DeepEqual(selected, []string{"community-b"}) {
		t.Fatalf("selected Communities = %v, want [community-b]", selected)
	}
}

func TestDynamicSelectorRejectsInvalidRatingAndBreaksVoteTieDownward(t *testing.T) {
	invalid := newTestDynamicSelector(t, ratingModelFunc(func(
		context.Context,
		RatingRequest,
	) (string, error) {
		return "not-json", nil
	}), DynamicSelectionConfig{
		Threshold: 1, Repeats: 1, MaxLevel: 0, MaxConcurrency: 1,
	})
	selected, err := invalid.SelectCommunities(t.Context(), SelectionRequest{
		Reports: []ReportCandidate{candidate("community-a", 0, nil, "report")},
	})
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != querybase.FailureInvalidModelResponse || len(selected) != 0 {
		t.Fatalf("invalid-rating selection/error = %v/%#v", selected, err)
	}

	responses := []string{
		`{"rating":3}`, `{"rating":2}`, `{"rating":3}`, `{"rating":2}`,
	}
	var call atomic.Int32
	tie := newTestDynamicSelector(t, ratingModelFunc(func(
		context.Context,
		RatingRequest,
	) (string, error) {
		return responses[int(call.Add(1))-1], nil
	}), DynamicSelectionConfig{
		Threshold: 3, Repeats: 4, MaxLevel: 0, MaxConcurrency: 1,
	})
	selected, err = tie.SelectCommunities(t.Context(), SelectionRequest{
		Reports: []ReportCandidate{candidate("community-a", 0, nil, "report")},
	})
	if err != nil || len(selected) != 0 || call.Load() != 4 {
		t.Fatalf("tie selection/calls/error = %v/%d/%v", selected, call.Load(), err)
	}
}

type correctingRatingModel struct {
	result     string
	correction RatingCorrection
}

func (model *correctingRatingModel) RateCommunity(context.Context, RatingRequest) (string, error) {
	return "not-json", nil
}

func (model *correctingRatingModel) CorrectRating(
	_ context.Context,
	correction RatingCorrection,
) (string, error) {
	model.correction = correction
	return model.result, nil
}

func TestDynamicSelectorReturnsRejectedRatingAndReasonForCorrection(t *testing.T) {
	model := &correctingRatingModel{result: `{"rating":3}`}
	selector := newTestDynamicSelector(t, model, DynamicSelectionConfig{
		Threshold: 1, Repeats: 1, MaxLevel: 0, MaxConcurrency: 1,
	})
	selected, err := selector.SelectCommunities(t.Context(), SelectionRequest{
		Question: "question",
		Reports:  []ReportCandidate{candidate("community-a", 0, nil, "report")},
	})
	if err != nil || !reflect.DeepEqual(selected, []string{"community-a"}) {
		t.Fatalf("selection/error = %v/%v", selected, err)
	}
	if model.correction.Result != "not-json" || !strings.Contains(model.correction.Reason, "JSON object") {
		t.Fatalf("correction = %#v", model.correction)
	}
}

// blockingRatingModel measures concurrent Report ratings while letting the
// test hold all active calls at one frontier.
type blockingRatingModel struct {
	started chan struct{}
	release chan struct{}
	active  atomic.Int32
	maximum atomic.Int32
}

func (m *blockingRatingModel) RateCommunity(
	ctx context.Context,
	_ RatingRequest,
) (string, error) {
	active := m.active.Add(1)
	defer m.active.Add(-1)
	for {
		maximum := m.maximum.Load()
		if active <= maximum || m.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	m.started <- struct{}{}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-m.release:
		return `{"rating":1}`, nil
	}
}

func TestDynamicSelectorBoundsConcurrentRatings(t *testing.T) {
	model := &blockingRatingModel{
		started: make(chan struct{}, 4), release: make(chan struct{}),
	}
	selector := newTestDynamicSelector(t, model, DynamicSelectionConfig{
		Threshold: 1, Repeats: 1, MaxLevel: 0, MaxConcurrency: 2,
	})
	reports := make([]ReportCandidate, 4)
	for index := range reports {
		id := string(rune('a' + index))
		reports[index] = candidate(id, 0, nil, id)
	}
	done := make(chan error, 1)
	go func() {
		_, err := selector.SelectCommunities(t.Context(), SelectionRequest{Reports: reports})
		done <- err
	}()
	for call := 0; call < 2; call++ {
		select {
		case <-model.started:
		case <-time.After(time.Second):
			t.Fatal("rating call did not start")
		}
	}
	select {
	case <-model.started:
		t.Fatal("third rating call started above the concurrency limit")
	case <-time.After(25 * time.Millisecond):
	}
	close(model.release)
	if err := <-done; err != nil {
		t.Fatalf("SelectCommunities() error = %v", err)
	}
	if model.maximum.Load() != 2 {
		t.Fatalf("maximum concurrent ratings = %d, want 2", model.maximum.Load())
	}
}

func TestDynamicSelectorPropagatesModelFailure(t *testing.T) {
	want := errors.New("provider unavailable")
	selector := newTestDynamicSelector(t, ratingModelFunc(func(
		context.Context,
		RatingRequest,
	) (string, error) {
		return "", want
	}), DynamicSelectionConfig{
		Threshold: 1, Repeats: 1, MaxLevel: 0, MaxConcurrency: 1,
	})
	_, err := selector.SelectCommunities(t.Context(), SelectionRequest{
		Reports: []ReportCandidate{candidate("community-a", 0, nil, "report")},
	})
	if !errors.Is(err, want) {
		t.Fatalf("SelectCommunities() error = %v, want %v", err, want)
	}
}

func newTestDynamicSelector(
	t *testing.T,
	model RatingModel,
	config DynamicSelectionConfig,
) *DynamicCommunitySelector {
	t.Helper()
	selector, err := NewDynamicCommunitySelector(
		model,
		"description={description}; question={question}",
		config,
	)
	if err != nil {
		t.Fatalf("NewDynamicCommunitySelector() error = %v", err)
	}
	return selector
}

func candidate(
	id string,
	level int,
	parentID *string,
	content string,
) ReportCandidate {
	return ReportCandidate{
		Community: queryreport.PublishedCommunity{
			ID: id, Level: level, ParentID: parentID,
		},
		Report: queryreport.PublishedReport{
			ID: "report-" + id, CommunityID: id, Summary: content, FullContent: content,
		},
	}
}

func stringPointer(value string) *string {
	return &value
}
