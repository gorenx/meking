package corpus_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/memory/activation"
	adapter "github.com/memoria-space/meking/memory/activation/adapter/corpus"
	"github.com/memoria-space/meking/zone"
)

type messages struct {
	ids    []string
	zone   zone.ID
	values []message.Occurrence
	err    error
}

func (m *messages) Read(ctx context.Context, ids []string) ([]message.Occurrence, error) {
	m.ids = ids
	m.zone, _ = zone.RequireID(ctx)
	return m.values, m.err
}

func TestEvidenceOnlyResolvesStoredMessageSources(t *testing.T) {
	ctx, err := zone.NewContext(t.Context(), zone.ID("22222222-2222-4222-8222-222222222222"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &messages{values: []message.Occurrence{{Message: message.Message{ID: "message-1"}}}}
	reader, err := adapter.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []activation.Evidence{{Source: activation.Inline{HostRecordID: "host-1", Role: "assistant", Text: "recall"}}, {Source: activation.StoredMessage{MessageID: "message-1"}}}
	if err := reader.Validate(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(provider.ids, []string{"message-1"}) || provider.zone != ctx.ZoneID() {
		t.Fatalf("wrong scope or IDs: %#v", provider)
	}
	provider.ids = nil
	if err := reader.Validate(ctx, evidence[:1]); err != nil || provider.ids != nil {
		t.Fatal("inline evidence called Corpus")
	}
	provider.values = nil
	if err := reader.Validate(ctx, evidence); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("missing source: %v", err)
	}
	provider.err = errors.New("read unavailable")
	if err := reader.Validate(ctx, evidence); !errors.Is(err, provider.err) {
		t.Fatalf("lost provider error: %v", err)
	}
	if err := reader.Validate(context.Background(), evidence); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("missing Zone: %v", err)
	}
	if _, err := adapter.New(nil); err == nil {
		t.Fatal("nil provider accepted")
	}
}
