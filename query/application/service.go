package application

import (
	"context"
	"errors"

	querybase "github.com/memoria-space/meking/query"
	querybasic "github.com/memoria-space/meking/query/basic"
	querydrift "github.com/memoria-space/meking/query/drift"
	queryglobal "github.com/memoria-space/meking/query/global"
	querylocal "github.com/memoria-space/meking/query/local"
	queryquestion "github.com/memoria-space/meking/query/question"
)

type Dependencies struct {
	Basic                  *querybasic.Searcher
	Local                  *querylocal.Searcher
	Global                 *queryglobal.Searcher
	DRIFT                  *querydrift.Searcher
	Questions              *queryquestion.Generator
	GlobalMaxContextTokens int
}

type Service struct {
	basic                  *querybasic.Searcher
	local                  *querylocal.Searcher
	global                 *queryglobal.Searcher
	drift                  *querydrift.Searcher
	questions              *queryquestion.Generator
	globalMaxContextTokens int
}

func New(dependencies Dependencies) (*Service, error) {
	switch {
	case dependencies.Basic == nil:
		return nil, errors.New("create Query application: Basic Search is required")
	case dependencies.Local == nil:
		return nil, errors.New("create Query application: Local Search is required")
	case dependencies.Global == nil:
		return nil, errors.New("create Query application: Global Search is required")
	case dependencies.DRIFT == nil:
		return nil, errors.New("create Query application: DRIFT Search is required")
	case dependencies.Questions == nil:
		return nil, errors.New("create Query application: Question Generation is required")
	case dependencies.GlobalMaxContextTokens <= 0:
		return nil, errors.New("create Query application: Global context budget must be positive")
	}
	return &Service{
		basic: dependencies.Basic, local: dependencies.Local,
		global: dependencies.Global, drift: dependencies.DRIFT,
		questions:              dependencies.Questions,
		globalMaxContextTokens: dependencies.GlobalMaxContextTokens,
	}, nil
}

func (service *Service) Run(ctx context.Context, request Request) (Execution, error) {
	return service.execute(ctx, request, nil)
}

func (service *Service) Stream(
	ctx context.Context,
	request Request,
	emit querybase.TextDeltaHandler,
) (Execution, error) {
	if emit == nil {
		return Execution{Method: request.Method}, querybase.NewInvalidInputFailure(
			"Query stream handler is required",
			nil,
		)
	}
	return service.execute(ctx, request, emit)
}

func (service *Service) execute(
	ctx context.Context,
	request Request,
	emit querybase.TextDeltaHandler,
) (Execution, error) {
	if service == nil {
		return Execution{Method: request.Method}, querybase.NewInternalFailure(
			errors.New("Query application is not configured"),
		)
	}
	if err := ValidateRequest(request); err != nil {
		return Execution{Method: request.Method}, err
	}
	var execution Execution
	var err error
	switch request.Method {
	case MethodBasic:
		execution, err = service.executeBasic(ctx, request, emit)
	case MethodLocal:
		execution, err = service.executeLocal(ctx, request, emit)
	case MethodGlobal:
		execution, err = service.executeGlobal(ctx, request, emit)
	case MethodDRIFT:
		execution, err = service.executeDRIFT(ctx, request, emit)
	}
	return execution, normalizeFailure(err)
}

func (service *Service) executeBasic(
	ctx context.Context,
	request Request,
	emit querybase.TextDeltaHandler,
) (Execution, error) {
	input := querybasic.SearchRequest{Question: request.Question, ResponseType: request.ResponseType}
	var result querybasic.Result
	var err error
	if emit == nil {
		result, err = service.basic.Search(ctx, input)
	} else {
		result, err = service.basic.Stream(ctx, input, emit)
	}
	return Execution{
		Method: MethodBasic, EpochID: result.EpochID, CorporaID: result.CorporaID,
		Response: result.Response, CitationAudit: result.CitationAudit,
	}, err
}

func (service *Service) executeLocal(
	ctx context.Context,
	request Request,
	emit querybase.TextDeltaHandler,
) (Execution, error) {
	input := querylocal.SearchRequest{
		Question: request.Question, ResponseType: request.ResponseType,
		Conversation:     append([]querybase.ConversationTurn(nil), request.Conversation...),
		IncludeEntityIDs: append([]string(nil), request.IncludeEntityIDs...),
		ExcludeEntityIDs: append([]string(nil), request.ExcludeEntityIDs...),
	}
	var result querylocal.Result
	var err error
	if emit == nil {
		result, err = service.local.Search(ctx, input)
	} else {
		result, err = service.local.Stream(ctx, input, emit)
	}
	return Execution{
		Method: MethodLocal, EpochID: result.EpochID,
		ReportSetID: result.ReportSetID, CommunitySetID: result.CommunitySetID,
		CorporaID: result.CorporaID, Response: result.Response,
		CitationAudit: result.CitationAudit,
	}, err
}

func (service *Service) executeGlobal(
	ctx context.Context,
	request Request,
	emit querybase.TextDeltaHandler,
) (Execution, error) {
	level := queryglobal.DefaultCommunityLevel
	if request.CommunityLevel != nil {
		level = *request.CommunityLevel
	}
	contextConfig := queryglobal.DefaultContextConfig()
	contextConfig.MaxContextTokens = service.globalMaxContextTokens
	contextConfig.CommunityLevel = &level
	contextConfig.DynamicSelection = request.DynamicCommunitySelection
	input := queryglobal.SearchRequest{
		Question: request.Question, ResponseType: request.ResponseType,
		Conversation:  append([]querybase.ConversationTurn(nil), request.Conversation...),
		ContextConfig: contextConfig,
	}
	var result queryglobal.SearchResult
	var err error
	if emit == nil {
		result, err = service.global.Search(ctx, input)
	} else {
		result, err = service.global.Stream(ctx, input, emit)
	}
	failedBatches := 0
	for _, batch := range result.Map.Batches {
		if batch.Status != queryglobal.MapBatchSucceeded {
			failedBatches++
		}
	}
	return Execution{
		Method: MethodGlobal, EpochID: result.Context.EpochID,
		ReportSetID: result.Context.ReportSetID, CommunitySetID: result.Context.CommunitySetID,
		CorporaID: result.Context.CorporaID, Response: result.Response,
		CitationAudit: result.CitationAudit,
		Global: &GlobalStatistics{
			MapBatches: len(result.Map.Batches), FailedMapBatches: failedBatches,
			ReduceTokens:    result.ReduceContext.TokenCount,
			ReducePoints:    len(result.ReduceContext.Points),
			ReduceTruncated: result.ReduceContext.Truncated,
		},
	}, err
}

func (service *Service) executeDRIFT(
	ctx context.Context,
	request Request,
	emit querybase.TextDeltaHandler,
) (Execution, error) {
	input := querydrift.SearchRequest{Question: request.Question, ResponseType: request.ResponseType}
	var result querydrift.Result
	var err error
	if emit == nil {
		result, err = service.drift.Search(ctx, input)
	} else {
		result, err = service.drift.Stream(ctx, input, emit)
	}
	return Execution{
		Method: MethodDRIFT, EpochID: result.Traversal.Primer.Epoch.ID,
		ReportSetID:    result.Traversal.Primer.Epoch.ReportSetID,
		CommunitySetID: result.Traversal.Primer.CommunitySetID,
		CorporaID:      result.Traversal.Primer.Epoch.CorporaID,
		Response:       result.Response, CitationAudit: result.CitationAudit,
	}, err
}

func (service *Service) Suggest(
	ctx context.Context,
	request SuggestionRequest,
) (SuggestionExecution, error) {
	if service == nil {
		return SuggestionExecution{}, querybase.NewInternalFailure(
			errors.New("Query application is not configured"),
		)
	}
	input := queryquestion.Request{
		History: append([]string(nil), request.History...), Count: request.Count,
	}
	if err := queryquestion.ValidateRequest(input); err != nil {
		return SuggestionExecution{}, querybase.NewInvalidInputFailure(err.Error(), err)
	}
	result, err := service.questions.Suggest(ctx, input)
	execution := SuggestionExecution{
		Questions: append([]string(nil), result.Questions...),
		EpochID:   result.Evidence.EpochID, ReportSetID: result.Evidence.ReportSetID,
		CommunitySetID: result.Evidence.CommunitySetID,
		CorporaID:      result.Evidence.CorporaID,
	}
	return execution, normalizeFailure(err)
}

func normalizeFailure(err error) error {
	if err == nil {
		return nil
	}
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	return querybase.NewInternalFailure(err)
}
