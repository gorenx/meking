package basic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	querycitation "github.com/memoria-space/meking/query/internal/citation"
	queryprompt "github.com/memoria-space/meking/query/internal/prompt"
	"github.com/memoria-space/meking/query/qctx"
	querysource "github.com/memoria-space/meking/query/source"
)

// Searcher owns the complete Basic Search use case. Each call fixes Current
// Epoch once, opens that Epoch's exact TextUnit vectors, and carries the
// selection through retrieval, answer generation, streaming, and Citation audit.
type Searcher struct {
	// epochs fixes the unified publication once at request start.
	epochs querybase.EpochReader
	// vectors opens the TextUnit Namespace with that same Corpora identity.
	vectors VectorStore
	// embedder supplies both question vectors and their model identity.
	embedder QuestionEmbedder
	// tokens counts the escaped context with the fixed Corpora tokenizer.
	tokens TokenCounter
	// model generates only the final answer after evidence admission succeeds.
	model AnswerModel
	// sources supplies Corpus order before prompting and locations after citation.
	sources querysource.Reader
	// prompt is validated and copied by composition before this Searcher is shared.
	prompt string
	// config is the validated immutable Basic policy used by every request.
	config Config
}

// NewSearcher creates a reusable Basic application service. The prompt must
// contain exactly supported template fields; runtime requests cannot alter it.
func NewSearcher(
	epochs querybase.EpochReader,
	vectors VectorStore,
	embedder QuestionEmbedder,
	tokens TokenCounter,
	model AnswerModel,
	sources querysource.Reader,
	prompt string,
	config Config,
) (*Searcher, error) {
	if epochs == nil {
		return nil, errors.New("create Basic Searcher: Epoch reader is required")
	}
	if vectors == nil {
		return nil, errors.New("create Basic Searcher: VectorStore is required")
	}
	if embedder == nil || strings.TrimSpace(embedder.Model()) == "" {
		return nil, errors.New("create Basic Searcher: QuestionEmbedder with a model is required")
	}
	if tokens == nil {
		return nil, errors.New("create Basic Searcher: TokenCounter is required")
	}
	if model == nil {
		return nil, errors.New("create Basic Searcher: AnswerModel is required")
	}
	if sources == nil {
		return nil, errors.New("create Basic Searcher: Source Reader is required")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("create Basic Searcher: answer prompt is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	const contextMarker = "\x00basic-context\x00"
	const responseMarker = "\x00basic-response-type\x00"
	rendered, err := queryprompt.Render(
		prompt,
		map[string]string{
			"context_data":  contextMarker,
			"response_type": responseMarker,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("validate Basic answer prompt: %w", err)
	}
	if !strings.Contains(rendered, contextMarker) || !strings.Contains(rendered, responseMarker) {
		return nil, errors.New("create Basic Searcher: answer prompt must use context_data and response_type")
	}
	return &Searcher{
		epochs: epochs, vectors: vectors, embedder: embedder, tokens: tokens,
		model: model, sources: sources, prompt: prompt, config: config,
	}, nil
}

// Search performs one synchronous Basic retrieval, answer, and Citation audit.
func (s *Searcher) Search(ctx context.Context, request SearchRequest) (Result, error) {
	if s == nil || s.model == nil {
		return Result{}, querybase.NewInternalFailure(errors.New("Basic Searcher is not configured"))
	}
	result, err := s.prepare(ctx, request.Question)
	if err != nil {
		return result, err
	}
	modelRequest, err := s.answerRequest(request, result.Context)
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

// Stream performs the same fixed retrieval once, emits only final answer text,
// and audits the completed response. A terminal error after any emitted text is
// marked PartialOutput so delivery never retries the stream transparently.
func (s *Searcher) Stream(
	ctx context.Context,
	request SearchRequest,
	emit querybase.TextDeltaHandler,
) (Result, error) {
	if s == nil || s.model == nil {
		return Result{}, querybase.NewInternalFailure(errors.New("Basic Searcher is not configured"))
	}
	streamer, ok := s.model.(StreamAnswerModel)
	if !ok {
		return Result{}, querybase.NewInternalFailure(
			errors.New("Basic answer model does not support streaming"),
		)
	}
	if emit == nil {
		return Result{}, querybase.NewInvalidInputFailure(
			"Basic Search stream handler is required",
			nil,
		)
	}
	result, err := s.prepare(ctx, request.Question)
	if err != nil {
		return result, err
	}
	modelRequest, err := s.answerRequest(request, result.Context)
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

func (s *Searcher) prepare(
	ctx context.Context,
	question string,
) (Result, error) {
	if s == nil || s.epochs == nil || s.vectors == nil || s.embedder == nil ||
		s.tokens == nil || s.sources == nil {
		return Result{}, querybase.NewInternalFailure(
			errors.New("Basic Searcher is not configured"),
		)
	}
	if strings.TrimSpace(question) == "" {
		return Result{}, querybase.NewInvalidInputFailure(
			"Basic Search question is required",
			nil,
		)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, normalizeFailure(err)
	}
	selected, err := s.selectEpoch(ctx)
	if err != nil {
		return Result{
			EpochID: selected.ID, CorporaID: selected.CorporaID,
		}, err
	}
	result := Result{EpochID: selected.ID, CorporaID: selected.CorporaID}
	dimension, err := s.preflightTextUnitVectors(ctx, selected)
	if err != nil {
		return result, err
	}
	vector, err := s.embedder.EmbedQuestion(ctx, question)
	if err != nil {
		return result, normalizeFailure(err)
	}
	if len(vector) != dimension {
		return result, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
			"Basic question vector dimension %d does not match TextUnit vector dimension %d",
			len(vector),
			dimension,
		))
	}
	matches, err := s.retrieve(ctx, selected, vector)
	if err != nil {
		return result, err
	}
	result.Matches = append([]TextUnitMatch(nil), matches...)
	if len(matches) == 0 {
		return result, querybase.NewNoEvidenceFailure(nil)
	}
	rows, err := s.sourceRows(ctx, selected.CorporaID, matches)
	if err != nil {
		return result, err
	}
	if len(rows) == 0 {
		return result, querybase.NewPublicationIncompleteFailure(
			errors.New("Basic vector matches have no Corpus TextUnits"),
		)
	}
	builder, err := qctx.NewBuilder(fixedTokenCounter{
		counter:   s.tokens,
		corporaID: selected.CorporaID,
	})
	if err != nil {
		return result, querybase.NewInternalFailure(err)
	}
	table, err := builder.Build(ctx, qctx.Request{
		Dataset:   querybase.CitationSources,
		Columns:   []string{"text"},
		Rows:      rows,
		Delimiter: s.config.Delimiter,
		MaxTokens: s.config.MaxContextTokens,
	})
	if errors.Is(err, qctx.ErrBudgetTooSmall) {
		return result, querybase.NewBudgetExceededFailure(err)
	}
	if err != nil {
		return result, normalizeFailure(err)
	}
	result.Context = table
	if len(table.Rows) == 0 {
		return result, querybase.NewBudgetExceededFailure(nil)
	}
	return result, nil
}

func (s *Searcher) answerRequest(request SearchRequest, context qctx.Table) (AnswerModelRequest, error) {
	responseType := strings.TrimSpace(request.ResponseType)
	if responseType == "" {
		responseType = s.config.ResponseType
	}
	systemPrompt, err := queryprompt.Render(
		s.prompt, map[string]string{
			"context_data":  context.Text,
			"response_type": responseType,
		},
	)
	if err != nil {
		return AnswerModelRequest{}, querybase.NewInternalFailure(err)
	}
	return AnswerModelRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   request.Question,
	}, nil
}

func (s *Searcher) selectEpoch(
	ctx context.Context,
) (querybase.Epoch, error) {
	selected, err := s.epochs.Current(ctx)
	if errors.Is(err, querybase.ErrNoEpoch) {
		return querybase.Epoch{}, querybase.NewNoPublicationFailure(err)
	}
	if err != nil {
		return querybase.Epoch{}, normalizeFailure(err)
	}
	if strings.TrimSpace(selected.CorporaID) == "" {
		return selected, querybase.NewPublicationIncompleteFailure(
			errors.New("Basic Epoch has no Corpora"),
		)
	}
	return selected, nil
}

func (s *Searcher) retrieve(
	ctx context.Context,
	selected querybase.Epoch,
	vector []float64,
) (matches []TextUnitMatch, resultErr error) {
	reader, err := s.vectors.OpenTextUnits(ctx, selected.CorporaID)
	if err != nil {
		return nil, normalizePublicationFailure(err)
	}
	if reader == nil {
		return nil, querybase.NewPublicationIncompleteFailure(
			errors.New("Basic TextUnit vector Namespace is missing"),
		)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				querybase.NewInternalFailure(closeErr),
			)
		}
	}()
	if reader.CorporaID() != selected.CorporaID ||
		reader.Model() != s.embedder.Model() || reader.Dimension() <= 0 {
		return nil, querybase.NewPublicationIncompleteFailure(
			errors.New("Basic TextUnit vector Namespace does not match the fixed Corpora and embedding model"),
		)
	}
	if len(vector) != reader.Dimension() {
		return nil, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
			"Basic question vector dimension %d does not match TextUnit vector dimension %d",
			len(vector),
			reader.Dimension(),
		))
	}
	matches, err = reader.Search(ctx, vector, s.config.TopK)
	if err != nil {
		return nil, normalizeFailure(err)
	}
	return matches, nil
}

func (s *Searcher) preflightTextUnitVectors(
	ctx context.Context,
	selected querybase.Epoch,
) (dimension int, resultErr error) {
	reader, err := s.vectors.OpenTextUnits(ctx, selected.CorporaID)
	if err != nil {
		return 0, normalizePublicationFailure(err)
	}
	if reader == nil {
		return 0, querybase.NewPublicationIncompleteFailure(
			errors.New("Basic TextUnit vector Namespace is missing"),
		)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				querybase.NewInternalFailure(closeErr),
			)
		}
	}()
	if reader.CorporaID() != selected.CorporaID ||
		reader.Model() != s.embedder.Model() || reader.Dimension() <= 0 {
		return 0, querybase.NewPublicationIncompleteFailure(
			errors.New("Basic TextUnit vector Namespace does not match the fixed Corpora and embedding model"),
		)
	}
	return reader.Dimension(), nil
}

func (s *Searcher) sourceRows(
	ctx context.Context,
	corporaID string,
	matches []TextUnitMatch,
) ([]qctx.Row, error) {
	matched := make(map[string]struct{}, len(matches))
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		if strings.TrimSpace(match.TextUnitID) == "" {
			return nil, querybase.NewPublicationIncompleteFailure(
				errors.New("Basic vector match has no TextUnit ID"),
			)
		}
		if _, duplicate := matched[match.TextUnitID]; duplicate {
			continue
		}
		matched[match.TextUnitID] = struct{}{}
		ids = append(ids, match.TextUnitID)
	}
	sources, err := s.sources.Read(ctx, corporaID, ids)
	if err != nil {
		return nil, normalizeFailure(err)
	}
	seen := make(map[string]struct{}, len(matched))
	rows := make([]qctx.Row, 0, len(matched))
	for _, source := range sources {
		if source.CorporaID != corporaID {
			return nil, querybase.NewInternalFailure(fmt.Errorf(
				"Basic Source Reader returned Corpora %q for %q",
				source.CorporaID,
				corporaID,
			))
		}
		if _, requested := matched[source.TextUnitID]; !requested {
			return nil, querybase.NewInternalFailure(fmt.Errorf(
				"Basic Source Reader returned unrequested TextUnit %q",
				source.TextUnitID,
			))
		}
		if _, duplicate := seen[source.TextUnitID]; duplicate {
			continue
		}
		seen[source.TextUnitID] = struct{}{}
		rows = append(rows, qctx.Row{
			Values:      []string{source.Text},
			CorporaID:   corporaID,
			TextUnitIDs: []string{source.TextUnitID},
		})
	}
	return rows, nil
}

func (s *Searcher) audit(ctx context.Context, result Result) (Result, error) {
	records := make([]querybase.CitationRecord, len(result.Context.Rows))
	for index, row := range result.Context.Rows {
		records[index] = row.CitationRecord
	}
	audit, err := querycitation.Audit(ctx, result.Response, records, s.sources)
	if err != nil {
		return result, normalizeFailure(err)
	}
	result.CitationAudit = audit
	return result, nil
}

type fixedTokenCounter struct {
	// counter resolves the tokenizer for corporaID without exposing it to qctx.
	counter TokenCounter
	// corporaID is the immutable publication fixed by the owning Search call.
	corporaID string
}

func (c fixedTokenCounter) Count(ctx context.Context, text string) (int, error) {
	return c.counter.Count(ctx, c.corporaID, text)
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
