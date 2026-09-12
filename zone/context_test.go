package zone

import (
	"context"
	"errors"
	"testing"
)

func TestContextCarriesZoneThroughDerivedContexts(t *testing.T) {
	t.Parallel()

	zoneContext, err := NewContext(t.Context(), rootID)
	if err != nil {
		t.Fatalf("NewContext() error = %v", err)
	}
	derived, cancel := context.WithCancel(zoneContext)
	cancel()
	if id, err := RequireID(derived); err != nil || id != rootID {
		t.Fatalf("RequireID(derived) = %q, %v", id, err)
	}
	if zoneContext.ZoneID() != rootID {
		t.Fatalf("ZoneID() = %q", zoneContext.ZoneID())
	}
	if !errors.Is(derived.Err(), context.Canceled) {
		t.Fatalf("derived context error = %v", derived.Err())
	}
}

func TestContextRejectsMissingAndConflictingZone(t *testing.T) {
	t.Parallel()

	if _, err := RequireID(t.Context()); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("RequireID(unscoped) error = %v", err)
	}
	rootContext, err := NewContext(t.Context(), rootID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewContext(rootContext, childID); !errors.Is(err, ErrContextConflict) {
		t.Fatalf("NewContext(conflict) error = %v", err)
	}
	if rebound, err := NewContext(rootContext, rootID); err != nil || rebound.ZoneID() != rootID {
		t.Fatalf("NewContext(same Zone) = %#v, %v", rebound, err)
	}
}

func TestRouteContextExplicitlyRoutesAcrossZones(t *testing.T) {
	t.Parallel()

	childContext, err := NewContext(t.Context(), childID)
	if err != nil {
		t.Fatal(err)
	}
	parentContext, err := RouteContext(childContext, rootID)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := RequireID(parentContext); err != nil || id != rootID {
		t.Fatalf("RequireID(routed) = %q, %v", id, err)
	}
	if id, err := RequireID(childContext); err != nil || id != childID {
		t.Fatalf("RequireID(source) = %q, %v", id, err)
	}
}

func TestContextCarriesChildIdentitySeparately(t *testing.T) {
	t.Parallel()

	parent, err := NewContext(t.Context(), rootID)
	if err != nil {
		t.Fatal(err)
	}
	mergeContext, err := WithChildZone(parent, childID)
	if err != nil {
		t.Fatal(err)
	}
	if current, err := RequireID(mergeContext); err != nil || current != rootID {
		t.Fatalf("RequireID() = %q, %v", current, err)
	}
	if child, err := RequireChildID(mergeContext); err != nil || child != childID {
		t.Fatalf("RequireChildID() = %q, %v", child, err)
	}
	if mergeContext.ZoneID() != rootID || mergeContext.ChildZoneID() != childID {
		t.Fatalf("ZoneContext identities = %q, %q", mergeContext.ZoneID(), mergeContext.ChildZoneID())
	}
	if _, err := WithChildZone(parent, rootID); !errors.Is(err, ErrContextConflict) {
		t.Fatalf("WithChildZone(current Zone) error = %v", err)
	}
}

func TestRouteContextClearsChildIdentity(t *testing.T) {
	t.Parallel()

	parent, err := NewContext(t.Context(), rootID)
	if err != nil {
		t.Fatal(err)
	}
	mergeContext, err := WithChildZone(parent, childID)
	if err != nil {
		t.Fatal(err)
	}
	routed, err := RouteContext(mergeContext, childID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RequireChildID(routed); !errors.Is(err, ErrChildContextRequired) {
		t.Fatalf("RequireChildID(routed) error = %v", err)
	}
}
