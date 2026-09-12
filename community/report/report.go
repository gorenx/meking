package report

import "github.com/memoria-space/meking/community"

// ID identifies one immutable Report independently of its content,
// Knowledge versions, and query visibility. Its canonical representation is a
// lowercase UUID v4 allocated only after the complete Report has been validated.
type ID string

// ClaimSource identifies one exact Claim evidence occurrence supplied to the
// report model. ID and Version select the immutable Claim; EvidenceIndex
// selects one source occurrence attached to that version.
type ClaimSource struct {
	// ID is the canonical lowercase Claim UUID read from one Knowledge Read View.
	ID string
	// Version is positive and only comparable within ID's Claim version chain.
	Version uint64
	// EvidenceIndex is the zero-based source occurrence in the selected Claim Version.
	// It is not the row number shown in the report prompt.
	EvidenceIndex int
}

// Settings records the exact generation choices that can change model
// output for otherwise identical evidence. It is copied from the configured
// Generator when a Report is finalized and is immutable thereafter.
type Settings struct {
	// Model is the non-empty provider-neutral model name selected from the fixed
	// Project snapshot. Credentials, endpoints, and response IDs are excluded.
	Model string
	// PromptHash is the raw SHA-256 of the configured final-report template and
	// the internal Fragment templates. A change to any model-visible instruction
	// therefore makes otherwise identical evidence require regeneration.
	PromptHash [32]byte
	// Tokenizer is the non-empty vocabulary name used to enforce MaxInputTokens.
	Tokenizer string
	// MaxInputTokens is the positive hard limit applied to the fully rendered
	// prompt, including instructions and complete Community evidence.
	MaxInputTokens int
	// MaxReportLength is the positive requested word limit inserted into Prompt.
	MaxReportLength int
}

// Report is the immutable result of generating one Community summary from a
// complete, exact set of Knowledge versions. ReportSet controls query
// visibility; Report itself has no Current flag, version chain, or Tombstone.
type Report struct {
	// ID is a canonical lowercase UUID v4 allocated once for this immutable value.
	ID ID
	// CommunityID identifies the exact sorted Entity membership summarized by
	// this Report. ReportSet links it to one complete Community hierarchy.
	CommunityID community.CommunityID
	// Period is the UTC calendar date of the source Knowledge publication. It is
	// part of Report identity because it appears in report metadata.
	Period string
	// Title is the non-empty model-generated name of the Community.
	Title string
	// Summary is the non-empty model-generated executive summary.
	Summary string
	// Findings preserves model order because reordering changes the report's
	// meaning, Markdown projection, and identity.
	Findings []community.ReportFinding
	// Rank is the finite model-generated impact rating consumed by Query.
	Rank float64
	// RatingExplanation is the non-empty model explanation for Rank.
	RatingExplanation string
	// FullContent is the deterministic Markdown projection used by embedding and
	// Query. It is persisted for reads but must equal community.RenderFullReport.
	FullContent string
	// FullContentJSON is the deterministic indented JSON projection of the same
	// structured result and must equal community.MarshalReportJSON output.
	FullContentJSON string
	// EntitySources contains every member Entity version sent to the model in
	// exact prompt-row order. Slice position is the prompt human_readable_id.
	EntitySources []community.EntityReference
	// RelationSources contains every directed internal Relation version sent to
	// the model in exact prompt-row order.
	RelationSources []community.RelationReference
	// ClaimSources contains one entry for every Claim evidence occurrence sent to the model
	// in exact prompt-row order.
	ClaimSources []ClaimSource
	// TextUnitIDs is the strictly sorted, duplicate-free union of evidence IDs
	// cited by all three source collections. Corpus owns content and occurrences.
	TextUnitIDs []string
	// Settings records the exact generation choices used for this Report.
	Settings Settings
}
