package report

import "github.com/memoria-space/meking/community"

// Entity is the Report subdomain's projection of one exact Knowledge Entity
// version. Community copies it from one fixed Knowledge read view; Generator
// validates, orders, renders, and records it without changing the source data.
type Entity struct {
	// ID is the canonical Knowledge EntityID used for Community membership.
	ID string
	// Version selects the immutable Knowledge Entity row copied with ID.
	Version uint64
	// Title is the required display name rendered into the model prompt.
	Title string
	// Description is copied from the selected Entity Version without rewriting.
	Description string
	// Degree is non-negative and orders prompt rows by Degree descending, then ID.
	Degree int
	// TextUnitIDs is the non-empty, strictly sorted evidence set of the selected
	// Entity Version. Every value must occur in Input.TextUnitIDs.
	TextUnitIDs []string
}

// Relation is the Report subdomain's projection of one exact directed
// Knowledge Relation version. Both endpoints must reference Input.Entities.
type Relation struct {
	// ID is the canonical Knowledge RelationID.
	ID string
	// Version selects the immutable Knowledge Relation row copied with ID.
	Version uint64
	// SourceEntityID is the exact directed source copied from the Relation Version.
	SourceEntityID string
	// TargetEntityID is the exact directed target copied from the Relation Version.
	TargetEntityID string
	// Description is copied from the selected Relation Version without rewriting.
	Description string
	// Weight is the finite relation strength copied from the selected Version.
	Weight float64
	// CombinedDegree is non-negative and orders prompt rows by CombinedDegree
	// descending, then ID.
	CombinedDegree int
	// TextUnitIDs is the non-empty, strictly sorted evidence set of the selected
	// Relation Version. Every value must occur in Input.TextUnitIDs.
	TextUnitIDs []string
}

// ClaimSubjectKind states which exact Knowledge object a Claim describes.
type ClaimSubjectKind string

const (
	// EntityClaimSubject requires ClaimSubject.ID to identify one Input Entity.
	EntityClaimSubject ClaimSubjectKind = "entity"
	// RelationClaimSubject requires ClaimSubject.ID to identify one Input Relation.
	RelationClaimSubject ClaimSubjectKind = "relation"
)

// ClaimSubject identifies the Entity or Relation described by a Claim without
// inferring Relation identity from its endpoints or from display text.
type ClaimSubject struct {
	// Kind is exactly EntityClaimSubject or RelationClaimSubject.
	Kind ClaimSubjectKind
	// ID is the canonical EntityID or RelationID selected by Kind.
	ID string
}

// Claim is one source-grounded Statement from an exact Knowledge Claim
// Version. A multi-Statement Claim contributes one value per canonical index.
type Claim struct {
	// ID is the canonical Knowledge ClaimID.
	ID string
	// Version selects the immutable Claim row containing this Statement.
	Version uint64
	// EvidenceIndex is the zero-based position in that Version's canonical
	// Statements slice; it is not the row number rendered in the prompt.
	EvidenceIndex int
	// Subject identifies the exact Entity or Relation described by this Claim.
	Subject ClaimSubject
	// SubjectText is the non-empty natural-language subject from the Statement.
	SubjectText string
	// ObjectText is the optional natural-language object from the Statement.
	ObjectText string
	// Type is the classification owned by the selected Claim Version.
	Type string
	// Status is the exact model-produced status stored by the source Statement.
	Status string
	// StartDate is model-provided date text; empty means no start was supplied.
	StartDate string
	// EndDate is model-provided date text; empty means no end was supplied.
	EndDate string
	// Description is the assertion explanation stored by the source Statement.
	Description string
	// SourceText is the exact non-empty Claim excerpt from TextUnitID.
	SourceText string
	// TextUnitID identifies the immutable Corpus evidence for this Statement.
	TextUnitID string
}

// Input is the complete projection required to generate immutable Reports.
// Community constructs it from one CommunitySet hierarchy, one fixed Knowledge
// read view, and one fixed Corpora before the first model call.
type Input struct {
	// Communities is the complete report hierarchy copied from one immutable
	// CommunitySet. Detection configuration and source versions are not report
	// generation inputs.
	Communities []community.Membership
	// Entities contains all current Entity projections in strict ID order.
	Entities []Entity
	// Relations contains all current directed Relation projections in strict ID order.
	Relations []Relation
	// Claims contains one value per canonical Statement in ClaimID and index order.
	Claims []Claim
	// TextUnitIDs contains all distinct TextUnits available in the fixed Corpus
	// set, strictly ordered by ID. It proves evidence closure without copying raw
	// TextUnit text into the graph prompt.
	TextUnitIDs []string
	// Period is the source Knowledge publication's UTC date copied to every Report.
	Period string
}

// Config defines model identity, prompt rendering, and complete-input limits
// for one Generator. Every field except MaxConcurrency participates in the
// immutable Report Settings because it can change generated content.
type Config struct {
	// Prompt is formatted with input_text and max_report_length for each Community.
	Prompt string
	// Model is the provider-neutral model name selected from the fixed Project snapshot.
	Model string
	// Tokenizer is the vocabulary used by ReportTokenCounter.
	Tokenizer string
	// MaxInputTokens is the positive hard limit for the fully rendered prompt.
	MaxInputTokens int
	// MaxReportLength is the positive requested word limit inserted into Prompt.
	MaxReportLength int
	// MaxConcurrency limits model calls at one hierarchy level. Zero means one.
	MaxConcurrency int
}
