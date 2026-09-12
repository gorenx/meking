package extraction

// claimObservation is one model-produced assertion linked to the TextUnit that
// supplied its evidence.
type claimObservation struct {
	// Subject is the model-returned name of the entity whose action or conduct
	// the assertion describes. The prompt requires a matched named entity, while
	// parsing only enforces a non-empty value and does not resolve an Entity ID.
	Subject string
	// Object is the model-returned name of the entity that reports, handles, or
	// is affected by the asserted action. Unknown objects use "NONE"; an empty
	// protocol field is invalid. This value is not resolved during extraction.
	Object string
	// Type is the model-returned reusable category for similar assertions, such
	// as "ANTI-COMPETITIVE PRACTICES". The prompt requests a capitalized label
	// reusable across inputs; extraction does not normalize it or enforce an enum.
	Type        string
	Status      string
	StartDate   string
	EndDate     string
	Description string
	SourceText  string
	TextUnitID  string
}

// Claim is one extracted assertion submitted to Knowledge reconciliation.
// Stable identity and Version are assigned from the resolved Subject and Type;
// TextUnitID retains the source evidence boundary.
type extractedClaim struct {
	// Subject retains the required model-returned entity name used to associate
	// this assertion with graph and community records; it is not an Entity ID.
	Subject string
	// Object retains the model-returned reporting, handling, or affected entity
	// name. "NONE" remains an unresolved value, not an Entity ID.
	Object string
	// Type retains the model-returned reusable assertion category. The extractor
	// does not normalize it or restrict it to a predefined enum.
	Type        string
	Status      string
	StartDate   string
	EndDate     string
	Description string
	SourceText  string
	TextUnitID  string
}
