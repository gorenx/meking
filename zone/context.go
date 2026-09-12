package zone

import (
	"context"
	"fmt"
)

// Context is a standard Context bound to one immutable Zone identity.
type Context interface {
	context.Context
	ZoneID() ID
	ChildZoneID() ID
}

// ZoneContext carries cancellation, deadlines, values, and Zone scope.
type ZoneContext struct {
	context.Context
	id      ID
	childID ID
}

type contextKey struct{}
type childContextKey struct{}

// NewContext binds one validated Zone identity to a parent Context.
func NewContext(parent context.Context, id ID) (*ZoneContext, error) {
	if parent == nil {
		return nil, ErrContextRequired
	}
	validated, err := ParseID(string(id))
	if err != nil {
		return nil, err
	}
	if current, currentErr := RequireID(parent); currentErr == nil {
		if current != validated {
			return nil, fmt.Errorf(
				"%w: current %q, requested %q",
				ErrContextConflict,
				current,
				validated,
			)
		}
		childID, _ := RequireChildID(parent)
		return &ZoneContext{Context: parent, id: validated, childID: childID}, nil
	}
	bound := context.WithValue(parent, contextKey{}, validated)
	return &ZoneContext{Context: bound, id: validated}, nil
}

// WithChildZone binds the Child source of one merge operation without changing
// the current Zone identity.
func WithChildZone(parent context.Context, childID ID) (*ZoneContext, error) {
	current, err := RequireID(parent)
	if err != nil {
		return nil, err
	}
	validated, err := ParseID(string(childID))
	if err != nil {
		return nil, err
	}
	if validated == current {
		return nil, fmt.Errorf("%w: current Zone and Child Zone are both %q", ErrContextConflict, current)
	}
	if existing, existingErr := RequireChildID(parent); existingErr == nil {
		if existing != validated {
			return nil, fmt.Errorf(
				"%w: current Child %q, requested Child %q",
				ErrContextConflict,
				existing,
				validated,
			)
		}
		return &ZoneContext{Context: parent, id: current, childID: existing}, nil
	}
	bound := context.WithValue(parent, childContextKey{}, validated)
	return &ZoneContext{Context: bound, id: current, childID: validated}, nil
}

// RouteContext explicitly routes one cross-Zone application operation to a
// target Zone while preserving cancellation, deadlines, and request values.
// Ordinary delivery adapters must use NewContext so accidental rebinding is
// still rejected.
func RouteContext(parent context.Context, id ID) (*ZoneContext, error) {
	if parent == nil {
		return nil, ErrContextRequired
	}
	validated, err := ParseID(string(id))
	if err != nil {
		return nil, err
	}
	withoutChild := context.WithValue(parent, childContextKey{}, ID(""))
	bound := context.WithValue(withoutChild, contextKey{}, validated)
	return &ZoneContext{Context: bound, id: validated}, nil
}

// RequireID resolves the Zone identity carried by ctx.
func RequireID(ctx context.Context) (ID, error) {
	if ctx == nil {
		return "", ErrContextRequired
	}
	id, ok := ctx.Value(contextKey{}).(ID)
	if !ok {
		return "", ErrContextRequired
	}
	validated, err := ParseID(string(id))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrContextRequired, err)
	}
	return validated, nil
}

// RequireChildID resolves the Child source carried separately by ctx.
func RequireChildID(ctx context.Context) (ID, error) {
	if ctx == nil {
		return "", ErrChildContextRequired
	}
	id, ok := ctx.Value(childContextKey{}).(ID)
	if !ok {
		return "", ErrChildContextRequired
	}
	validated, err := ParseID(string(id))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrChildContextRequired, err)
	}
	return validated, nil
}

// ZoneID returns the identity bound to this ZoneContext.
func (zoneContext *ZoneContext) ZoneID() ID {
	if zoneContext == nil {
		return ""
	}
	return zoneContext.id
}

// ChildZoneID returns the separately bound Child source, or an empty identity
// when this is an ordinary Zone-scoped operation.
func (zoneContext *ZoneContext) ChildZoneID() ID {
	if zoneContext == nil {
		return ""
	}
	return zoneContext.childID
}
