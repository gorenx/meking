package sqlite

import (
	"context"
	"fmt"
	"math"

	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/journal/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/zone"
)

func (store *Store) ReadEventType(
	ctx context.Context,
	zoneID journal.ZoneID,
	eventType journal.EventType,
	from journal.Position,
	limit int,
) ([]journal.Event, error) {
	if from > math.MaxInt64 {
		return nil, fmt.Errorf("%w: Position %d exceeds SQLite range", journal.ErrInvalidEvent, from)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: read limit must be positive", journal.ErrInvalidEvent)
	}
	rows, err := store.queries.ListEventTypeEvents(ctx, db.ListEventTypeEventsParams{
		ZoneID:    string(zoneID),
		EventType: string(eventType),
		Position:  int64(from),
		ReadLimit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("read Journal EventType %q from Position %d: %w", eventType, from, err)
	}
	return storedEvents(rows)
}

func (store *Store) ReadEntries(
	ctx context.Context,
	zoneID journal.ZoneID,
	offset uint64,
	limit int,
) ([]journal.Event, error) {
	if offset > math.MaxInt64 {
		return nil, fmt.Errorf("%w: Offset %d exceeds SQLite range", journal.ErrInvalidEvent, offset)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: read limit must be positive", journal.ErrInvalidEvent)
	}
	rows, err := store.queries.ListJournalEntries(ctx, db.ListJournalEntriesParams{
		ZoneID:      string(zoneID),
		ReadLimit:   int64(limit),
		EntryOffset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("browse Journal entries from Offset %d: %w", offset, err)
	}
	return storedEvents(rows)
}

func storedEvents(rows []db.JournalEvent) ([]journal.Event, error) {
	events := make([]journal.Event, 0, len(rows))
	for _, row := range rows {
		event, err := storedEvent(row)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (store *Store) ReadStreamAfter(
	ctx context.Context,
	streamID journal.StreamID,
	after journal.StreamSequence,
	limit int,
) ([]journal.Event, error) {
	if after > math.MaxInt64 {
		return nil, fmt.Errorf("%w: Stream Sequence %d exceeds SQLite range", journal.ErrInvalidEvent, after)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: read limit must be positive", journal.ErrInvalidEvent)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read Journal Stream %q: %w", streamID, err)
	}
	rows, err := store.queries.ListStreamEventsAfter(ctx, db.ListStreamEventsAfterParams{
		ZoneID:         string(zoneID),
		StreamID:       string(streamID),
		StreamSequence: int64(after),
		Limit:          int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("read Journal Stream %q after Sequence %d: %w", streamID, after, err)
	}
	return storedEvents(rows)
}
