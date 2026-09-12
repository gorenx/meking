package extraction

import "errors"

var (
	// ErrNoEntitiesDetected marks a graph extraction stage without entity evidence.
	ErrNoEntitiesDetected = errors.New("graph extraction failed: no entities detected during extraction")
	// ErrUnresolvedEntityReference rejects a Relation whose endpoint is absent.
	ErrUnresolvedEntityReference = errors.New("graph extraction failed: unresolved Entity reference")
	// ErrInvalidGraphExtraction rejects model output that cannot be represented
	// as complete Entity and Relation evidence.
	ErrInvalidGraphExtraction = errors.New("invalid graph extraction protocol")
)

// TextUnitInput is the immutable corpus data required by graph extraction.
type TextUnitInput struct {
	// ID is copied to every observation as its evidence reference.
	ID string
	// Text is trimmed and inserted into the graph extraction prompt.
	Text string
}

// entityObservation is one model-produced entity fact tied to its evidence unit.
type entityObservation struct {
	// Title is the cleaned, uppercased entity name produced by the model.
	Title string
	// Type is the cleaned, uppercased entity category produced by the model.
	Type string
	// Aliases contains alternative names produced by the model's JSON array for
	// this TextUnit. Values use the same cleaning and uppercase rules as Title,
	// exclude empty values and Title, and are byte-lexicographically sorted with
	// duplicates removed. The non-nil empty slice means that the record did not
	// supply aliases.
	Aliases []string
	// Description is the cleaned model statement about the entity.
	Description string
	// TextUnitID identifies the TextUnit that supports this observation.
	TextUnitID string
}

// relationshipObservation is one model-produced edge tied to its evidence unit.
type relationshipObservation struct {
	// Source is the cleaned, uppercased title of the relationship origin.
	Source string
	// Target is the cleaned, uppercased title of the relationship destination.
	Target string
	// Type is the normalized business relationship category.
	Type string
	// Description is the cleaned model statement explaining the relationship.
	Description string
	// TextUnitID identifies the TextUnit that supports this observation.
	TextUnitID string
	// Weight is the finite, non-negative model strength value.
	Weight float64
}

// graphObservations contains independent evidence extracted from text units.
type graphObservations struct {
	// Entities preserves extracted entity order and duplicates.
	Entities []entityObservation
	// Relationships preserves valid extracted edge order and duplicates.
	Relationships []relationshipObservation
}

func emptyGraphObservations() graphObservations {
	return graphObservations{
		Entities:      make([]entityObservation, 0),
		Relationships: make([]relationshipObservation, 0),
	}
}
