package corpus

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/memory/activation"
	"github.com/memoria-space/meking/zone"
)

type Messages interface {
	Read(context.Context, []string) ([]message.Occurrence, error)
}
type Reader struct{ messages Messages }

func New(messages Messages) (*Reader, error) {
	if messages == nil {
		return nil, errors.New("activation: Message reader is required")
	}
	return &Reader{messages: messages}, nil
}

func (reader *Reader) Validate(ctx context.Context, evidence []activation.Evidence) error {
	if _, err := zone.RequireID(ctx); err != nil {
		return err
	}
	ids := make([]string, 0, len(evidence))
	for _, item := range evidence {
		switch source := item.Source.(type) {
		case activation.StoredMessage:
			ids = append(ids, source.MessageID)
		case activation.Inline:
		default:
			return activation.ErrInvalidObservation
		}
	}
	if len(ids) == 0 {
		return nil
	}
	values, err := reader.messages.Read(ctx, ids)
	if err != nil {
		return err
	}
	found := make(map[string]bool, len(values))
	for _, value := range values {
		found[value.Message.ID] = true
	}
	for _, id := range ids {
		if !found[id] {
			return message.ErrNotFound
		}
	}
	return nil
}
