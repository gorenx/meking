package query

// TextDeltaHandler consumes one answer fragment synchronously. Returning an
// error stops generation and propagates back to the query caller.
type TextDeltaHandler func(delta string) error
