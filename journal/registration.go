package journal

import "context"

// EventContractRegistration binds an exact event version to the producer-owned
// body contract used before any append.
type EventContractRegistration struct {
	Key      EventKey
	Contract EventContract
}

// AtomicBatchContract names the member facts and final completion boundary
// that must become visible in one Journal append.
type AtomicBatchContract struct {
	MemberEvents    []EventKey
	CompletionEvent EventKey
}

// ConsumerRegistration binds a durable Consumer identity to the exact event
// versions handled by that consumer. ID is part of persisted delivery state;
// changing it starts an independent position rather than renaming a label.
type ConsumerRegistration struct {
	ID     ConsumerID
	Events []EventKey
}

// Consumer is implemented by each consumer-side adapter. Its registration is
// the owner-defined stable Position identity and exact event subscription; the
// delivery composition only collects Consumers. Handle returns nil only after
// its durable business result and recovery identity have committed.
type Consumer interface {
	ConsumerRegistration() ConsumerRegistration
	Handle(ctx context.Context, event Event) error
}
