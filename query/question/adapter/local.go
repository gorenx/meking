package adapter

import (
	"context"
	"errors"

	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
	queryquestion "github.com/memoria-space/meking/query/question"
)

// LocalEvidenceProvider reuses Local's complete retrieval policy while hiding
// Local session ownership and answer/Citation behavior from Question Generation.
type LocalEvidenceProvider struct {
	open func(context.Context) (localSession, error)
}

type localSession interface {
	Evidence(context.Context, querylocal.SearchRequest) (querylocal.Evidence, error)
	Close() error
}

var _ queryquestion.EvidenceProvider = (*LocalEvidenceProvider)(nil)

func NewLocalEvidenceProvider(searcher *querylocal.Searcher) (*LocalEvidenceProvider, error) {
	if searcher == nil {
		return nil, errors.New("create Question LocalEvidenceProvider: Local Searcher is required")
	}
	return &LocalEvidenceProvider{open: func(ctx context.Context) (localSession, error) {
		return searcher.Open(ctx)
	}}, nil
}

// Prepare opens one Local session, builds one context from the ordered user
// questions, and closes the fixed Entity-vector view before it returns.
func (p *LocalEvidenceProvider) Prepare(
	ctx context.Context,
	request queryquestion.EvidenceRequest,
) (_ queryquestion.Evidence, resultErr error) {
	if p == nil || p.open == nil {
		return queryquestion.Evidence{}, querybase.NewInternalFailure(
			errors.New("Question LocalEvidenceProvider is not configured"),
		)
	}
	session, err := p.open(ctx)
	if err != nil {
		return queryquestion.Evidence{}, err
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, querybase.NewInternalFailure(closeErr))
		}
	}()
	conversation := make([]querybase.ConversationTurn, len(request.PreviousQuestions))
	for index, value := range request.PreviousQuestions {
		conversation[index] = querybase.ConversationTurn{Role: querybase.RoleUser, Content: value}
	}
	evidence, err := session.Evidence(ctx, querylocal.SearchRequest{
		Question: request.CurrentQuestion, Conversation: conversation,
	})
	return queryquestion.Evidence{
		EpochID: evidence.EpochID, ReportSetID: evidence.ReportSetID,
		CommunitySetID: evidence.CommunitySetID, CorporaID: evidence.CorporaID,
		Context: evidence.Context.Text,
	}, err
}
