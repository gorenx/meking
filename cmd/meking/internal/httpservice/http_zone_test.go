package httpservice

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/memoria-space/meking/zone"
)

type memoryZoneDefinitions struct {
	definitions []zone.Definition
}

func (store *memoryZoneDefinitions) Create(_ context.Context, definition zone.Definition) error {
	for _, existing := range store.definitions {
		if existing.ID == definition.ID {
			return errors.New("duplicate Zone")
		}
	}
	store.definitions = append(store.definitions, definition)
	return nil
}

func (store *memoryZoneDefinitions) Resolve(_ context.Context, id zone.ID) (zone.Definition, error) {
	for _, definition := range store.definitions {
		if definition.ID == id {
			return definition, nil
		}
	}
	return zone.Definition{}, zone.ErrNotFound
}

func (store *memoryZoneDefinitions) ResolveRoot(_ context.Context, userID string) (zone.Definition, error) {
	for _, definition := range store.definitions {
		if definition.Role == zone.RoleRoot && definition.UserID == userID {
			return definition, nil
		}
	}
	return zone.Definition{}, zone.ErrNotFound
}

func (store *memoryZoneDefinitions) List(context.Context) ([]zone.Definition, error) {
	return append([]zone.Definition(nil), store.definitions...), nil
}

func TestHTTPZoneCatalogCreatesAndResolvesRootAndChild(t *testing.T) {
	store := &memoryZoneDefinitions{}
	catalog, err := zone.NewCatalog(zone.CatalogDependencies{Definitions: store})
	if err != nil {
		t.Fatal(err)
	}
	dependencies := defaultHTTPDependencies()
	dependencies.zones = catalog
	dependencies.zoneCatalog = catalog
	handler, err := newHTTPHandler(httpTestConfig(t), []string{"example.com"}, dependencies)
	if err != nil {
		t.Fatal(err)
	}

	rootResponse := serveHTTPRequest(t, handler, http.MethodPost, "/api/v1/zones", []byte(`{}`))
	var root httpZoneDefinition
	decodeHTTPTestResponse(t, rootResponse, &root)
	if rootResponse.Code != http.StatusCreated || root.Role != "root" || root.ParentZoneID != nil {
		t.Fatalf("root status/body = %d/%#v", rootResponse.Code, root)
	}

	childResponse := serveHTTPRequest(
		t, handler, http.MethodPost, "/api/v1/zones",
		[]byte(`{"parent_zone_id":"`+root.ID+`"}`),
	)
	var child httpZoneDefinition
	decodeHTTPTestResponse(t, childResponse, &child)
	if childResponse.Code != http.StatusCreated || child.Role != "child" ||
		child.ParentZoneID == nil || *child.ParentZoneID != root.ID {
		t.Fatalf("child status/body = %d/%#v", childResponse.Code, child)
	}

	listResponse := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones", nil)
	var list httpZoneDefinitionList
	decodeHTTPTestResponse(t, listResponse, &list)
	if listResponse.Code != http.StatusOK || len(list.Zones) != 2 {
		t.Fatalf("list status/body = %d/%#v", listResponse.Code, list)
	}

	getResponse := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/"+child.ID, nil)
	var resolved httpZoneDefinition
	decodeHTTPTestResponse(t, getResponse, &resolved)
	if getResponse.Code != http.StatusOK || resolved.ID != child.ID {
		t.Fatalf("resolve status/body = %d/%#v", getResponse.Code, resolved)
	}

	grandchildResponse := serveHTTPRequest(
		t, handler, http.MethodPost, "/api/v1/zones",
		[]byte(`{"parent_zone_id":"`+child.ID+`"}`),
	)
	if grandchildResponse.Code != http.StatusConflict || len(store.definitions) != 2 {
		t.Fatalf("grandchild status/definitions = %d/%d", grandchildResponse.Code, len(store.definitions))
	}
}
