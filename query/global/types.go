// Package global owns ReportSet-backed Global Search selection, context,
// Map/Reduce execution, citations, and request-lifetime read consistency.
package global

import (
	"context"

	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

// TokenCounter is Query's shared exact-Corpus tokenizer contract.
type TokenCounter = querybase.TokenCounter

// EntityEvidence is one exact Entity version used by at least one published
// Report. Global Search uses its TextUnitIDs only to calculate occurrence
// weight; report text remains the model evidence.
type EntityEvidence struct {
	// Reference is the immutable Entity ID and Version recorded by Report
	// generation. Values are unique and sorted by ID then Version in ReportEvidence.
	Reference querybase.KnowledgeReference
	// TextUnitIDs is the immutable, sorted evidence set stored on that exact Entity
	// Version. Knowledge is the fact source.
	TextUnitIDs []string
}

// ReportEvidence is the fixed input from which one Global Search request
// selects and weights Reports. ReportView contains the exact ReportSet,
// CommunitySet, Corpora, Reports, and hierarchy selected by one Epoch;
// Entities supplies occurrence weights without consulting Current.
type ReportEvidence struct {
	// ReportView is isolated by query/report before this value is returned.
	ReportView queryreport.View
	// Entities contains every distinct Entity reference used by the Reports.
	Entities []EntityEvidence
}

// ReportReader loads the exact report publication named by an Epoch already
// fixed by the Global aggregate. It never resolves Current independently.
type ReportReader interface {
	Publication(ctx context.Context, selected querybase.Epoch) (queryreport.View, error)
}

// ReportEvidenceReader resolves the exact ReportSet and Entity versions named
// by an Epoch already fixed by the Global aggregate.
type ReportEvidenceReader interface {
	ReportEvidence(ctx context.Context, selected querybase.Epoch) (ReportEvidence, error)
}
