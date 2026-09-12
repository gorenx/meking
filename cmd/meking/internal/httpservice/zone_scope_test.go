package httpservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memoria-space/meking/zone"
)

const httpTestZoneID zone.ID = "10000000-0000-4000-8000-000000000001"

type zoneResolverFunc func(context.Context, zone.ID) (zone.Definition, error)

func (resolve zoneResolverFunc) Resolve(
	ctx context.Context,
	id zone.ID,
) (zone.Definition, error) {
	return resolve(ctx, id)
}

func TestZoneScopeMiddlewareBindsResolvedZone(t *testing.T) {
	resolver := zoneResolverFunc(func(_ context.Context, id zone.ID) (zone.Definition, error) {
		return zone.NewRootDefinition(id, "", time.Now().UTC())
	})
	nextCalled := false
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		nextCalled = true
		id, err := zone.RequireID(request.Context())
		if err != nil || id != httpTestZoneID {
			t.Fatalf("RequireID() = %q, %v", id, err)
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	handler, err := zoneScopeMiddleware(resolver, next)
	if err != nil {
		t.Fatalf("zoneScopeMiddleware() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("zone_id", string(httpTestZoneID))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !nextCalled {
		t.Fatalf("status/nextCalled = %d/%t", response.Code, nextCalled)
	}
}

func TestZoneScopeMiddlewareRejectsInvalidAndMissingZone(t *testing.T) {
	resolverCalled := false
	resolver := zoneResolverFunc(func(_ context.Context, _ zone.ID) (zone.Definition, error) {
		resolverCalled = true
		return zone.Definition{}, zone.ErrNotFound
	})
	handler, err := zoneScopeMiddleware(resolver, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next Handler was called")
	}))
	if err != nil {
		t.Fatal(err)
	}

	invalid := httptest.NewRequest(http.MethodGet, "/", nil)
	invalid.SetPathValue("zone_id", "invalid")
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest || resolverCalled {
		t.Fatalf("invalid status/resolverCalled = %d/%t", invalidResponse.Code, resolverCalled)
	}

	missing := httptest.NewRequest(http.MethodGet, "/", nil)
	missing.SetPathValue("zone_id", string(httpTestZoneID))
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusNotFound || !resolverCalled {
		t.Fatalf("missing status/resolverCalled = %d/%t", missingResponse.Code, resolverCalled)
	}
}

func TestZoneScopeMiddlewareRequiresDependencies(t *testing.T) {
	if _, err := zoneScopeMiddleware(nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); err == nil {
		t.Fatal("zoneScopeMiddleware(nil resolver) accepted invalid dependencies")
	}
	resolver := zoneResolverFunc(func(context.Context, zone.ID) (zone.Definition, error) {
		return zone.Definition{}, errors.New("unused")
	})
	if _, err := zoneScopeMiddleware(resolver, nil); err == nil {
		t.Fatal("zoneScopeMiddleware(nil next) accepted invalid dependencies")
	}
}
