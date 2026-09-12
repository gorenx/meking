// Package report reads one fixed ReportSet together with the CommunitySet,
// Corpora, and exact Knowledge versions needed to interpret it.
package report

import (
	"context"
	"errors"

	querybase "github.com/memoria-space/meking/query"
)

const (
	// MaximumReportPageSize bounds one report browse response independently of
	// the number of Reports in the selected immutable publication.
	MaximumReportPageSize = 100
)

var (
	// ErrNoReportPublication means no complete ReportSet has been selected yet.
	ErrNoReportPublication = errors.New("no report publication")
	// ErrReportNotFound means the requested Report is absent from the fixed publication.
	ErrReportNotFound = errors.New("report not found")
	// ErrInvalidReportRequest identifies caller input that cannot select a
	// bounded report page or exact Report.
	ErrInvalidReportRequest = errors.New("invalid report request")
)

// ReportSources is the complete evidence identity closure recorded by one
// immutable Report. Query exposes or reuses these exact source identities
// without resolving them against current Knowledge or Corpus state.
type ReportSources struct {
	// Entities contains exact Entity versions in model-input order. It may be
	// empty; duplicate handling is owned by Report generation.
	Entities []querybase.KnowledgeReference
	// Relations contains exact directed Relation versions in model-input order.
	// It may be empty; every item refers to one immutable Knowledge row.
	Relations []querybase.KnowledgeReference
	// Claims contains exact Claim Statements in model-input order. It may be
	// empty; EvidenceIndex selects content within the referenced Version.
	Claims []querybase.ClaimReference
	// TextUnitIDs is the strictly sorted, duplicate-free, non-empty evidence set
	// recorded by Report generation and resolved inside the fixed Corpora.
	TextUnitIDs []string
}

// ReportFinding is one model-produced finding preserved in immutable Report
// order for detail responses.
type ReportFinding struct {
	// Summary is the required short finding title produced by the report model.
	Summary string
	// Explanation is the required supporting narrative produced by the report model.
	Explanation string
}

// PublishedReport is one immutable Report selected by a ReportPublication. It
// carries only data consumed by browsing; generation settings remain owned by
// Community Report.
type PublishedReport struct {
	// ID is the required immutable Report UUID allocated by Community Report.
	ID string
	// CommunityID is the required Community summarized by this Report and must
	// identify one member of the publication's fixed CommunitySet.
	CommunityID string
	// Period is the required business period supplied to Report generation.
	Period string
	// Title is the required model-generated Community name.
	Title string
	// Summary is the required model-generated executive summary.
	Summary string
	// Findings preserves the model's ordered structured findings and may be empty.
	Findings []ReportFinding
	// Rank is the finite model-generated impact rating used by Query consumers.
	Rank float64
	// RatingExplanation is the model explanation associated with Rank.
	RatingExplanation string
	// FullContent is the required deterministic Markdown projection persisted by
	// Community Report and returned without regeneration.
	FullContent string
	// Sources is the complete immutable Knowledge and Corpus input identity set
	// recorded by Community Report when this value was generated.
	Sources ReportSources
}

// PublishedCommunity is one hierarchy position and membership in the exact
// CommunitySet fixed by a ReportPublication. Children are derived from ParentID
// so the provider cannot return two competing hierarchy representations.
type PublishedCommunity struct {
	// ID is the required immutable Community identity derived from membership.
	ID string
	// Number is the non-negative display ordinal assigned by Community detection;
	// it is unique within the CommunitySet but is not a persistent identity.
	Number int
	// Level is the non-negative hierarchy depth, where roots use zero.
	Level int
	// ParentID is nil for a root and otherwise references another Community in the
	// same publication whose Level is exactly one lower.
	ParentID *string
	// EntityIDs is the required, strictly sorted, duplicate-free membership used
	// to derive ID. Query uses its length as the Report's Community size.
	EntityIDs []string
}

// ReportPublication is the complete immutable ReportSet projection fixed at
// the start of one Query use case. It prevents one response from combining
// independently changing Community, Report, and Corpus selections.
type ReportPublication struct {
	// ReportSetID is the required immutable selection published by Community.
	ReportSetID string
	// CommunitySetID is the required immutable hierarchy summarized by ReportSetID.
	CommunitySetID string
	// CorporaID is the required immutable evidence collection used by all Reports.
	CorporaID string
	// Communities is the complete deterministic CommunitySet order. It is
	// non-empty and has exactly one corresponding value in Reports.
	Communities []PublishedCommunity
	// Reports is the complete immutable ReportSet payload in the same order as
	// Communities, with exactly one Report for each Community.
	Reports []PublishedReport
}

// ReportPublicationReader loads one exact complete report publication without
// exposing Community persistence or Project paths.
type ReportPublicationReader interface {
	ReportPublication(ctx context.Context, reportSetID string) (ReportPublication, error)
}

// View is one complete ReportSet fixed for a Query use case. It is an isolated
// value: later publication and Knowledge changes cannot alter it.
type View struct {
	// EpochID is the positive unified publication fixed before ReportSetID was read.
	EpochID int64
	// ReportSetID is the immutable publication selected once for this value.
	ReportSetID string
	// CommunitySetID identifies the complete hierarchy summarized by Reports.
	CommunitySetID string
	// CorporaID identifies the immutable evidence collection used by Reports.
	CorporaID string
	// Communities preserves the complete deterministic CommunitySet order.
	Communities []PublishedCommunity
	// Reports preserves Community order and has one value per Community.
	Reports []PublishedReport
}

// ReportPageRequest selects one positive ordinal page whose size does not
// exceed MaximumReportPageSize.
type ReportPageRequest struct {
	// Page is one-based. Values below one are invalid.
	Page int
	// PageSize is the positive maximum number of Reports returned by this page.
	PageSize int
}

// ReportSummary is the navigation projection of one published Report.
type ReportSummary struct {
	// ID is the immutable Report identity accepted by the detail use case.
	ID string
	// CommunityID is the immutable summarized Community identity.
	CommunityID string
	// CommunityNumber is the CommunitySet-local display ordinal.
	CommunityNumber int
	// Level is the Community hierarchy depth, where roots use zero.
	Level int
	// Title is the persisted model-generated Report title.
	Title string
	// Summary is the persisted model-generated executive summary.
	Summary string
	// Rank is the finite model-generated impact rating.
	Rank float64
	// Period is the business period fixed by Report generation.
	Period string
	// Size is the number of Entity IDs in the exact Community membership.
	Size int
}

// ReportPage preserves the exact publication identities used for paging so a
// caller never interprets results through a later ReportSet or Corpora.
type ReportPage struct {
	// EpochID is the positive unified publication fixed for this page request.
	EpochID int64
	// ReportSetID is the immutable publication selected once for this response.
	ReportSetID string
	// CommunitySetID identifies the hierarchy summarized by ReportSetID.
	CommunitySetID string
	// CorporaID identifies the evidence collection bound when ReportSet was published.
	CorporaID string
	// Page echoes the requested one-based page.
	Page int
	// PageSize echoes the requested positive page bound.
	PageSize int
	// Total is the number of Reports in the fixed complete ReportSet.
	Total int
	// Reports preserves deterministic Community order for this page and is empty
	// when Page is beyond Total.
	Reports []ReportSummary
}

// ReportDetail is the complete browsing projection of one Report and
// its exact source identities in the fixed publication.
type ReportDetail struct {
	// EpochID is the positive unified publication fixed for this detail request.
	EpochID int64
	// ReportSetID is the immutable publication selected once for this response.
	ReportSetID string
	// CommunitySetID identifies the hierarchy containing CommunityID.
	CommunitySetID string
	// CorporaID identifies the exact evidence collection bound at publication.
	CorporaID string
	// ID is the immutable Report identity requested by the caller.
	ID string
	// CommunityID is the immutable summarized Community identity.
	CommunityID string
	// CommunityNumber is the CommunitySet-local display ordinal.
	CommunityNumber int
	// Level is the Community hierarchy depth, where roots use zero.
	Level int
	// ParentID is nil for a root and otherwise identifies its direct parent in the
	// same CommunitySet.
	ParentID *string
	// Children contains direct child Community IDs ordered by Community number.
	Children []string
	// Title is the persisted model-generated Report title.
	Title string
	// Summary is the persisted model-generated executive summary.
	Summary string
	// FullContent is the persisted deterministic Markdown Report projection.
	FullContent string
	// Rank is the finite model-generated impact rating.
	Rank float64
	// RatingExplanation is the persisted model explanation for Rank.
	RatingExplanation string
	// Findings preserves the model-produced ordered structured findings.
	Findings []ReportFinding
	// Period is the business period fixed by Report generation.
	Period string
	// Size is the number of Entity IDs in the exact Community membership.
	Size int
	// Sources contains the exact historical Knowledge and Corpus identities used
	// to generate this immutable Report.
	Sources ReportSources
}
