// Package drift owns DRIFT's Epoch-fixed report Primer, bounded Local traversal,
// final reduction, and cross-branch Citation bookkeeping.
package drift

import (
	"context"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
	queryreport "github.com/memoria-space/meking/query/report"
)

// ReportReader loads the exact ReportSet named by an Epoch already fixed by
// DRIFT. It returns an isolated value and never resolves Current.
type ReportReader interface {
	Publication(ctx context.Context, selected querybase.Epoch) (queryreport.View, error)
}

// Embedder turns the generated hypothetical answer into the query vector used
// against the Epoch's report collection. Model identifies the required vector
// manifest model.
type Embedder interface {
	Model() string
	Embed(ctx context.Context, text string) ([]float64, error)
}

// ReportMatch is one cosine-ranked Report identity returned from an exact
// ReportSet vector Namespace.
type ReportMatch struct {
	ReportID string
	// Score is the finite cosine similarity in [-1,1].
	Score float64
}

// ReportVectorReader is one already-opened, request-stable view of the
// community_full_content vectors for exactly one ReportSet.
type ReportVectorReader interface {
	ReportSetID() string
	Model() string
	Dimension() int
	// Search considers only the supplied Report IDs from the fixed ReportSet.
	Search(ctx context.Context, vector []float64, limit int, reportIDs []string) ([]ReportMatch, error)
	Close() error
}

// ReportVectorStore opens report vectors by explicit ReportSet identity and
// never selects Current or creates a missing collection during Query.
type ReportVectorStore interface {
	OpenReports(ctx context.Context, reportSetID string) (ReportVectorReader, error)
}

// ModelRequest is one complete single-message DRIFT model call. Prompt is
// rendered by Primer; MaxCompletionTokens is the positive hard cap copied from
// PrimerConfig.
type ModelRequest struct {
	Prompt              string
	MaxCompletionTokens int
}

// Model supplies the two distinct model operations required before Local
// traversal. GenerateHypothetical returns text; GeneratePrimer returns one JSON
// object using the schema parsed by Primer.
type Model interface {
	GenerateHypothetical(ctx context.Context, request ModelRequest) (string, error)
	GeneratePrimer(ctx context.Context, request ModelRequest) (string, error)
}

// PrimerCorrection is one rejected structured fold result returned to the
// same Agent with the request that established its contract.
type PrimerCorrection struct {
	Request ModelRequest
	Result  string
	Reason  string
}

func (correction PrimerCorrection) Feedback() string {
	return correctionFeedback(correction.Reason, "complete corrected DRIFT Primer JSON object")
}

type PrimerCorrector interface {
	CorrectPrimer(ctx context.Context, correction PrimerCorrection) (string, error)
}

// PrimerRequest starts DRIFT's initial global report retrieval.
type PrimerRequest struct {
	Question string
}

// Usage records counts measured by Query with the fixed Corpora tokenizer.
// It is diagnostic evidence for later request-wide model and token limits, not
// a provider billing record.
type Usage struct {
	Calls        int
	PromptTokens int
	OutputTokens int
}

// PrimerResponse is one validated structured result produced from a contiguous
// fold of selected Reports.
type PrimerResponse struct {
	IntermediateAnswer string
	// Score is the inclusive 0..100 relevance score produced by the model.
	Score int
	// FollowUpQueries preserves the non-empty model order. Duplicates are retained
	// because later graph construction must keep every parent edge.
	FollowUpQueries []string
	Usage           Usage
}

// SelectedReport pairs one exact immutable Report with its vector rank. Reports
// preserve vector result order and retain their source closure for later DRIFT
// Citation bookkeeping.
type SelectedReport struct {
	Report queryreport.PublishedReport
	// Score is the finite cosine similarity returned by that ReportSet's vector
	// collection and is retained for diagnostics rather than model rendering.
	Score float64
}

// PrimerFold identifies which selected Reports produced one structured
// response. ReportIDs preserve their contiguous order in PrimerResult.Reports.
type PrimerFold struct {
	// Index is the zero-based position assigned before concurrent model calls.
	Index int
	// ReportIDs preserves the exact contiguous vector-rank slice rendered for
	// this fold, allowing diagnostics to identify its immutable evidence.
	ReportIDs []string
	Response  PrimerResponse
}

// PrimerResult is the complete initial DRIFT state handed to later traversal.
// Epoch is fixed once at entry; all other identities and content derive from
// that Epoch's exact ReportSet and Corpora.
type PrimerResult struct {
	// Epoch is copied from the single Current selection made at Search entry and
	// fixes every Report, vector, Corpus tokenizer, and later Local branch read.
	Epoch querybase.Epoch
	// CommunitySetID is copied from the exact Report view loaded by Primer. Epoch
	// names the ReportSet but does not duplicate this hierarchy identity.
	CommunitySetID string
	Question       string
	// HypotheticalAnswer is generated from the first Report template and embedded
	// for retrieval. If the model returns no text, it equals Question.
	HypotheticalAnswer string
	// Reports contains the validated Top-K vector matches in rank order.
	Reports []SelectedReport
	// Folds preserves deterministic contiguous grouping and model completion order.
	Folds []PrimerFold
	// IntermediateAnswer joins fold answers with a blank line in fold order.
	IntermediateAnswer string
	// Score is the arithmetic mean of all fold scores.
	Score float64
	// FollowUpQueries concatenates fold outputs in order without deduplication.
	FollowUpQueries []string
	// Usage includes one hypothetical call and every non-empty fold call.
	Usage Usage
}

// LocalSession supplies concurrently built Local evidence while retaining one
// Epoch, its ReportSet, and one Entity-vector image for the complete DRIFT
// traversal. Close waits for active Evidence calls and prevents new calls.
type LocalSession interface {
	Epoch() querybase.Epoch
	Evidence(ctx context.Context, request querylocal.SearchRequest) (querylocal.Evidence, error)
	Close() error
}

// LocalSessionOpener creates the Local evidence view for the Epoch already
// fixed by Primer; it never selects Current independently.
type LocalSessionOpener interface {
	Open(ctx context.Context, selected querybase.Epoch) (LocalSession, error)
}

// BranchModelRequest is the two-message structured completion for one Local
// evidence branch.
type BranchModelRequest struct {
	SystemPrompt        string
	UserPrompt          string
	MaxCompletionTokens int
}

// BranchModel produces one JSON response, score, and follow-up list from Local
// evidence already fixed by the supplied session.
type BranchModel interface {
	GenerateBranch(ctx context.Context, request BranchModelRequest) (string, error)
}

// BranchCorrection is one rejected structured branch result returned to the
// same Agent with the request that established its contract.
type BranchCorrection struct {
	Request BranchModelRequest
	Result  string
	Reason  string
}

func (correction BranchCorrection) Feedback() string {
	return correctionFeedback(correction.Reason, "complete corrected DRIFT branch JSON object")
}

type BranchCorrector interface {
	CorrectBranch(ctx context.Context, correction BranchCorrection) (string, error)
}

func correctionFeedback(reason, expected string) string {
	return "Your previous result was rejected.\n" +
		"Reason:\n" + strings.TrimSpace(reason) + "\n" +
		"Return the " + expected + " only."
}

// TraversalRequest continues one completed Primer through Local branches.
type TraversalRequest struct {
	Primer       PrimerResult
	ResponseType string
}

// BranchStatus is the request-local execution state of one unique question.
type BranchStatus string

const (
	BranchPending   BranchStatus = "pending"
	BranchSucceeded BranchStatus = "succeeded"
	BranchFailed    BranchStatus = "failed"
)

// Branch is one unique Local question in stable BFS discovery order. Repeated
// questions share this value while Edge retains every parent occurrence.
type Branch struct {
	Question string
	// Depth starts at one because the Primer question is the graph root at zero.
	Depth  int
	Answer string
	// Score is the inclusive 0..100 relevance score returned by the branch model.
	Score int
	// FollowUpQueries preserves model order and duplicates after whitespace validation.
	FollowUpQueries []string
	Status          BranchStatus
	Failure         *querybase.Failure
	Evidence        querylocal.Evidence
	CitationAudit   querybase.CitationAudit
	Usage           Usage
}

// Edge preserves one generated parent-child occurrence. Parent and Child use
// exact trimmed question text; duplicate edges are intentional.
type Edge struct {
	Parent string
	Child  string
}

// TraversalResult keeps global Report evidence in Primer and Local evidence in
// Branches so their request-local record numbers cannot be interpreted as one
// namespace before the later Citation merge stage.
type TraversalResult struct {
	Primer   PrimerResult
	Branches []Branch
	Edges    []Edge
	Usage    Usage
}
