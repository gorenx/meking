package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/journal/adapter/sqlite/internal/db"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

func (s *Store) Append(
	ctx context.Context,
	events []journal.ProposedEvent,
) error {
	if len(events) == 0 {
		return fmt.Errorf("%w: append batch is empty", journal.ErrInvalidEvent)
	}
	executor, err := transactionsqlite.Current(ctx, s.database)
	if err != nil {
		return fmt.Errorf("append Journal events: %w", err)
	}
	queries := db.New(executor)
	seen := make(map[journal.EventID]struct{}, len(events))
	for _, event := range events {
		if _, duplicate := seen[event.EventID]; duplicate {
			return fmt.Errorf("%w: append batch repeats EventID %q", journal.ErrInvalidEvent, event.EventID)
		}
		seen[event.EventID] = struct{}{}
		existing, found, err := eventByID(ctx, queries, event.EventID)
		if err != nil {
			return err
		}
		if found {
			if !sameEvent(existing.ProposedEvent, event) {
				return fmt.Errorf("%w: EventID %q already identifies different content", journal.ErrEventConflict, event.EventID)
			}
			continue
		}
		sequence, err := nextStreamSequence(ctx, queries, event.ZoneID, event.StreamID)
		if err != nil {
			return err
		}
		eventSequence, err := nextEventSequence(ctx, queries, event.ZoneID, event.Type)
		if err != nil {
			return err
		}
		if err := queries.CreateEvent(ctx, db.CreateEventParams{
			EventID:        string(event.EventID),
			ZoneID:         string(event.ZoneID),
			StreamID:       string(event.StreamID),
			StreamSequence: sequence,
			EventType:      string(event.Type),
			EventSequence:  eventSequence,
			SchemaVersion:  int64(event.SchemaVersion),
			OccurredAt:     event.OccurredAt.UTC().Format(time.RFC3339Nano),
			CorrelationID:  string(event.CorrelationID),
			CausationID:    string(event.CausationID),
			Body:           event.Body,
		}); err != nil {
			return fmt.Errorf("append Journal EventID %q: %w", event.EventID, err)
		}
	}
	return nil
}

func nextEventSequence(
	ctx context.Context,
	queries *db.Queries,
	zoneID journal.ZoneID,
	eventType journal.EventType,
) (int64, error) {
	position, err := queries.GetEventTypePosition(ctx, db.GetEventTypePositionParams{
		ZoneID:    string(zoneID),
		EventType: string(eventType),
	})
	if err != nil {
		return 0, fmt.Errorf("read Journal EventType %q position: %w", eventType, err)
	}
	if position == math.MaxInt64 {
		return 0, fmt.Errorf("%w: Journal EventType %q sequence exhausted", journal.ErrInvalidEvent, eventType)
	}
	return position, nil
}

func nextStreamSequence(
	ctx context.Context,
	queries *db.Queries,
	zoneID journal.ZoneID,
	streamID journal.StreamID,
) (int64, error) {
	current, err := queries.GetLastStreamSequence(ctx, db.GetLastStreamSequenceParams{
		ZoneID: string(zoneID), StreamID: string(streamID),
	})
	if err != nil {
		return 0, fmt.Errorf("read Journal Stream %q sequence: %w", streamID, err)
	}
	if current == math.MaxInt64 {
		return 0, fmt.Errorf("%w: Journal Stream %q sequence exhausted", journal.ErrInvalidEvent, streamID)
	}
	return current + 1, nil
}

func eventByID(
	ctx context.Context,
	queries *db.Queries,
	eventID journal.EventID,
) (journal.Event, bool, error) {
	row, err := queries.GetEventByID(ctx, string(eventID))
	if errors.Is(err, sql.ErrNoRows) {
		return journal.Event{}, false, nil
	}
	if err != nil {
		return journal.Event{}, false, fmt.Errorf("read Journal EventID %q: %w", eventID, err)
	}
	event, err := storedEvent(row)
	if err != nil {
		return journal.Event{}, false, fmt.Errorf("decode Journal EventID %q: %w", eventID, err)
	}
	return event, true, nil
}

func storedEvent(row db.JournalEvent) (journal.Event, error) {
	if row.EventSequence < 0 || row.StreamSequence <= 0 || row.SchemaVersion <= 0 || row.SchemaVersion > math.MaxUint32 {
		return journal.Event{}, fmt.Errorf("%w: stored Journal ordering or version is invalid", journal.ErrInvalidEvent)
	}
	parsedTime, err := time.Parse(time.RFC3339Nano, row.OccurredAt)
	if err != nil {
		return journal.Event{}, fmt.Errorf("%w: stored Journal OccurredAt is invalid: %v", journal.ErrInvalidEvent, err)
	}
	return journal.Event{
		ProposedEvent: journal.ProposedEvent{
			EventID:       journal.EventID(row.EventID),
			ZoneID:        journal.ZoneID(row.ZoneID),
			StreamID:      journal.StreamID(row.StreamID),
			Type:          journal.EventType(row.EventType),
			SchemaVersion: journal.SchemaVersion(row.SchemaVersion),
			OccurredAt:    parsedTime.UTC(),
			CorrelationID: journal.CorrelationID(row.CorrelationID),
			CausationID:   journal.EventID(row.CausationID),
			Body:          row.Body,
		},
		Sequence:       journal.EventSequence(row.EventSequence),
		StreamSequence: journal.StreamSequence(row.StreamSequence),
	}, nil
}

func sameEvent(left, right journal.ProposedEvent) bool {
	return left.EventID == right.EventID &&
		left.ZoneID == right.ZoneID &&
		left.StreamID == right.StreamID &&
		left.Type == right.Type &&
		left.SchemaVersion == right.SchemaVersion &&
		left.OccurredAt.Equal(right.OccurredAt) &&
		left.CorrelationID == right.CorrelationID &&
		left.CausationID == right.CausationID &&
		left.Body == right.Body
}
