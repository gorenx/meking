package local

import (
	"context"

	"github.com/memoria-space/meking/knowledge"
	querybase "github.com/memoria-space/meking/query"
	"github.com/memoria-space/meking/query/qctx"
	queryreport "github.com/memoria-space/meking/query/report"
)

// ReportReader loads the exact report publication named by an Epoch already
// fixed by Local. It returns an isolated value and never resolves Current.
type ReportReader interface {
	Publication(ctx context.Context, selected querybase.Epoch) (queryreport.View, error)
}

// EntityMatch is one active Entity version returned from the fixed vector
// view. ID and Version together are the only identity Local uses for its exact
// Knowledge read; Score is used only for request-local ranking.
type EntityMatch struct {
	// ID is the stable Knowledge Entity identity stored with the vector.
	ID string
	// Version is the positive immutable Entity version that produced the vector.
	Version uint64
	// Score is the finite cosine similarity in [-1,1] returned by the vector implementation.
	Score float64
}

// EntityVectorReader is one already-opened, request-stable view of active
// Entity vectors. Its implementation may use any physical store, but Search
// must support concurrent calls and continue observing the same image for the
// reader's lifetime.
type EntityVectorReader interface {
	// Model is the embedding model identity shared by all records in this view.
	Model() string
	// Dimension is the vector length required by Search. Zero identifies a
	// valid empty active index and prevents Local from calling the embedder.
	Dimension() int
	// Search returns at most limit cosine-ranked matches in deterministic score
	// order. Every match must contain a positive exact Knowledge Version.
	Search(ctx context.Context, vector []float64, limit int) ([]EntityMatch, error)
	// Close releases the fixed view and is idempotent.
	Close() error
}

// EntityVectorStore opens the exact Entity vector set selected by Epoch.
// Entity-vector storage contract Local owns; no database, directory, update,
// deletion, or publication implementation belongs to this package.
type EntityVectorStore interface {
	Open(ctx context.Context, entities []knowledge.Reference[knowledge.EntityID]) (EntityVectorReader, error)
}

// QuestionEmbedder produces one dense retrieval vector and exposes the model
// identity required to match the fixed Entity vector reader. The supplied text
// may include recent user questions selected by Local. EmbedQuestion supports
// concurrent calls from one Local Session.
type QuestionEmbedder interface {
	Model() string
	EmbedQuestion(ctx context.Context, question string) ([]float64, error)
}

// TokenCounter is Query's shared exact-Corpus tokenizer contract.
type TokenCounter = querybase.TokenCounter

// KnowledgeRequest is one exact, duplicate-free batch selected by Local from
// vector matches and the fixed ReportSet source closure. Each slice preserves
// caller order and contains no Current lookup or optional knowledge kind.
type KnowledgeRequest struct {
	// Entities contains exact Entity references required for selected rows and
	// Relation endpoint display.
	Entities []querybase.KnowledgeReference
	// Relations contains exact directed Relation references exposed by the
	// selected Reports.
	Relations []querybase.KnowledgeReference
	// Claims contains exact Claim Statements exposed by the selected Reports.
	Claims []querybase.ClaimReference
}

// Entity is the Local-owned projection of one exact Knowledge Entity version.
// It carries only fields used for ranking, prompt rows, and Citation sources.
type Entity struct {
	// ID is the stable Entity identity returned for the requested exact reference.
	ID string
	// Version is the positive immutable version selected by the vector or Report source.
	Version uint64
	// Title is the exact Entity title shown in the model-visible row.
	Title string
	// Description is the exact Entity description shown in the model-visible row.
	Description string
	// TextUnitIDs is the non-empty source set copied from this Entity version.
	TextUnitIDs []string
	// Degree is the complete graph degree stored on this Entity version.
	Degree int
}

// Relationship is the Local-owned projection of one exact directed Knowledge
// Relation version. Endpoint IDs remain stable identities; display titles are
// resolved from the report's exact Entity source closure by Local.
type Relationship struct {
	// ID is the stable directed Relation identity.
	ID string
	// Version is the positive immutable Relation version selected by a Report source.
	Version uint64
	// SourceEntityID is the stable directed source endpoint.
	SourceEntityID string
	// TargetEntityID is the stable directed target endpoint.
	TargetEntityID string
	// Description is the exact Relation description shown in context.
	Description string
	// Weight is the finite graph aggregation strength shown in context.
	Weight float64
	// CombinedDegree is the endpoint-degree sum used for deterministic ranking.
	CombinedDegree int
	// TextUnitIDs is the non-empty source set copied from this Relation version.
	TextUnitIDs []string
}

// ClaimSubject identifies the Entity or Relation described by a Claim without
// exposing Knowledge persistence discriminators to Local Search.
type ClaimSubject interface {
	SubjectID() string
	claimSubject()
}

type EntityClaimSubject struct {
	ID string
}

func (s EntityClaimSubject) SubjectID() string { return s.ID }
func (EntityClaimSubject) claimSubject()       {}

type RelationClaimSubject struct {
	ID string
}

func (s RelationClaimSubject) SubjectID() string { return s.ID }
func (RelationClaimSubject) claimSubject()       {}

// Claim is one exact model-visible Claim Statement. Claim Version and
// EvidenceIndex select its immutable source content.
type Claim struct {
	// ID is the stable Claim identity.
	ID string
	// Version is the positive immutable Claim version selected by a Report source.
	Version uint64
	// EvidenceIndex is the zero-based immutable Statement position in Version.
	EvidenceIndex int
	// Subject is the stable Entity or Relation described by this Claim.
	Subject ClaimSubject
	// Type is the Claim classification stored on the exact Claim version.
	Type string
	// Status is the model-produced status of this exact Statement.
	Status string
	// StartDate is optional source date text; empty means no start was supplied.
	StartDate string
	// EndDate is optional source date text; empty means no end was supplied.
	EndDate string
	// Description is the model-produced explanation of this Statement.
	Description string
	// SourceText is the exact source excerpt retained by the Statement.
	SourceText string
	// TextUnitID is the single Corpus source that produced this Statement.
	TextUnitID string
}

// Knowledge contains exact rows returned in the same order as one
// KnowledgeRequest. The provider does not select Current, graph scope, or
// prompt order; Local consumes the provider's ordered exact-read contract
// without repeating Knowledge's publication-integrity checks.
type Knowledge struct {
	// Entities corresponds one-for-one with KnowledgeRequest.Entities.
	Entities []Entity
	// Relationships corresponds one-for-one with KnowledgeRequest.Relations.
	Relationships []Relationship
	// Claims corresponds one-for-one with KnowledgeRequest.Claims.
	Claims []Claim
}

// KnowledgeReader resolves exact immutable Knowledge rows without exposing
// MVCC storage, ReadView lifetime, or type-specific persistence readers. Both
// methods support concurrent reads at the same fixed boundary.
type KnowledgeReader interface {
	// CurrentEntityReferences resolves stable Entity IDs at the same fixed
	// Knowledge boundary used by Read. Missing IDs are omitted.
	CurrentEntityReferences(
		ctx context.Context,
		versions knowledge.Manifest,
		entityIDs []string,
	) ([]querybase.KnowledgeReference, error)
	Read(ctx context.Context, versions knowledge.Manifest, request KnowledgeRequest) (Knowledge, error)
}

// AnswerModelRequest is the exact two-message Local completion input.
type AnswerModelRequest struct {
	// SystemPrompt contains the fully rendered fixed evidence and response type.
	SystemPrompt string
	// UserPrompt is the original non-blank question without normalization.
	UserPrompt string
}

// AnswerModel produces a complete Local answer from already-fixed evidence.
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

// SearchRequest contains request text and explicit stable Entity filters. The
// filters affect only this request and never change the active vector image.
type SearchRequest struct {
	// Question is the required user text sent unchanged to embedding and answer models.
	Question string
	// Conversation is copied before recent user-led exchanges are rendered.
	Conversation []querybase.ConversationTurn
	// IncludeEntityIDs are resolved from the fixed vector view and prepended in
	// caller order before semantic matches.
	IncludeEntityIDs []string
	// ExcludeEntityIDs remove both explicit and semantic matches by stable ID.
	ExcludeEntityIDs []string
	// ResponseType optionally overrides Config.ResponseType for this answer.
	ResponseType string
}

// Context is the exact model-visible Local evidence. Each Table contains only
// rows admitted under its fixed budget, and Text is their final concatenation
// with the optional conversation prefix.
type Context struct {
	// Text is the exact concatenation sent through context_data.
	Text string
	// Conversation is the optional recent history prefix and has no Citation rows.
	Conversation string
	// Reports contains admitted rows from the fixed ReportSet.
	Reports qctx.Table
	// Entities contains admitted rows from exact vector-matched Entity versions.
	Entities qctx.Table
	// Relationships contains admitted directed graph rows touching an admitted Entity.
	Relationships qctx.Table
	// Claims contains admitted Statements whose Subject row is also admitted.
	Claims qctx.Table
	// Sources contains admitted TextUnit rows supporting earlier evidence rows.
	Sources qctx.Table
}

// Evidence is one model-visible Local context produced from a Session's fixed
// Report and Entity-vector views. It exists so a complete Local answer and a
// DRIFT branch can reuse exactly the same retrieval and budget rules without
// either use case reopening Current or sharing mutable context rows.
type Evidence struct {
	// EpochID is the positive unified publication that selected ReportSetID and
	// CorporaID for this Local request.
	EpochID int64
	// ReportSetID identifies the immutable Reports fixed once for this request.
	ReportSetID string
	// CommunitySetID identifies the hierarchy bound by ReportSetID.
	CommunitySetID string
	// CorporaID identifies all TextUnit source reads in this request.
	CorporaID string
	// Matches preserves explicit and semantic Entity ranking before context budgeting.
	Matches []EntityMatch
	// Context is the exact evidence admitted for the answer model.
	Context Context
}

// Result is one completed Local query. Evidence records the independently
// fixed inputs and exact model-visible rows; ReportSet may legitimately lag
// behind the Entity vector image because every row retains its source Version.
type Result struct {
	Evidence
	// Response is the model output and is never rewritten by Citation audit.
	Response string
	// CitationAudit resolves only records actually present in Context.
	CitationAudit querybase.CitationAudit
}
