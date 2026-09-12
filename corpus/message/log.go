package message

import (
	"context"
	"errors"
	"fmt"
	"math"
)

type Transaction interface {
	WithTx(ctx context.Context, work func(ctx context.Context) error) error
}

// Entries is the persistence contract consumed by the Message Log.
type Entries interface {
	Find(ctx context.Context, ids []string) ([]Occurrence, error)
	LastPosition(ctx context.Context) (position uint64, found bool, err error)
	Save(ctx context.Context, occurrences []Occurrence) error
}

type Dependencies struct {
	Transaction
	Entries
}

// Log is the ordered Message collection of the current Session Zone.
type Log struct {
	dependencies Dependencies
}

func NewLog(dependencies Dependencies) (*Log, error) {
	switch {
	case dependencies.Transaction == nil:
		return nil, errors.New("create Message Log: transaction is required")
	case dependencies.Entries == nil:
		return nil, errors.New("create Message Log: entries are required")
	default:
		return &Log{dependencies: dependencies}, nil
	}
}

func (log *Log) Append(ctx context.Context, messages []Message) ([]Occurrence, error) {
	if log == nil {
		return nil, errors.New("Message Log is not configured")
	}
	prepared, err := prepareMessages(messages)
	if err != nil {
		return nil, err
	}
	var occurrences []Occurrence
	err = log.dependencies.Transaction.WithTx(ctx, func(ctx context.Context) error {
		existing, err := log.existing(ctx, prepared)
		if err != nil {
			return err
		}
		missing := make([]Message, 0, len(prepared)-len(existing))
		for _, value := range prepared {
			if _, found := existing[value.ID]; !found {
				missing = append(missing, value)
			}
		}
		if len(missing) == 0 {
			occurrences = occurrencesFor(prepared, existing)
			return nil
		}
		last, found, err := log.dependencies.Entries.LastPosition(ctx)
		if err != nil {
			return err
		}
		if found && (last == math.MaxUint64 || uint64(len(missing)-1) > math.MaxUint64-last-1) {
			return fmt.Errorf("%w: Position is exhausted", ErrSequenceConflict)
		}
		next := uint64(0)
		if found {
			next = last + 1
		}
		added := make([]Occurrence, len(missing))
		for index, value := range missing {
			added[index] = Occurrence{Message: value, Position: next + uint64(index)}
			existing[value.ID] = added[index]
		}
		if err := log.dependencies.Entries.Save(ctx, added); err != nil {
			return err
		}
		occurrences = occurrencesFor(prepared, existing)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return occurrences, nil
}

// CheckAppend verifies that stored Message identities still name the same
// immutable content without changing the Log. Append repeats the check while
// holding the write transaction and reuses exact matches.
func (log *Log) CheckAppend(ctx context.Context, messages []Message) error {
	if log == nil {
		return errors.New("Message Log is not configured")
	}
	prepared, err := prepareMessages(messages)
	if err != nil {
		return err
	}
	_, err = log.existing(ctx, prepared)
	return err
}

func (log *Log) existing(ctx context.Context, messages []Message) (map[string]Occurrence, error) {
	ids := make([]string, len(messages))
	byID := make(map[string]Message, len(messages))
	for index, value := range messages {
		ids[index] = value.ID
		byID[value.ID] = value
	}
	existing, err := log.dependencies.Entries.Find(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[string]Occurrence, len(existing))
	for _, occurrence := range existing {
		expected, requested := byID[occurrence.Message.ID]
		if !requested {
			return nil, fmt.Errorf("%w: persistence returned unrequested Message %q", ErrSequenceConflict, occurrence.Message.ID)
		}
		if _, duplicate := result[occurrence.Message.ID]; duplicate {
			return nil, fmt.Errorf("%w: persistence returned duplicate Message %q", ErrSequenceConflict, occurrence.Message.ID)
		}
		if occurrence.Message != expected {
			return nil, fmt.Errorf("%w: ID %q", ErrIdentityConflict, occurrence.Message.ID)
		}
		result[occurrence.Message.ID] = occurrence
	}
	return result, nil
}

func occurrencesFor(messages []Message, byID map[string]Occurrence) []Occurrence {
	occurrences := make([]Occurrence, len(messages))
	for index, value := range messages {
		occurrences[index] = byID[value.ID]
	}
	return occurrences
}

func prepareMessages(messages []Message) ([]Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: at least one Message is required", ErrInvalid)
	}
	seen := make(map[string]struct{}, len(messages))
	prepared := append([]Message(nil), messages...)
	for index, value := range prepared {
		if err := Validate(value); err != nil {
			return nil, fmt.Errorf("append Message %d: %w", index, err)
		}
		if _, duplicate := seen[value.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate Message ID %q", ErrInvalid, value.ID)
		}
		seen[value.ID] = struct{}{}
	}
	return prepared, nil
}

func (log *Log) Read(ctx context.Context, ids []string) ([]Occurrence, error) {
	if log == nil {
		return nil, errors.New("Message Log is not configured")
	}
	if len(ids) == 0 {
		return []Occurrence{}, nil
	}
	seen := make(map[string]struct{}, len(ids))
	prepared := append([]string(nil), ids...)
	for index, id := range prepared {
		if id == "" {
			return nil, fmt.Errorf("%w: Message ID %d is required", ErrInvalid, index)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("%w: duplicate Message ID %q", ErrInvalid, id)
		}
		seen[id] = struct{}{}
	}
	result, err := log.dependencies.Entries.Find(ctx, prepared)
	if err != nil {
		return nil, err
	}
	if len(result) != len(prepared) {
		return nil, ErrNotFound
	}
	return result, nil
}
