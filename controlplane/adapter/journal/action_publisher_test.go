package journal

import (
	"context"
	"errors"
	"testing"

	corpusevents "github.com/memoria-space/meking/corpus/integration"
	journalcore "github.com/memoria-space/meking/journal"
)

type publisherStub struct {
	err error
}

func (stub *publisherStub) Publish(
	context.Context,
	[]journalcore.ProposedEvent,
) error {
	return stub.err
}

func TestActionPublisherSignalsRelevantInput(t *testing.T) {
	next := &publisherStub{}
	signals := NewActionSignals()
	publisher, err := NewActionPublisher(next, signals)
	if err != nil {
		t.Fatal(err)
	}
	key := journalcore.EventKeyFor[corpusevents.DocumentRecordedV1]()
	if err := publisher.Publish(t.Context(), []journalcore.ProposedEvent{
		{
			Type:          key.Type,
			SchemaVersion: key.Version,
		},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-signals.Wakeups():
	default:
		t.Fatal("relevant Action input did not signal Automatic evaluation")
	}
}

func TestActionPublisherSignalsAnyCommittedEvent(t *testing.T) {
	next := &publisherStub{}
	signals := NewActionSignals()
	publisher, err := NewActionPublisher(next, signals)
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.Publish(t.Context(), []journalcore.ProposedEvent{
		{
			Type:          "knowledge.internal",
			SchemaVersion: 1,
		},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-signals.Wakeups():
	default:
		t.Fatal("committed event did not signal Automatic evaluation")
	}
}

func TestActionPublisherDoesNotSignalFailedAppend(t *testing.T) {
	failure := errors.New("append failed")
	next := &publisherStub{
		err: failure,
	}
	signals := NewActionSignals()
	publisher, err := NewActionPublisher(next, signals)
	if err != nil {
		t.Fatal(err)
	}
	key := journalcore.EventKeyFor[corpusevents.DocumentRecordedV1]()
	err = publisher.Publish(t.Context(), []journalcore.ProposedEvent{
		{
			Type:          key.Type,
			SchemaVersion: key.Version,
		},
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Publish() error = %v", err)
	}
	select {
	case <-signals.Wakeups():
		t.Fatal("failed Journal append signaled Automatic evaluation")
	default:
	}
}
