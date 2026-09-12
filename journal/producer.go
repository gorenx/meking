package journal

import (
	"context"
	"time"
)

// Envelope carries producer-owned identity and causal metadata with a typed
// body. Zone ownership is supplied by the Zone-bound Producer.
type Envelope[B EventBody] struct {
	EventID       EventID
	StreamID      StreamID
	CorrelationID CorrelationID
	CausationID   EventID
	OccurredAt    time.Time
	Body          B
}

// Producer maps one owner's typed event bodies to registered Journal events and
// publishes them through that owner's active transaction.
type Producer[B EventBody] interface {
	Publish(ctx context.Context, events []Envelope[B]) error
}
