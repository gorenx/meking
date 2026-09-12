package assembly

import (
	"fmt"

	"github.com/memoria-space/meking/zone"
	zonesqlite "github.com/memoria-space/meking/zone/adapter/sqlite"
)

type zoneApplications struct {
	catalog *zone.Catalog
}

func openZones(resources databaseResources) (zoneApplications, error) {
	definitions, err := zonesqlite.NewStore(resources.database)
	if err != nil {
		return zoneApplications{}, fmt.Errorf("open Zone Definition Store: %w", err)
	}
	catalog, err := zone.NewCatalog(zone.CatalogDependencies{Definitions: definitions})
	if err != nil {
		return zoneApplications{}, fmt.Errorf("open Zone Catalog: %w", err)
	}
	return zoneApplications{catalog: catalog}, nil
}
