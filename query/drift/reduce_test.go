package drift

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
	"github.com/memoria-space/meking/query/qctx"
	queryreport "github.com/memoria-space/meking/query/report"
)

const testReducePrompt = "Evidence:\n{context_data}\nFormat:{response_type}"

type reduceModelStub struct {
	request  ReduceModelRequest
	response string
	err      error
	stream   bool
}

func (m *reduceModelStub) GenerateReduce(
	_ context.Context,
	request ReduceModelRequest,
) (string, error) {
	m.request = request
	return m.response, m.err
}

func (m *reduceModelStub) StreamReduce(
	_ context.Context,
	request ReduceModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	m.stream = true
	m.request = request
	if m.response != "" {
		if err := emit(m.response); err != nil {
			return m.response, err
		}
	}
	return m.response, m.err
}

func TestReducerRenumbersBranchLocalCitationsWithoutMutatingBranches(t *testing.T) {
	model := &reduceModelStub{response: "final"}
	reducer := newReducerForTest(t, model, DefaultReduceConfig(), reduceTestLimits())
	traversal := reduceTraversal()

	result, err := reducer.Reduce(t.Context(), ReduceRequest{Traversal: traversal})
	if err != nil {
		t.Fatalf("Reduce() error = %v", err)
	}
	if len(result.Context.Answers) != 3 {
		t.Fatalf("answers = %#v", result.Context.Answers)
	}
	if !strings.Contains(result.Context.Answers[0].Response, "[Data: Reports (0)]") ||
		!strings.Contains(result.Context.Answers[1].Response, "[Data: Entities (0)]") ||
		!strings.Contains(result.Context.Answers[2].Response, "[Data: Entities (1)]") {
		t.Fatalf("renumbered answers = %#v", result.Context.Answers)
	}
	if traversal.Branches[0].Answer != "first [Data: Entities (0)]" ||
		traversal.Branches[1].Answer != "second [Data: Entities (0)]" {
		t.Fatalf("Reducer mutated branch answers = %#v", traversal.Branches)
	}
	if len(result.Context.CitationRecords) != 3 ||
		result.Context.CitationRecords[0].Reference != (querybase.CitationReference{
			Dataset: querybase.CitationReports, RecordID: 0,
		}) || result.Context.CitationRecords[1].Reference.RecordID != 0 ||
		result.Context.CitationRecords[2].Reference.RecordID != 1 {
		t.Fatalf("citation ledger = %#v", result.Context.CitationRecords)
	}
	if result.Usage.Calls != traversal.Usage.Calls+1 || result.Response != "final" ||
		!strings.Contains(model.request.SystemPrompt, result.Context.Text) {
		t.Fatalf("reduction/model request = %#v / %#v", result, model.request)
	}
}

func TestReducerAdmitsWholeStablePrefixAndExcludesItsCitationRowsTogether(t *testing.T) {
	model := &reduceModelStub{response: "final"}
	config := DefaultReduceConfig()
	config.MaxContextTokens = 240
	reducer := newReducerForTest(t, model, config, reduceTestLimits())
	traversal := reduceTraversal()
	traversal.Branches[0].Answer = strings.Repeat("large branch ", 100) + "[Data: Entities (0)]"

	result, err := reducer.Reduce(t.Context(), ReduceRequest{Traversal: traversal})
	if err != nil {
		t.Fatalf("Reduce() error = %v", err)
	}
	if !result.Context.Truncated || len(result.Context.Answers) != 1 ||
		len(result.Context.CitationRecords) != 1 ||
		result.Context.CitationRecords[0].Reference.Dataset != querybase.CitationReports {
		t.Fatalf("truncated context = %#v", result.Context)
	}
}

func TestReducerTruncatesFirstPrimerAnswerWithinContextAndRequestPromptLimits(t *testing.T) {
	model := &reduceModelStub{response: "final"}
	config := DefaultReduceConfig()
	config.MaxContextTokens = 160
	limits := reduceTestLimits()
	limits.PromptTokens = 170
	reducer := newReducerForTest(t, model, config, limits)
	traversal := reduceTraversal()
	traversal.Primer.Folds[0].Response.IntermediateAnswer = strings.Repeat("primer evidence ", 100)

	result, err := reducer.Reduce(t.Context(), ReduceRequest{Traversal: traversal})
	if err != nil {
		t.Fatalf("Reduce() error = %v", err)
	}
	if !result.Context.Truncated || len(result.Context.Answers) != 1 ||
		result.Context.TokenCount > config.MaxContextTokens ||
		!strings.Contains(result.Context.Answers[0].Response, "[Data: Reports (0)]") ||
		len(result.Context.CitationRecords) != 1 ||
		result.Usage.PromptTokens > limits.PromptTokens || model.request == (ReduceModelRequest{}) {
		t.Fatalf("truncated first Reduce answer = %#v / request %#v", result, model.request)
	}
}

func TestReducerStripsBranchReferencesRejectedByItsLocalAudit(t *testing.T) {
	model := &reduceModelStub{response: "final"}
	reducer := newReducerForTest(t, model, DefaultReduceConfig(), reduceTestLimits())
	traversal := reduceTraversal()
	traversal.Branches = traversal.Branches[:1]
	traversal.Branches[0].CitationAudit.Items[0].Status = querybase.CitationInvalid
	traversal.Branches[0].CitationAudit.Items[0].InvalidReason = querybase.CitationUnresolvedSource

	result, err := reducer.Reduce(t.Context(), ReduceRequest{Traversal: traversal})
	if err != nil {
		t.Fatalf("Reduce() error = %v", err)
	}
	if strings.Contains(result.Context.Answers[1].Response, "[Data:") ||
		len(result.Context.CitationRecords) != 1 {
		t.Fatalf("rejected branch citation entered Reduce = %#v", result.Context)
	}
}

func TestReducerMarksStreamFailureAfterOutputAsPartial(t *testing.T) {
	model := &reduceModelStub{response: "prefix"}
	reducer := newReducerForTest(t, model, DefaultReduceConfig(), reduceTestLimits())
	_, err := reducer.Stream(t.Context(), ReduceRequest{Traversal: reduceTraversal()}, func(string) error {
		return errors.New("delivery stopped")
	})
	var failure *querybase.Failure
	if !errors.As(err, &failure) || !failure.PartialOutput || !model.stream {
		t.Fatalf("Stream() error/model = %#v / %#v", err, model)
	}
}

func newReducerForTest(
	t *testing.T,
	model ReduceModel,
	config ReduceConfig,
	limits RequestLimits,
) *Reducer {
	t.Helper()
	reducer, err := NewReducer(model, branchTokens{}, testReducePrompt, config, limits)
	if err != nil {
		t.Fatalf("NewReducer() error = %v", err)
	}
	return reducer
}

func reduceTestLimits() RequestLimits {
	return RequestLimits{
		ModelCalls: 100, PromptTokens: 1_000_000,
		OutputTokens: 1_000_000, Duration: time.Second,
	}
}

func reduceTraversal() TraversalResult {
	epoch := traversalEpoch()
	primer := PrimerResult{
		Epoch: epoch, Question: "global",
		Reports: []SelectedReport{{Report: queryreport.PublishedReport{
			ID: "report", Sources: queryreport.ReportSources{TextUnitIDs: []string{"report-source"}},
		}}},
		Folds: []PrimerFold{{
			ReportIDs: []string{"report"},
			Response: PrimerResponse{
				IntermediateAnswer: "primer", Score: 90,
			},
		}},
	}
	return TraversalResult{
		Primer: primer,
		Branches: []Branch{
			reduceBranch("q1", "first [Data: Entities (0)]", "entity-one"),
			reduceBranch("q2", "second [Data: Entities (0)]", "entity-two"),
		},
		Usage: Usage{Calls: 4, PromptTokens: 20, OutputTokens: 10},
	}
}

func reduceBranch(question string, answer string, textUnitID string) Branch {
	reference := querybase.CitationReference{Dataset: querybase.CitationEntities, RecordID: 0}
	return Branch{
		Question: question, Answer: answer, Score: 80, Status: BranchSucceeded,
		Evidence: querylocal.Evidence{Context: querylocal.Context{Entities: qctx.Table{
			Rows: []qctx.AcceptedRow{{CitationRecord: querybase.CitationRecord{
				Reference: reference, CorporaID: "corpus", TextUnitIDs: []string{textUnitID},
			}}},
		}}},
		CitationAudit: querybase.CitationAudit{Items: []querybase.Citation{{
			Reference: reference, Parsed: true, Status: querybase.CitationValid,
		}}},
	}
}
