package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/journal/adapter/sqlite/internal/db"
)

func (store *Store) EventTypePosition(
	ctx context.Context,
	zoneID journal.ZoneID,
	eventType journal.EventType,
) (journal.Position, error) {
	position, err := store.queries.GetEventTypePosition(ctx, db.GetEventTypePositionParams{
		ZoneID:    string(zoneID),
		EventType: string(eventType),
	})
	if err != nil {
		return 0, fmt.Errorf("read Journal EventType %q position: %w", eventType, err)
	}
	if position < 0 {
		return 0, fmt.Errorf("%w: stored EventType position is negative", journal.ErrInvalidEvent)
	}
	return journal.Position(position), nil
}

func (store *Store) ReadPendingEvents(
	ctx context.Context,
	zoneID journal.ZoneID,
	key journal.EventKey,
	position journal.Position,
) (journal.PendingEvents, error) {
	if position > math.MaxInt64 {
		return journal.PendingEvents{}, fmt.Errorf(
			"%w: Position %d exceeds SQLite range",
			journal.ErrInvalidEvent,
			position,
		)
	}
	row, err := store.queries.ReadPendingEvents(ctx, db.ReadPendingEventsParams{
		ZoneID:        string(zoneID),
		EventType:     string(key.Type),
		SchemaVersion: int64(key.Version),
		EventSequence: int64(position),
	})
	if err != nil {
		return journal.PendingEvents{}, err
	}
	if row.EventTypePosition < 0 || row.EventCount < 0 {
		return journal.PendingEvents{}, fmt.Errorf("%w: stored pending event values are negative", journal.ErrInvalidEvent)
	}
	since, err := pendingSince(row.PendingSince)
	if err != nil {
		return journal.PendingEvents{}, err
	}
	return journal.PendingEvents{
		ThroughPosition: journal.Position(row.EventTypePosition),
		Count:           uint64(row.EventCount),
		Since:           since,
	}, nil
}

func pendingSince(value any) (time.Time, error) {
	switch typed := value.(type) {
	case nil:
		return time.Time{}, nil
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, typed)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse pending Journal Event time: %w", err)
		}
		return parsed.UTC(), nil
	case sql.NullString:
		if !typed.Valid {
			return time.Time{}, nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, typed.String)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse pending Journal Event time: %w", err)
		}
		return parsed.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf(
			"%w: unsupported pending Journal Event time %T",
			journal.ErrInvalidEvent,
			value,
		)
	}
}
