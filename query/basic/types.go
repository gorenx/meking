package basic

import (
	"context"

	querybase "github.com/memoria-space/meking/query"
	"github.com/memoria-space/meking/query/qctx"
)

// TokenCounter is Query's shared exact-Corpus tokenizer contract.
type TokenCounter = querybase.TokenCounter

// QuestionEmbedder produces one dense question vector and exposes the immutable
// model identity needed to match a published TextUnit vector Namespace.
type QuestionEmbedder interface {
	Model() string
	EmbedQuestion(ctx context.Context, question string) ([]float64, error)
}

// TextUnitMatch is one cosine-ranked TextUnit identity returned by a fixed
// vector Namespace. Duplicate IDs are retained until Basic folds them.
type TextUnitMatch struct {
	// TextUnitID is the Corpus content identity stored as the Semantic Vector ID.
	TextUnitID string
	// Score is the cosine similarity returned by the fixed TextUnit vector reader.
	Score float64
}

// TextUnitVectorReader is one request-fixed TextUnit vector view. Close releases
// its storage read view and must be called before the owning request returns.
type TextUnitVectorReader interface {
	CorporaID() string
	Model() string
	Dimension() int
	Search(ctx context.Context, vector []float64, limit int) ([]TextUnitMatch, error)
	Close() error
}

// VectorStore opens TextUnit vectors for an explicit Corpora without
// selecting current state or creating missing collections.
type VectorStore interface {
	OpenTextUnits(ctx context.Context, corporaID string) (TextUnitVectorReader, error)
}

// AnswerModelRequest is the exact two-message Basic completion input.
type AnswerModelRequest struct {
	// SystemPrompt contains the exact Sources table and requested answer format.
	SystemPrompt string
	// UserPrompt is the original non-blank question without normalization.
	UserPrompt string
}

// AnswerModel produces complete Basic answers from already-rendered evidence.
type AnswerModel interface {
	GenerateAnswer(ctx context.Context, request AnswerModelRequest) (string, error)
}

// StreamAnswerModel extends AnswerModel when final answer deltas can be emitted.
type StreamAnswerModel interface {
	AnswerModel
	StreamAnswer(
		ctx context.Context,
		request AnswerModelRequest,
		emit querybase.TextDeltaHandler,
	) (string, error)
}

// SearchRequest contains only per-request Basic choices. Retrieval and budget
// policy are fixed by the Searcher's domain-owned Config.
type SearchRequest struct {
	// Question is the required user text sent unchanged to both embedding and completion.
	Question string
	// ResponseType optionally overrides Config.ResponseType for this answer.
	ResponseType string
}

// Result is a completed Basic answer and the exact evidence and Citation
// terminal state used to produce it.
type Result struct {
	// EpochID is the positive unified publication that selected CorporaID for
	// this Basic request.
	EpochID int64
	// CorporaID is the immutable Corpus publication fixed for this request.
	CorporaID string
	// Response is the model output and is never rewritten by Citation auditing.
	Response string
	// Context is the exact escaped Sources table admitted under the hard budget.
	Context qctx.Table
	// Matches preserves the physical vector ranking returned before identity folding.
	Matches []TextUnitMatch
	// CitationAudit resolves only the request-local rows present in Context.
	CitationAudit querybase.CitationAudit
}
