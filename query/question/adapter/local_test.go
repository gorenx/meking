package adapter

import (
	"context"
	"errors"
	"reflect"
	"testing"

	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
	"github.com/memoria-space/meking/query/qctx"
	queryquestion "github.com/memoria-space/meking/query/question"
)

type localSessionStub struct {
	evidence querylocal.Evidence
	err      error
	closeErr error
	requests []querylocal.SearchRequest
	closed   int
}

func (s *localSessionStub) Evidence(
	_ context.Context,
	request querylocal.SearchRequest,
) (querylocal.Evidence, error) {
	s.requests = append(s.requests, request)
	return s.evidence, s.err
}

func (s *localSessionStub) Close() error {
	s.closed++
	return s.closeErr
}

func TestLocalEvidenceProviderMapsHistoryAndClosesFixedSession(t *testing.T) {
	session := &localSessionStub{evidence: querylocal.Evidence{
		EpochID: 3, ReportSetID: "reports-1", CommunitySetID: "communities-1",
		CorporaID: "corpus-1",
		Context:   querylocal.Context{Text: "fixed context", Entities: qctx.Table{}},
	}}
	provider := &LocalEvidenceProvider{open: func(context.Context) (localSession, error) {
		return session, nil
	}}
	result, err := provider.Prepare(t.Context(), queryquestion.EvidenceRequest{
		CurrentQuestion: "Current?", PreviousQuestions: []string{"First?", "Second?"},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if result.EpochID != 3 || result.Context != "fixed context" || session.closed != 1 {
		t.Fatalf("result/session = %#v/%#v", result, session)
	}
	want := []querylocal.SearchRequest{{
		Question: "Current?",
		Conversation: []querybase.ConversationTurn{
			{Role: querybase.RoleUser, Content: "First?"},
			{Role: querybase.RoleUser, Content: "Second?"},
		},
	}}
	if !reflect.DeepEqual(session.requests, want) {
		t.Fatalf("requests = %#v, want %#v", session.requests, want)
	}
}

func TestLocalEvidenceProviderPreservesBuildAndCloseFailures(t *testing.T) {
	buildErr := querybase.NewNoEvidenceFailure(errors.New("none"))
	session := &localSessionStub{err: buildErr, closeErr: errors.New("close")}
	provider := &LocalEvidenceProvider{open: func(context.Context) (localSession, error) {
		return session, nil
	}}
	_, err := provider.Prepare(t.Context(), queryquestion.EvidenceRequest{CurrentQuestion: "question"})
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != querybase.FailureNoEvidence ||
		!errors.Is(err, session.closeErr) || session.closed != 1 {
		t.Fatalf("Prepare() error/closed = %#v/%d", err, session.closed)
	}
}
