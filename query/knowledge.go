package query

// KnowledgeReference identifies one exact immutable Entity or Relation
// version used by a Query aggregate. The containing field supplies the
// knowledge namespace; Version is only comparable inside ID's own chain.
type KnowledgeReference struct {
	// ID is the required stable logical identity produced by Knowledge.
	ID string
	// Version is the required positive immutable version selected by a fixed
	// Report source, Entity vector match, or another published Query input.
	Version uint64
}

// ClaimReference identifies one exact Claim evidence occurrence without
// copying its content into a Report, Local, or DRIFT contract.
type ClaimReference struct {
	// ID is the required stable Claim identity produced by Knowledge.
	ID string
	// Version is the required positive immutable Claim version.
	Version uint64
	// EvidenceIndex is the zero-based source occurrence inside that exact Version. It is
	// not a request-local Citation row number.
	EvidenceIndex int
}
