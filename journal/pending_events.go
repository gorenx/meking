package journal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/zone"
)

// PendingEvents describes exact EventKey facts at or after a consumer's next
// position. ThroughPosition is the EventType position observed by this read.
type PendingEvents struct {
	ThroughPosition Position
	Count           uint64
	Since           time.Time
}

type PendingEventReader interface {
	EventTypePosition(context.Context, ZoneID, EventType) (Position, error)
	ReadPendingEvents(context.Context, ZoneID, EventKey, Position) (PendingEvents, error)
}

func (service *Service) EventTypePosition(
	ctx context.Context,
	eventType EventType,
) (Position, error) {
	if service == nil || service.reader == nil {
		return 0, errors.New("read Journal EventType position: Service is not configured")
	}
	if err := validateIdentity(string(eventType), "EventType"); err != nil {
		return 0, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return 0, err
	}
	reader, supported := service.reader.(PendingEventReader)
	if !supported {
		return service.eventTypePositionByReading(ctx, eventType)
	}
	position, err := reader.EventTypePosition(ctx, zoneID, eventType)
	if err != nil {
		return 0, fmt.Errorf("read Journal EventType %q position: %w", eventType, err)
	}
	return position, nil
}

func (service *Service) PendingEvents(
	ctx context.Context,
	key EventKey,
	position Position,
) (PendingEvents, error) {
	if service == nil || service.reader == nil {
		return PendingEvents{}, errors.New("read pending Journal events: Service is not configured")
	}
	if err := validateEventKey(key); err != nil {
		return PendingEvents{}, err
	}
	if !service.supports(key) {
		return PendingEvents{}, fmt.Errorf(
			"%w: pending EventKey %s v%d is not registered",
			ErrInvalidRegistration,
			key.Type,
			key.Version,
		)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return PendingEvents{}, err
	}
	if reader, supported := service.reader.(PendingEventReader); supported {
		pending, err := reader.ReadPendingEvents(ctx, zoneID, key, position)
		if err != nil {
			return PendingEvents{}, fmt.Errorf(
				"read pending Journal EventKey %s v%d: %w",
				key.Type,
				key.Version,
				err,
			)
		}
		return validatePendingEvents(pending, position)
	}
	through, err := service.EventTypePosition(ctx, key.Type)
	if err != nil {
		return PendingEvents{}, err
	}
	pending, err := service.pendingEventsByReading(ctx, key, position, through)
	if err != nil {
		return PendingEvents{}, err
	}
	return validatePendingEvents(pending, position)
}

func (service *Service) eventTypePositionByReading(
	ctx context.Context,
	eventType EventType,
) (Position, error) {
	const readLimit = 512
	position := Position(0)
	for {
		events, err := service.Events(ctx, eventType, position, ^Position(0), readLimit)
		if err != nil {
			return 0, err
		}
		if len(events) == 0 {
			return position, nil
		}
		position = Position(events[len(events)-1].Sequence) + 1
	}
}

func (service *Service) pendingEventsByReading(
	ctx context.Context,
	key EventKey,
	position Position,
	through Position,
) (PendingEvents, error) {
	const readLimit = 512
	pending := PendingEvents{ThroughPosition: through}
	for position < through {
		events, err := service.Events(ctx, key.Type, position, through, readLimit)
		if err != nil {
			return PendingEvents{}, err
		}
		if len(events) == 0 {
			break
		}
		for _, event := range events {
			position = Position(event.Sequence) + 1
			if event.Key() != key {
				continue
			}
			pending.Count++
			if pending.Since.IsZero() || event.OccurredAt.Before(pending.Since) {
				pending.Since = event.OccurredAt
			}
		}
	}
	return pending, nil
}

func validatePendingEvents(
	pending PendingEvents,
	position Position,
) (PendingEvents, error) {
	if pending.ThroughPosition < position {
		return PendingEvents{}, fmt.Errorf(
			"%w: EventType Position %d precedes Consumer Position %d",
			ErrInvalidEvent,
			pending.ThroughPosition,
			position,
		)
	}
	if pending.Count == 0 {
		if !pending.Since.IsZero() {
			return PendingEvents{}, fmt.Errorf("%w: Since requires pending events", ErrInvalidEvent)
		}
		return pending, nil
	}
	if pending.Since.IsZero() {
		return PendingEvents{}, fmt.Errorf("%w: pending events require Since", ErrInvalidEvent)
	}
	pending.Since = pending.Since.UTC()
	return pending, nil
}
