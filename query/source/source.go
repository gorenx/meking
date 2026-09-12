// Package source defines the shared Query projection of exact Corpus TextUnit
// occurrences. It does not select evidence, resolve Current, or audit citations.
package source

import "context"

// TextUnitSource is one exact TextUnit appearance returned to a Query aggregate.
// Corpus remains the fact source; Query retains only fields needed to show the
// supporting chunk and its stable Text and Document identities.
type TextUnitSource struct {
	// CorporaID identifies the immutable evidence collection requested by the
	// owning Query aggregate.
	CorporaID string
	// TextUnitID is the content-derived identity requested from Corpus.
	TextUnitID string
	// Text is the immutable TextUnit content shown as supporting evidence.
	Text string
	// TextID identifies the normalized Text containing this unit.
	TextID string
	// DocumentID identifies the source file from which TextID was produced.
	DocumentID string
	// DocumentLocation is the source path, object key, or URL recorded by Corpus.
	DocumentLocation string
	// TextTitle is the display title stored for Text.
	TextTitle string
}

// Reader batch-resolves requested TextUnit identities inside one exact
// Corpora. Missing requested identities are omitted; duplicate content at
// different positions remains as separate TextUnitSource values in Corpus order.
type Reader interface {
	Read(
		ctx context.Context,
		corporaID string,
		textUnitIDs []string,
	) ([]TextUnitSource, error)
}
