package journal

import (
	"context"
	"errors"

	journalcore "github.com/memoria-space/meking/journal"
)

// ActionPublisher emits an in-memory wakeup after a relevant Journal append.
// The enclosing owner transaction may still roll back, so evaluators always
// re-read committed Journal state and never treat the wakeup as a fact.
type ActionPublisher struct {
	next    journalcore.Publisher
	signals *ActionSignals
}

func NewActionPublisher(
	next journalcore.Publisher,
	signals *ActionSignals,
) (*ActionPublisher, error) {
	if next == nil {
		return nil, errors.New("create Control Action Publisher: next Publisher is required")
	}
	if signals == nil {
		return nil, errors.New("create Control Action Publisher: Action Signals are required")
	}
	return &ActionPublisher{
		next:    next,
		signals: signals,
	}, nil
}

func (publisher *ActionPublisher) Publish(
	ctx context.Context,
	events []journalcore.ProposedEvent,
) error {
	if publisher == nil || publisher.next == nil || publisher.signals == nil {
		return errors.New("publish Control Action signal: Publisher is required")
	}
	if err := publisher.next.Publish(ctx, events); err != nil {
		return err
	}
	if len(events) > 0 {
		publisher.signals.notify()
	}
	return nil
}

var _ journalcore.Publisher = (*ActionPublisher)(nil)
