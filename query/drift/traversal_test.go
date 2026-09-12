package drift

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
	queryreport "github.com/memoria-space/meking/query/report"
)

const testBranchPrompt = "Context:{context_data}\nFormat:{response_type}\nGlobal:{global_query}\nFollowups:{followups}"

type branchSession struct {
	mu       sync.Mutex
	calls    []string
	failures map[string]error
	epoch    querybase.Epoch
}

func (s *branchSession) Epoch() querybase.Epoch {
	if s.epoch.ID == 0 {
		return traversalEpoch()
	}
	return s.epoch
}

func (s *branchSession) Close() error { return nil }

func (s *branchSession) Evidence(
	_ context.Context,
	request querylocal.SearchRequest,
) (querylocal.Evidence, error) {
	s.mu.Lock()
	s.calls = append(s.calls, request.Question)
	err := s.failures[request.Question]
	s.mu.Unlock()
	if err != nil {
		return querylocal.Evidence{}, err
	}
	return querylocal.Evidence{
		EpochID: 7, ReportSetID: "reports", CommunitySetID: "communities",
		CorporaID: "corpus",
		Context:   querylocal.Context{Text: "evidence " + request.Question},
	}, nil
}

type branchModelStub struct {
	mu        sync.Mutex
	requests  []BranchModelRequest
	responses map[string]string
	failures  map[string]error
}

type correctingBranchModel struct {
	branchModelStub
	correction BranchCorrection
}

func (model *correctingBranchModel) CorrectBranch(
	_ context.Context,
	correction BranchCorrection,
) (string, error) {
	model.correction = correction
	return `{"response":"fixed","score":80,"follow_up_queries":[]}`, nil
}

func (m *branchModelStub) GenerateBranch(
	_ context.Context,
	request BranchModelRequest,
) (string, error) {
	m.mu.Lock()
	m.requests = append(m.requests, request)
	response := m.responses[request.UserPrompt]
	err := m.failures[request.UserPrompt]
	m.mu.Unlock()
	return response, err
}

type branchTokens struct{}

func (branchTokens) Count(_ context.Context, _ string, text string) (int, error) {
	return len([]rune(text)), nil
}

type selectiveTokenFailure string

func (failed selectiveTokenFailure) Count(_ context.Context, _ string, text string) (int, error) {
	if text == string(failed) {
		return 0, errors.New("token count failed")
	}
	return len([]rune(text)), nil
}

func TestTraversalUsesStableBFSAndKeepsEveryParentEdge(t *testing.T) {
	primer := PrimerResult{
		Epoch:    querybase.Epoch{ID: 7, CorporaID: "corpus", ReportSetID: "reports"},
		Question: "global",
		Reports: []SelectedReport{{Report: queryreport.PublishedReport{
			ID: "report", Findings: []queryreport.ReportFinding{{Summary: "finding"}},
			Sources: queryreport.ReportSources{TextUnitIDs: []string{"global-source"}},
		}}},
		Folds: []PrimerFold{{
			ReportIDs: []string{"report"},
			Response:  PrimerResponse{FollowUpQueries: []string{"q1", "q2", "q2", "q3"}},
		}},
		FollowUpQueries: []string{"q1", "q2", "q2", "q3"},
		Usage:           Usage{Calls: 3, PromptTokens: 10, OutputTokens: 5},
	}
	model := &branchModelStub{responses: map[string]string{
		"q1": `{"response":"answer one","score":90,"follow_up_queries":["q4"]}`,
		"q2": `{"response":"answer two","score":70,"follow_up_queries":["q5"]}`,
		"q3": `{"response":"answer three","score":50,"follow_up_queries":["q4"]}`,
		"q4": `{"response":"answer four","score":40,"follow_up_queries":[]}`,
	}}
	config := traversalTestConfig()
	config.BatchSize = 2
	config.MaxBranches = 4
	traversal := newTraversalForTest(t, model, branchTokens{}, config)
	session := &branchSession{}

	result, err := traversal.Explore(t.Context(), TraversalRequest{Primer: primer}, session)
	if err != nil {
		t.Fatalf("Explore() error = %v", err)
	}
	wantQuestions := []string{"q1", "q2", "q3", "q4"}
	gotQuestions := make([]string, len(result.Branches))
	for index, branch := range result.Branches {
		gotQuestions[index] = branch.Question
	}
	if !reflect.DeepEqual(gotQuestions, wantQuestions) {
		t.Fatalf("BFS questions = %v, want %v", gotQuestions, wantQuestions)
	}
	wantEdges := []Edge{
		{Parent: "global", Child: "q1"},
		{Parent: "global", Child: "q2"},
		{Parent: "global", Child: "q2"},
		{Parent: "global", Child: "q3"},
		{Parent: "q1", Child: "q4"},
		{Parent: "q3", Child: "q4"},
	}
	if !reflect.DeepEqual(result.Edges, wantEdges) {
		t.Fatalf("Edges = %#v, want %#v", result.Edges, wantEdges)
	}
	sort.Strings(session.calls)
	if !reflect.DeepEqual(session.calls, []string{"q1", "q2", "q3", "q4"}) {
		t.Fatalf("Local evidence order = %v", session.calls)
	}
	for index := 0; index < 4; index++ {
		if result.Branches[index].Status != BranchSucceeded ||
			result.Branches[index].Evidence.Context.Text != "evidence "+result.Branches[index].Question {
			t.Fatalf("Branch %d = %#v", index, result.Branches[index])
		}
	}
	if result.Usage.Calls != primer.Usage.Calls+4 {
		t.Fatalf("Usage = %#v", result.Usage)
	}

	primer.Reports[0].Report.Findings[0].Summary = "mutated"
	primer.Reports[0].Report.Sources.TextUnitIDs[0] = "mutated"
	primer.Folds[0].ReportIDs[0] = "mutated"
	primer.Folds[0].Response.FollowUpQueries[0] = "mutated"
	primer.FollowUpQueries[0] = "mutated"
	if result.Primer.Reports[0].Report.Findings[0].Summary != "finding" ||
		result.Primer.Reports[0].Report.Sources.TextUnitIDs[0] != "global-source" ||
		result.Primer.Folds[0].ReportIDs[0] != "report" ||
		result.Primer.Folds[0].Response.FollowUpQueries[0] != "q1" ||
		result.Primer.FollowUpQueries[0] != "q1" {
		t.Fatalf("Traversal result aliases Primer input = %#v", result.Primer)
	}
}

func TestTraversalIsolatesLocalAndStructuredResponseFailures(t *testing.T) {
	model := &branchModelStub{responses: map[string]string{
		"invalid": `{"response":"answer","score":50,"follow_up_queries":["one","two","three"]}`,
		"good":    `{"response":"answer","score":80,"follow_up_queries":[]}`,
	}}
	config := traversalTestConfig()
	config.FollowUpLimit = 2
	traversal := newTraversalForTest(t, model, branchTokens{}, config)
	session := &branchSession{failures: map[string]error{
		"missing": querybase.NewNoEvidenceFailure(errors.New("missing")),
	}}
	result, err := traversal.Explore(t.Context(), TraversalRequest{Primer: traversalPrimer(
		"missing", "invalid", "good",
	)}, session)
	if err != nil {
		t.Fatalf("Explore() error = %v", err)
	}
	assertBranchFailure(t, result.Branches[0], querybase.FailureNoEvidence)
	assertBranchFailure(t, result.Branches[1], querybase.FailureInvalidModelResponse)
	if result.Branches[2].Status != BranchSucceeded || result.Branches[2].Answer != "answer" {
		t.Fatalf("successful branch = %#v", result.Branches[2])
	}
	if len(model.requests) != 2 || model.requests[0].UserPrompt != "invalid" ||
		model.requests[1].UserPrompt != "good" {
		t.Fatalf("model requests = %#v", model.requests)
	}
}

func TestTraversalReturnsRejectedResultAndReasonForCorrection(t *testing.T) {
	model := &correctingBranchModel{branchModelStub: branchModelStub{
		responses: map[string]string{"invalid": `{"response":"answer","score":101,"follow_up_queries":[]}`},
	}}
	traversal := newTraversalForTest(t, model, branchTokens{}, traversalTestConfig())
	result, err := traversal.Explore(
		t.Context(), TraversalRequest{Primer: traversalPrimer("invalid")}, &branchSession{},
	)
	if err != nil || len(result.Branches) != 1 || result.Branches[0].Status != BranchSucceeded ||
		result.Branches[0].Answer != "fixed" || result.Branches[0].Usage.Calls != 2 {
		t.Fatalf("Explore() result/error = %#v/%v", result, err)
	}
	if !strings.Contains(model.correction.Result, `"score":101`) ||
		!strings.Contains(model.correction.Reason, "between 0 and 100") {
		t.Fatalf("correction = %#v", model.correction)
	}
}

func TestTraversalKeepsBranchModelFailureWhenItsResponseCannotBeCounted(t *testing.T) {
	model := &branchModelStub{
		responses: map[string]string{
			"failed": "uncountable",
			"good":   `{"response":"answer","score":80,"follow_up_queries":[]}`,
		},
		failures: map[string]error{
			"failed": querybase.NewProviderFailure(false, 400, errors.New("provider rejected request")),
		},
	}
	traversal := newTraversalForTest(
		t, model, selectiveTokenFailure("uncountable"), traversalTestConfig(),
	)

	result, err := traversal.Explore(
		t.Context(), TraversalRequest{Primer: traversalPrimer("failed", "good")}, &branchSession{},
	)
	if err != nil {
		t.Fatalf("Explore() error = %v", err)
	}
	assertBranchFailure(t, result.Branches[0], querybase.FailureProviderPermanent)
	if result.Branches[1].Status != BranchSucceeded {
		t.Fatalf("successful branch = %#v", result.Branches[1])
	}
}

func TestTraversalRejectsLocalSessionFromAnotherEpoch(t *testing.T) {
	traversal := newTraversalForTest(
		t, &branchModelStub{}, branchTokens{}, traversalTestConfig(),
	)
	session := &branchSession{epoch: querybase.Epoch{
		ID: 8, CorporaID: "other-corpus", ReportSetID: "other-reports",
	}}
	_, err := traversal.Explore(
		t.Context(), TraversalRequest{Primer: traversalPrimer("q1")}, session,
	)
	assertTraversalFailure(t, err, querybase.FailureInternal)
	if len(session.calls) != 0 {
		t.Fatalf("mismatched Session read Local evidence: %v", session.calls)
	}
}

func TestTraversalEnforcesModelPromptOutputAndDepthBounds(t *testing.T) {
	t.Run("exact branch count", func(t *testing.T) {
		model := &branchModelStub{responses: map[string]string{
			"q1": `{"response":"one","score":50,"follow_up_queries":[]}`,
		}}
		config := traversalTestConfig()
		config.MaxBranches = 1
		traversal := newTraversalForTest(t, model, branchTokens{}, config)
		result, err := traversal.Explore(
			t.Context(), TraversalRequest{Primer: traversalPrimer("q1")}, &branchSession{},
		)
		if err != nil {
			t.Fatalf("Explore() error = %v", err)
		}
		if len(result.Branches) != 1 || result.Branches[0].Status != BranchSucceeded {
			t.Fatalf("exact branch limit = %#v", result)
		}
	})

	t.Run("model calls", func(t *testing.T) {
		model := &branchModelStub{responses: map[string]string{
			"q1": `{"response":"one","score":50,"follow_up_queries":[]}`,
		}}
		config := traversalTestConfig()
		config.Limits.ModelCalls = 2
		traversal := newTraversalForTest(t, model, branchTokens{}, config)
		primer := traversalPrimer("q1", "q2")
		primer.Usage.Calls = 1
		result, err := traversal.Explore(t.Context(), TraversalRequest{Primer: primer}, &branchSession{})
		if err != nil {
			t.Fatalf("Explore() error = %v", err)
		}
		if len(model.requests) != 1 || len(result.Branches) != 1 ||
			result.Branches[0].Status != BranchSucceeded {
			t.Fatalf("model-call limit = requests:%d result:%#v", len(model.requests), result)
		}
	})

	t.Run("prompt tokens", func(t *testing.T) {
		model := &branchModelStub{}
		config := traversalTestConfig()
		config.Limits.PromptTokens = 1
		traversal := newTraversalForTest(t, model, branchTokens{}, config)
		result, err := traversal.Explore(
			t.Context(), TraversalRequest{Primer: traversalPrimer("q1")}, &branchSession{},
		)
		if err != nil {
			t.Fatalf("Explore() error = %v", err)
		}
		if len(model.requests) != 0 || len(result.Branches) != 0 {
			t.Fatalf("prompt limit = requests:%d result:%#v", len(model.requests), result)
		}
	})

	t.Run("output tokens", func(t *testing.T) {
		model := &branchModelStub{responses: map[string]string{
			"q1": `{"response":"a deliberately long answer","score":50,"follow_up_queries":[]}`,
		}}
		config := traversalTestConfig()
		config.MaxCompletionTokens = 20
		traversal := newTraversalForTest(t, model, branchTokens{}, config)
		result, err := traversal.Explore(
			t.Context(), TraversalRequest{Primer: traversalPrimer("q1")}, &branchSession{},
		)
		if err != nil {
			t.Fatalf("Explore() error = %v", err)
		}
		assertBranchFailure(t, result.Branches[0], querybase.FailureInvalidModelResponse)
	})

	t.Run("depth", func(t *testing.T) {
		model := &branchModelStub{responses: map[string]string{
			"q1": `{"response":"one","score":50,"follow_up_queries":["q2"]}`,
		}}
		config := traversalTestConfig()
		config.MaxDepth = 1
		traversal := newTraversalForTest(t, model, branchTokens{}, config)
		result, err := traversal.Explore(
			t.Context(), TraversalRequest{Primer: traversalPrimer("q1")}, &branchSession{},
		)
		if err != nil {
			t.Fatalf("Explore() error = %v", err)
		}
		if len(result.Branches) != 1 || result.Branches[0].Question != "q1" ||
			result.Branches[0].Status != BranchSucceeded || len(model.requests) != 1 {
			t.Fatalf("depth limit = %#v / requests:%#v", result, model.requests)
		}
	})
}

type boundedBranchModel struct {
	started chan string
	release chan struct{}
	mu      sync.Mutex
	active  int
	maximum int
}

type boundedEvidenceSession struct {
	started chan string
	release chan struct{}
	mu      sync.Mutex
	active  int
	maximum int
}

func (s *boundedEvidenceSession) Epoch() querybase.Epoch { return traversalEpoch() }
func (s *boundedEvidenceSession) Close() error           { return nil }

func (s *boundedEvidenceSession) Evidence(
	ctx context.Context,
	request querylocal.SearchRequest,
) (querylocal.Evidence, error) {
	s.mu.Lock()
	s.active++
	s.maximum = max(s.maximum, s.active)
	s.mu.Unlock()
	s.started <- request.Question
	select {
	case <-s.release:
	case <-ctx.Done():
		s.mu.Lock()
		s.active--
		s.mu.Unlock()
		return querylocal.Evidence{}, ctx.Err()
	}
	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	return querylocal.Evidence{
		EpochID: 7, ReportSetID: "reports", CommunitySetID: "communities",
		CorporaID: "corpus",
		Context:   querylocal.Context{Text: "evidence " + request.Question},
	}, nil
}

func TestTraversalBoundsConcurrentEvidenceBuilds(t *testing.T) {
	session := &boundedEvidenceSession{
		started: make(chan string, 4),
		release: make(chan struct{}, 4),
	}
	model := &branchModelStub{responses: map[string]string{
		"q1": `{"response":"one","score":50,"follow_up_queries":[]}`,
		"q2": `{"response":"two","score":50,"follow_up_queries":[]}`,
		"q3": `{"response":"three","score":50,"follow_up_queries":[]}`,
		"q4": `{"response":"four","score":50,"follow_up_queries":[]}`,
	}}
	config := traversalTestConfig()
	config.BatchSize = 4
	config.MaxConcurrency = 2
	traversal := newTraversalForTest(t, model, branchTokens{}, config)
	done := make(chan error, 1)
	go func() {
		_, err := traversal.Explore(
			t.Context(), TraversalRequest{Primer: traversalPrimer("q1", "q2", "q3", "q4")}, session,
		)
		done <- err
	}()
	for range 2 {
		select {
		case <-session.started:
		case <-time.After(time.Second):
			t.Fatal("two evidence builds did not start")
		}
	}
	select {
	case third := <-session.started:
		t.Fatalf("third evidence build %q started before a concurrency slot was released", third)
	case <-time.After(20 * time.Millisecond):
	}
	for range 2 {
		session.release <- struct{}{}
		select {
		case <-session.started:
		case <-time.After(time.Second):
			t.Fatal("next evidence build did not start after a slot was released")
		}
	}
	session.release <- struct{}{}
	session.release <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Explore() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Explore() did not complete")
	}
	session.mu.Lock()
	maximum := session.maximum
	session.mu.Unlock()
	if maximum != 2 {
		t.Fatalf("maximum evidence concurrency = %d", maximum)
	}
}

func (m *boundedBranchModel) GenerateBranch(
	ctx context.Context,
	request BranchModelRequest,
) (string, error) {
	m.mu.Lock()
	m.active++
	m.maximum = max(m.maximum, m.active)
	m.mu.Unlock()
	m.started <- request.UserPrompt
	select {
	case <-m.release:
	case <-ctx.Done():
		m.mu.Lock()
		m.active--
		m.mu.Unlock()
		return "", ctx.Err()
	}
	m.mu.Lock()
	m.active--
	m.mu.Unlock()
	return `{"response":"answer","score":50,"follow_up_queries":[]}`, nil
}

func TestTraversalBoundsConcurrentBranchModelsAndRestoresBFSResultOrder(t *testing.T) {
	model := &boundedBranchModel{
		started: make(chan string, 4),
		release: make(chan struct{}, 4),
	}
	config := traversalTestConfig()
	config.BatchSize = 4
	config.MaxConcurrency = 2
	traversal := newTraversalForTest(t, model, branchTokens{}, config)
	type outcome struct {
		result TraversalResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := traversal.Explore(
			t.Context(), TraversalRequest{Primer: traversalPrimer("q1", "q2", "q3", "q4")},
			&branchSession{},
		)
		done <- outcome{result: result, err: err}
	}()
	for range 2 {
		select {
		case <-model.started:
		case <-time.After(time.Second):
			t.Fatal("two branch calls did not start")
		}
	}
	select {
	case third := <-model.started:
		t.Fatalf("third branch %q started before a concurrency slot was released", third)
	case <-time.After(20 * time.Millisecond):
	}
	model.release <- struct{}{}
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("third branch did not start after a slot was released")
	}
	model.release <- struct{}{}
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("fourth branch did not start after a slot was released")
	}
	model.release <- struct{}{}
	model.release <- struct{}{}
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Explore() error = %v", got.err)
		}
		for index, branch := range got.result.Branches {
			if branch.Question != "q"+string(rune('1'+index)) || branch.Status != BranchSucceeded {
				t.Fatalf("Branch %d = %#v", index, branch)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("Explore() did not complete")
	}
	model.mu.Lock()
	maximum := model.maximum
	model.mu.Unlock()
	if maximum != 2 {
		t.Fatalf("maximum concurrency = %d", maximum)
	}
}

func TestTraversalDistinguishesDurationBudgetFromCallerCancellation(t *testing.T) {
	t.Run("duration", func(t *testing.T) {
		model := &boundedBranchModel{
			started: make(chan string, 1),
			release: make(chan struct{}),
		}
		config := traversalTestConfig()
		config.Limits.Duration = 20 * time.Millisecond
		traversal := newTraversalForTest(t, model, branchTokens{}, config)
		result, err := traversal.Explore(
			t.Context(), TraversalRequest{Primer: traversalPrimer("q1")}, &branchSession{},
		)
		assertTraversalFailure(t, err, querybase.FailureBudgetExceeded)
		if len(result.Branches) != 0 {
			t.Fatalf("duration result = %#v", result)
		}
	})

	t.Run("caller cancellation", func(t *testing.T) {
		model := &boundedBranchModel{
			started: make(chan string, 1),
			release: make(chan struct{}),
		}
		traversal := newTraversalForTest(t, model, branchTokens{}, traversalTestConfig())
		ctx, cancel := context.WithCancel(t.Context())
		type outcome struct {
			result TraversalResult
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			result, err := traversal.Explore(
				ctx, TraversalRequest{Primer: traversalPrimer("q1")}, &branchSession{},
			)
			done <- outcome{result: result, err: err}
		}()
		select {
		case <-model.started:
		case <-time.After(time.Second):
			t.Fatal("branch call did not start")
		}
		cancel()
		select {
		case got := <-done:
			assertTraversalFailure(t, got.err, querybase.FailureCancelled)
			assertBranchFailure(t, got.result.Branches[0], querybase.FailureCancelled)
		case <-time.After(time.Second):
			t.Fatal("Explore() did not stop after caller cancellation")
		}
	})
}

func traversalPrimer(questions ...string) PrimerResult {
	return PrimerResult{
		Epoch:           traversalEpoch(),
		Question:        "global",
		FollowUpQueries: append([]string(nil), questions...),
	}
}

func traversalEpoch() querybase.Epoch {
	return querybase.Epoch{ID: 7, CorporaID: "corpus", ReportSetID: "reports"}
}

func traversalTestConfig() TraversalConfig {
	return TraversalConfig{
		MaxDepth: 3, BatchSize: 4, FollowUpLimit: 4, MaxBranches: 20,
		MaxConcurrency: 1, MaxCompletionTokens: 1000, ResponseType: "paragraphs",
		Limits: RequestLimits{
			ModelCalls: 100, PromptTokens: 1_000_000,
			OutputTokens: 1_000_000, Duration: time.Second,
		},
	}
}

func newTraversalForTest(
	t *testing.T,
	model BranchModel,
	tokens querybase.TokenCounter,
	config TraversalConfig,
) *Traversal {
	t.Helper()
	traversal, err := NewTraversal(model, tokens, testBranchPrompt, config)
	if err != nil {
		t.Fatalf("NewTraversal() error = %v", err)
	}
	return traversal
}

func assertBranchFailure(t *testing.T, branch Branch, want querybase.FailureCategory) {
	t.Helper()
	if branch.Status != BranchFailed || branch.Failure == nil || branch.Failure.Category != want {
		t.Fatalf("Branch = %#v, want failure %s", branch, want)
	}
}

func assertTraversalFailure(t *testing.T, err error, want querybase.FailureCategory) {
	t.Helper()
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != want {
		t.Fatalf("error = %v, want category %s", err, want)
	}
}
