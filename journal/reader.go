package journal

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/zone"
)

// EventReader reads only committed Journal events.
type EventReader interface {
	ReadEventType(
		ctx context.Context,
		zoneID ZoneID,
		eventType EventType,
		from Position,
		limit int,
	) ([]Event, error)
	ReadEntries(ctx context.Context, zoneID ZoneID, offset uint64, limit int) ([]Event, error)
	ReadStreamAfter(
		ctx context.Context,
		streamID StreamID,
		after StreamSequence,
		limit int,
	) ([]Event, error)
}

func (s *Service) Events(
	ctx context.Context,
	eventType EventType,
	from Position,
	through Position,
	limit int,
) ([]Event, error) {
	if s == nil || s.reader == nil {
		return nil, errors.New("read Journal events: Service is not configured")
	}
	if err := validateIdentity(string(eventType), "EventType"); err != nil {
		return nil, err
	}
	if through < from {
		return nil, fmt.Errorf(
			"%w: EventType %q Position %d exceeds through %d",
			ErrInvalidEvent,
			eventType,
			from,
			through,
		)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: read limit must be positive", ErrInvalidEvent)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, err
	}
	events, err := s.reader.ReadEventType(ctx, zoneID, eventType, from, limit)
	if err != nil {
		return nil, fmt.Errorf(
			"read Journal EventType %q from Position %d: %w",
			eventType,
			from,
			err,
		)
	}
	previous := from
	for index, event := range events {
		if err := validateZoneID(event.ZoneID); err != nil {
			return nil, fmt.Errorf("stored event %d: %w", index, err)
		}
		if event.Type != eventType {
			return nil, fmt.Errorf(
				"%w: stored event %d has EventType %q, expected %q",
				ErrInvalidEvent,
				index,
				event.Type,
				eventType,
			)
		}
		position := Position(event.Sequence)
		if position < from || (index > 0 && position <= previous) {
			return nil, fmt.Errorf(
				"%w: stored event %d is not ordered from Position %d",
				ErrInvalidEvent,
				index,
				from,
			)
		}
		previous = position
	}
	return TrimThrough(events, through), nil
}

func (s *Service) Entries(
	ctx context.Context,
	offset uint64,
	limit int,
) ([]Event, error) {
	if s == nil || s.reader == nil {
		return nil, errors.New("browse Journal events: Service is not configured")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: read limit must be positive", ErrInvalidEvent)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, err
	}
	events, err := s.reader.ReadEntries(ctx, zoneID, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("browse Journal events from Offset %d: %w", offset, err)
	}
	for index, event := range events {
		if event.ZoneID != zoneID {
			return nil, fmt.Errorf(
				"%w: browsed event %d has inconsistent Zone",
				ErrInvalidEvent,
				index,
			)
		}
	}
	return events, nil
}

func (s *Service) StreamEventsAfter(
	ctx context.Context,
	streamID StreamID,
	after StreamSequence,
	limit int,
) ([]Event, error) {
	if s == nil || s.reader == nil {
		return nil, errors.New("read Journal Stream: Service is not configured")
	}
	if err := validateIdentity(string(streamID), "StreamID"); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: read limit must be positive", ErrInvalidEvent)
	}
	events, err := s.reader.ReadStreamAfter(ctx, streamID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("read Journal Stream %q after Sequence %d: %w", streamID, after, err)
	}
	previousSequence := after
	for index, event := range events {
		if event.StreamID != streamID {
			return nil, fmt.Errorf("%w: stored Stream event %d has inconsistent ownership", ErrInvalidEvent, index)
		}
		if previousSequence == ^StreamSequence(0) || event.StreamSequence != previousSequence+1 {
			return nil, fmt.Errorf("%w: stored Stream event %d is not contiguous after Sequence %d", ErrInvalidEvent, index, previousSequence)
		}
		previousSequence = event.StreamSequence
	}
	return events, nil
}
