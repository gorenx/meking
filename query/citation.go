package query

import querysource "github.com/memoria-space/meking/query/source"

// CitationDataset identifies one model-visible context table supported by the
// shared Query citation grammar.
type CitationDataset string

const (
	CitationSources       CitationDataset = "Sources"
	CitationReports       CitationDataset = "Reports"
	CitationEntities      CitationDataset = "Entities"
	CitationRelationships CitationDataset = "Relationships"
	CitationClaims        CitationDataset = "Claims"
)

// CitationStatus reports whether one generated reference is backed by a row
// in the owning aggregate's final model context and at least one Corpus source.
type CitationStatus string

const (
	CitationValid   CitationStatus = "valid"
	CitationInvalid CitationStatus = "invalid"
)

// CitationInvalidReason gives callers a stable explanation without rejecting
// an otherwise completed model answer.
type CitationInvalidReason string

const (
	CitationMalformed          CitationInvalidReason = "malformed"
	CitationUnsupportedDataset CitationInvalidReason = "unsupported_dataset"
	CitationInvalidRecordID    CitationInvalidReason = "invalid_record_id"
	CitationTooManyRecords     CitationInvalidReason = "too_many_record_ids"
	CitationOutsideContext     CitationInvalidReason = "outside_context"
	CitationUnresolvedSource   CitationInvalidReason = "unresolved_source"
)

// CitationReference identifies one request-local integer in a model-visible
// dataset. It is never a persistent knowledge, report, or Corpus identity.
type CitationReference struct {
	// Dataset names the table that owns RecordID.
	Dataset CitationDataset
	// RecordID is the non-negative zero-based integer rendered for this request.
	RecordID int
}

// CitationRecord is one row the owning Query aggregate allows the final model
// answer to reference. The aggregate creates it only after its own selection,
// budgeting, Map/Reduce, or branch-merging rules have finished.
type CitationRecord struct {
	// Reference is the exact dataset and integer shown to the model.
	Reference CitationReference
	// CorporaID identifies the immutable source collection for this row.
	CorporaID string
	// TextUnitIDs is the non-empty, sorted, duplicate-free source identity set
	// supporting this row. Source resolution returns every Corpus occurrence.
	TextUnitIDs []string
}

// Citation preserves one generated citation segment and its source resolution.
type Citation struct {
	// Raw preserves the generated dataset group or malformed citation block.
	Raw string
	// Reference is meaningful only when Parsed is true.
	Reference CitationReference
	// Parsed reports that dataset and record identity are syntactically valid.
	Parsed bool
	// Status is valid only when the cited row resolves to at least one source.
	Status CitationStatus
	// InvalidReason is empty for a valid Citation.
	InvalidReason CitationInvalidReason
	// Sources contains every Corpus occurrence supporting the cited row.
	Sources []querysource.TextUnitSource
}

// CitationAudit annotates generated references without changing answer text or
// success. Each Basic, Local, Global, or DRIFT aggregate owns when it is run.
type CitationAudit struct {
	// Missing reports that the answer contains no exact data citation marker.
	Missing bool
	// Items preserves generated citation order, including invalid entries.
	Items []Citation
}
