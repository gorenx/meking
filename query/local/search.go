package local

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/memoria-space/meking/knowledge"
	querybase "github.com/memoria-space/meking/query"
	querycitation "github.com/memoria-space/meking/query/internal/citation"
	queryprompt "github.com/memoria-space/meking/query/internal/prompt"
	queryreport "github.com/memoria-space/meking/query/report"
	querysource "github.com/memoria-space/meking/query/source"
)

// Searcher owns the complete Local query use case. A request fixes one
// ReportSet and its exact Entity Version Namespace, then carries
// their exact identities through retrieval, historical Knowledge reads,
// context admission, answer generation, and Citation audit.
type Searcher struct {
	// epochs fixes the unified publication once at request start.
	epochs querybase.EpochReader
	// reports loads the exact ReportSet named by that fixed Epoch.
	reports ReportReader
	// vectors opens the Entity Namespace fixed by the Epoch Knowledge Versions.
	vectors EntityVectorStore
	// embedder supplies both query vectors and their immutable model identity.
	embedder QuestionEmbedder
	// knowledge resolves only exact ID and Version references chosen by Local.
	knowledge KnowledgeReader
	// tokens counts all context with the fixed ReportSet Corpus tokenizer.
	tokens TokenCounter
	// model generates only the final answer after evidence admission succeeds.
	model AnswerModel
	// sources supplies TextUnit text for the Sources table and Corpus occurrences
	// for final Citation audit.
	sources querysource.Reader
	// prompt is validated and copied before this Searcher is shared.
	prompt string
	// config is the validated immutable retrieval and budget policy.
	config Config
}

// NewSearcher creates a reusable Local application service. The prompt must
// contain context_data and response_type; no request can add another field.
func NewSearcher(
	epochs querybase.EpochReader,
	reports ReportReader,
	vectors EntityVectorStore,
	embedder QuestionEmbedder,
	knowledge KnowledgeReader,
	tokens TokenCounter,
	model AnswerModel,
	sources querysource.Reader,
	prompt string,
	config Config,
) (*Searcher, error) {
	switch {
	case epochs == nil:
		return nil, errors.New("create Local Searcher: Epoch reader is required")
	case reports == nil:
		return nil, errors.New("create Local Searcher: Report reader is required")
	case vectors == nil:
		return nil, errors.New("create Local Searcher: Entity vector store is required")
	case embedder == nil || strings.TrimSpace(embedder.Model()) == "":
		return nil, errors.New("create Local Searcher: QuestionEmbedder with a model is required")
	case knowledge == nil:
		return nil, errors.New("create Local Searcher: Knowledge reader is required")
	case tokens == nil:
		return nil, errors.New("create Local Searcher: TokenCounter is required")
	case model == nil:
		return nil, errors.New("create Local Searcher: AnswerModel is required")
	case sources == nil:
		return nil, errors.New("create Local Searcher: Source Reader is required")
	case strings.TrimSpace(prompt) == "":
		return nil, errors.New("create Local Searcher: answer prompt is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	const contextMarker = "\x00local-context\x00"
	const responseMarker = "\x00local-response-type\x00"
	rendered, err := queryprompt.Render(prompt, map[string]string{
		"context_data": contextMarker, "response_type": responseMarker,
	})
	if err != nil {
		return nil, fmt.Errorf("validate Local answer prompt: %w", err)
	}
	if !strings.Contains(rendered, contextMarker) || !strings.Contains(rendered, responseMarker) {
		return nil, errors.New("create Local Searcher: answer prompt must use context_data and response_type")
	}
	return &Searcher{
		epochs: epochs, reports: reports, vectors: vectors, embedder: embedder, knowledge: knowledge,
		tokens: tokens, model: model, sources: sources, prompt: prompt, config: config,
	}, nil
}

// Search performs one synchronous Local retrieval, answer, and Citation audit.
func (s *Searcher) Search(ctx context.Context, request SearchRequest) (Result, error) {
	evidence, err := s.buildEvidence(ctx, request)
	result := Result{Evidence: evidence}
	if err != nil {
		return result, err
	}
	modelRequest, err := s.answerRequest(request, evidence)
	if err != nil {
		return result, err
	}
	response, err := s.model.GenerateAnswer(ctx, modelRequest)
	if err != nil {
		return result, normalizeFailure(err)
	}
	result.Response = response
	return s.audit(ctx, result)
}

// Stream fixes and builds the same evidence once, emits only final answer
// deltas, and marks a terminal failure after emitted text as partial output.
func (s *Searcher) Stream(
	ctx context.Context,
	request SearchRequest,
	emit querybase.TextDeltaHandler,
) (Result, error) {
	if s == nil || s.model == nil {
		return Result{}, querybase.NewInternalFailure(errors.New("Local Searcher is not configured"))
	}
	streamer, ok := s.model.(StreamAnswerModel)
	if !ok {
		return Result{}, querybase.NewInternalFailure(
			errors.New("Local answer model does not support streaming"),
		)
	}
	if emit == nil {
		return Result{}, querybase.NewInvalidInputFailure(
			"Local Search stream handler is required", nil,
		)
	}
	evidence, err := s.buildEvidence(ctx, request)
	result := Result{Evidence: evidence}
	if err != nil {
		return result, err
	}
	modelRequest, err := s.answerRequest(request, evidence)
	if err != nil {
		return result, err
	}
	response, err := streamer.StreamAnswer(ctx, modelRequest, emit)
	result.Response = response
	if err != nil {
		failure := normalizeFailure(err)
		if response != "" {
			failure = querybase.MarkPartialOutput(failure)
		}
		return result, failure
	}
	result, err = s.audit(ctx, result)
	if err != nil && response != "" {
		return result, querybase.MarkPartialOutput(err)
	}
	return result, err
}

func (s *Searcher) buildEvidence(
	ctx context.Context,
	request SearchRequest,
) (result Evidence, resultErr error) {
	if s == nil || s.epochs == nil || s.reports == nil || s.vectors == nil || s.embedder == nil ||
		s.knowledge == nil || s.tokens == nil || s.model == nil || s.sources == nil {
		return Evidence{}, querybase.NewInternalFailure(
			errors.New("Local Searcher is not configured"),
		)
	}
	if err := validateSearchRequest(request); err != nil {
		return Evidence{}, querybase.NewInvalidInputFailure(err.Error(), err)
	}
	if err := ctx.Err(); err != nil {
		return Evidence{}, normalizeFailure(err)
	}
	selected, view, vectors, err := s.openViews(ctx)
	if err != nil {
		return Evidence{}, err
	}
	defer func() {
		if closeErr := vectors.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				querybase.NewInternalFailure(fmt.Errorf("close Local Entity vector reader: %w", closeErr)),
			)
		}
	}()
	return s.prepareEvidence(ctx, selected.Knowledge, request, view, vectors)
}

// prepareEvidence constructs Local evidence from views fixed by the caller. It
// neither renders the Local answer prompt nor resolves Current, so DRIFT can
// reuse the same retrieval without inheriting Local answer generation.
func (s *Searcher) prepareEvidence(
	ctx context.Context,
	versions knowledge.Manifest,
	request SearchRequest,
	view queryreport.View,
	vectors EntityVectorReader,
) (Evidence, error) {
	result := Evidence{
		EpochID:        view.EpochID,
		ReportSetID:    view.ReportSetID,
		CommunitySetID: view.CommunitySetID,
		CorporaID:      view.CorporaID,
	}
	if vectors.Dimension() == 0 {
		return result, querybase.NewNoEvidenceFailure(nil)
	}
	retrievalText := buildRetrievalText(
		request.Question,
		request.Conversation,
		s.config.ConversationTurns,
	)
	queryVector, err := s.embedder.EmbedQuestion(ctx, retrievalText)
	if err != nil {
		return result, normalizeFailure(err)
	}
	if len(queryVector) != vectors.Dimension() {
		return result, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
			"Local question vector dimension %d does not match Entity vector dimension %d",
			len(queryVector), vectors.Dimension(),
		))
	}
	matches, err := s.selectEntityMatches(ctx, versions, vectors, queryVector, request)
	if err != nil {
		return result, err
	}
	result.Matches = append([]EntityMatch(nil), matches...)
	if len(matches) == 0 {
		return result, querybase.NewNoEvidenceFailure(nil)
	}
	evidence, err := s.readEvidence(ctx, versions, view, matches)
	if err != nil {
		return result, err
	}
	contextValue, err := s.buildContext(ctx, view, request.Conversation, evidence)
	if err != nil {
		return result, err
	}
	result.Context = contextValue
	if contextRecordCount(contextValue) == 0 {
		return result, querybase.NewBudgetExceededFailure(nil)
	}
	return result, nil
}

func (s *Searcher) answerRequest(
	request SearchRequest,
	evidence Evidence,
) (AnswerModelRequest, error) {
	responseType := strings.TrimSpace(request.ResponseType)
	if responseType == "" {
		responseType = s.config.ResponseType
	}
	systemPrompt, err := queryprompt.Render(s.prompt, map[string]string{
		"context_data": evidence.Context.Text, "response_type": responseType,
	})
	if err != nil {
		return AnswerModelRequest{}, querybase.NewInternalFailure(
			fmt.Errorf("render Local answer prompt: %w", err),
		)
	}
	return AnswerModelRequest{
		SystemPrompt: systemPrompt, UserPrompt: request.Question,
	}, nil
}

// buildRetrievalText preserves Local's conversational entity-mapping behavior:
// the current question is followed by the most recent user questions in reverse
// chronological order. Assistant and system turns remain prompt history only.
func buildRetrievalText(
	question string,
	conversation []querybase.ConversationTurn,
	limit int,
) string {
	userTurns := make([]string, 0, limit)
	for index := len(conversation) - 1; index >= 0 && len(userTurns) < limit; index-- {
		if conversation[index].Role == querybase.RoleUser {
			userTurns = append(userTurns, conversation[index].Content)
		}
	}
	if len(userTurns) == 0 {
		if conversation != nil {
			return question + "\n"
		}
		return question
	}
	return question + "\n" + strings.Join(userTurns, "\n")
}

func (s *Searcher) openViews(
	ctx context.Context,
) (querybase.Epoch, queryreport.View, EntityVectorReader, error) {
	selected, err := s.epochs.Current(ctx)
	if err != nil {
		if errors.Is(err, querybase.ErrNoEpoch) {
			return querybase.Epoch{}, queryreport.View{}, nil, querybase.NewNoPublicationFailure(err)
		}
		return querybase.Epoch{}, queryreport.View{}, nil, normalizePublicationFailure(err)
	}
	view, err := s.reports.Publication(ctx, selected)
	if err != nil {
		return selected, queryreport.View{}, nil, normalizePublicationFailure(err)
	}
	vectors, err := s.openVectors(ctx, selected.Knowledge.Entities)
	if err != nil {
		return selected, view, nil, err
	}
	return selected, view, vectors, nil
}

func (s *Searcher) openVectors(
	ctx context.Context,
	entities []knowledge.Reference[knowledge.EntityID],
) (EntityVectorReader, error) {
	vectors, err := s.vectors.Open(ctx, entities)
	if err != nil {
		return nil, normalizePublicationFailure(err)
	}
	if vectors == nil {
		return nil, querybase.NewPublicationIncompleteFailure(
			errors.New("Local Entity vector reader is missing"),
		)
	}
	if strings.TrimSpace(vectors.Model()) == "" || vectors.Dimension() < 0 {
		failure := querybase.NewPublicationIncompleteFailure(
			errors.New("Local Entity vector reader metadata is incomplete"),
		)
		if closeErr := vectors.Close(); closeErr != nil {
			return nil, errors.Join(
				failure,
				querybase.NewInternalFailure(fmt.Errorf("close rejected Local Entity vector reader: %w", closeErr)),
			)
		}
		return nil, failure
	}
	if vectors.Model() != s.embedder.Model() {
		failure := querybase.NewPublicationIncompleteFailure(fmt.Errorf(
			"Local Entity vector model %q does not match question model %q",
			vectors.Model(), s.embedder.Model(),
		))
		if closeErr := vectors.Close(); closeErr != nil {
			return nil, errors.Join(
				failure,
				querybase.NewInternalFailure(fmt.Errorf("close rejected Local Entity vector reader: %w", closeErr)),
			)
		}
		return nil, failure
	}
	return vectors, nil
}

func (s *Searcher) audit(ctx context.Context, result Result) (Result, error) {
	records := result.Context.CitationRecords()
	audit, err := querycitation.Audit(ctx, result.Response, records, s.sources)
	if err != nil {
		return result, normalizeFailure(err)
	}
	result.CitationAudit = audit
	return result, nil
}

func validateSearchRequest(request SearchRequest) error {
	if err := validateEvidenceRequest(request); err != nil {
		return err
	}
	if request.ResponseType != "" && (strings.TrimSpace(request.ResponseType) == "" ||
		request.ResponseType != strings.TrimSpace(request.ResponseType)) {
		return errors.New("Local Search response type must not have surrounding whitespace")
	}
	return nil
}

func validateEvidenceRequest(request SearchRequest) error {
	if strings.TrimSpace(request.Question) == "" {
		return errors.New("Local Search question is required")
	}
	include, err := validateEntityIDs(request.IncludeEntityIDs, "included")
	if err != nil {
		return err
	}
	exclude, err := validateEntityIDs(request.ExcludeEntityIDs, "excluded")
	if err != nil {
		return err
	}
	for id := range include {
		if _, conflict := exclude[id]; conflict {
			return fmt.Errorf("Local Search Entity %q cannot be both included and excluded", id)
		}
	}
	for index, turn := range request.Conversation {
		if turn.Role != querybase.RoleUser && turn.Role != querybase.RoleAssistant &&
			turn.Role != querybase.RoleSystem {
			return fmt.Errorf("Local Search conversation turn %d has an unsupported role", index)
		}
		if strings.TrimSpace(turn.Content) == "" {
			return fmt.Errorf("Local Search conversation turn %d content is required", index)
		}
	}
	return nil
}

func validateEntityIDs(ids []string, kind string) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(ids))
	for index, id := range ids {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return nil, fmt.Errorf("Local Search %s Entity ID %d is invalid", kind, index)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("Local Search %s Entity ID %q occurs more than once", kind, id)
		}
		seen[id] = struct{}{}
	}
	return seen, nil
}

func validateEntityMatch(match EntityMatch) error {
	if strings.TrimSpace(match.ID) == "" || match.ID != strings.TrimSpace(match.ID) {
		return errors.New("Entity match ID is required without surrounding whitespace")
	}
	if match.Version == 0 {
		return fmt.Errorf("Entity match %q Version must be positive", match.ID)
	}
	if math.IsNaN(match.Score) || math.IsInf(match.Score, 0) ||
		match.Score < -1 || match.Score > 1 {
		return fmt.Errorf("Entity match %q cosine score must be between -1 and 1", match.ID)
	}
	return nil
}

func normalizePublicationFailure(err error) *querybase.Failure {
	if isCancellation(err) {
		return querybase.NewCancelledFailure(err)
	}
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return querybase.NewPublicationIncompleteFailure(err)
}

func normalizeFailure(err error) *querybase.Failure {
	if isCancellation(err) {
		return querybase.NewCancelledFailure(err)
	}
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return querybase.NewInternalFailure(err)
}

func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
