package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/journal/adapter/sqlite/internal/db"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

func (store *Store) Position(
	ctx context.Context,
	consumerID journal.ConsumerID,
	eventType journal.EventType,
) (journal.Position, error) {
	if strings.TrimSpace(string(consumerID)) == "" {
		return 0, fmt.Errorf("%w: ConsumerID is required", journal.ErrConsumerPosition)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return 0, err
	}
	stored, err := store.queries.GetConsumerPosition(ctx, db.GetConsumerPositionParams{
		ZoneID:     string(zoneID),
		ConsumerID: string(consumerID),
		EventType:  string(eventType),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read Journal Consumer %q EventType %q Position: %w", consumerID, eventType, err)
	}
	if stored < 0 {
		return 0, fmt.Errorf("%w: stored Consumer Position is negative", journal.ErrConsumerPosition)
	}
	return journal.Position(stored), nil
}

func (store *Store) Advance(
	ctx context.Context,
	consumerID journal.ConsumerID,
	eventType journal.EventType,
	expected journal.Position,
	position journal.Position,
) error {
	if strings.TrimSpace(string(consumerID)) == "" {
		return fmt.Errorf("%w: ConsumerID is required", journal.ErrConsumerPosition)
	}
	if position <= expected || position > math.MaxInt64 || expected > math.MaxInt64 {
		return fmt.Errorf("%w: Consumer %q cannot advance from %d to %d", journal.ErrConsumerPosition, consumerID, expected, position)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	return store.transactions.WithTx(ctx, func(ctx context.Context) error {
		executor, err := transactionsqlite.Current(ctx, store.database)
		if err != nil {
			return fmt.Errorf("join Journal Consumer Position transaction: %w", err)
		}
		queries := db.New(executor)
		head, err := queries.GetEventTypePosition(ctx, db.GetEventTypePositionParams{
			ZoneID:    string(zoneID),
			EventType: string(eventType),
		})
		if err != nil {
			return fmt.Errorf("read Journal EventType %q position: %w", eventType, err)
		}
		if position > journal.Position(head) {
			return fmt.Errorf("%w: Consumer Position %d exceeds EventType %q Position %d", journal.ErrConsumerPosition, position, eventType, head)
		}
		if err := queries.EnsureConsumerPosition(ctx, db.EnsureConsumerPositionParams{
			ZoneID:     string(zoneID),
			ConsumerID: string(consumerID),
			EventType:  string(eventType),
		}); err != nil {
			return fmt.Errorf("initialize Journal Consumer %q EventType %q Position: %w", consumerID, eventType, err)
		}
		updated, err := queries.AdvanceConsumerPosition(ctx, db.AdvanceConsumerPositionParams{
			ZoneID:           string(zoneID),
			ConsumerID:       string(consumerID),
			EventType:        string(eventType),
			Position:         int64(position),
			ExpectedPosition: int64(expected),
		})
		if err != nil {
			return fmt.Errorf("advance Journal Consumer %q EventType %q Position: %w", consumerID, eventType, err)
		}
		if updated != 1 {
			return fmt.Errorf("%w: Consumer %q EventType %q Position changed from %d", journal.ErrConsumerPosition, consumerID, eventType, expected)
		}
		return nil
	})
}

func (store *Store) PendingZone(
	ctx context.Context,
	consumerID journal.ConsumerID,
	key journal.EventKey,
) (journal.ZoneID, error) {
	value, err := store.queries.GetPendingConsumerZone(ctx, db.GetPendingConsumerZoneParams{
		ConsumerID:   string(consumerID),
		EventType:    string(key.Type),
		SchemaVersion: int64(key.Version),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read Journal Consumer %q pending Zone: %w", consumerID, err)
	}
	zoneID, err := zone.ParseID(value)
	if err != nil {
		return "", fmt.Errorf("read Journal Consumer %q pending Zone: %w", consumerID, err)
	}
	return zoneID, nil
}
