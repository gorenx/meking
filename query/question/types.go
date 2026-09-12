package question

import "context"

const (
	DefaultCount = 5
	MaximumCount = 20
)

// Request carries an ordered question history into one suggestion operation.
// The last item is the current Local retrieval question and model user message;
// all preceding items become user-only conversation history in original order.
type Request struct {
	History []string
	Count   int
}

// EvidenceRequest is the minimum Local retrieval input owned by Question
// Generation. The adapter must fix its Local publication and vector view once,
// build one context, and release that view before returning.
type EvidenceRequest struct {
	CurrentQuestion   string
	PreviousQuestions []string
}

// Evidence identifies the fixed inputs and exact model-visible Local context
// reused by candidate generation. It deliberately omits Local Citation records
// because suggestions are not factual answers.
type Evidence struct {
	EpochID        int64
	ReportSetID    string
	CommunitySetID string
	CorporaID      string
	Context        string
}

// EvidenceProvider adapts Local retrieval to Question Generation without
// exposing Local sessions or allowing this aggregate to reopen Current.
type EvidenceProvider interface {
	Prepare(ctx context.Context, request EvidenceRequest) (Evidence, error)
}

// ModelRequest is the exact two-message completion input after Local evidence
// and the requested candidate count have been rendered into the system prompt.
type ModelRequest struct {
	SystemPrompt string
	UserPrompt   string
}

// Model returns one complete raw candidate list. A provider adapter may consume
// a model stream internally, but Question Generation never exposes partial lists.
type Model interface {
	Generate(ctx context.Context, request ModelRequest) (string, error)
}

// CandidateCorrection returns a rejected candidate list and its validation
// reason to the same Agent conversation contract.
type CandidateCorrection struct {
	Request ModelRequest
	Result  string
	Reason  string
}

type CandidateCorrector interface {
	CorrectCandidates(ctx context.Context, correction CandidateCorrection) (string, error)
}

// Result returns the normalized candidates and the fixed identities needed to
// explain which Local publication produced them without returning prompt data.
type Result struct {
	Questions      []string
	Evidence       Evidence
	PromptTokens   int
	ModelCallCount int
}
