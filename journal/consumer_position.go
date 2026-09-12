package journal

import "context"

// ConsumerPositions records the next EventType sequence each consumer will
// process in each Zone.
type ConsumerPositions interface {
	Position(context.Context, ConsumerID, EventType) (Position, error)
	Advance(context.Context, ConsumerID, EventType, Position, Position) error
	PendingZone(context.Context, ConsumerID, EventKey) (ZoneID, error)
}
