package assembly

import (
	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/zone"
)

func openProcessRuntime(
	events eventJournal,
	zones *zone.Catalog,
	domainConsumers []journal.Consumer,
	actionRunners []processRunner,
) (*processRuntime, error) {
	return newProcessRuntime(processRuntimeDependencies{
		Journal:           events.service,
		Zones:             zones,
		ConsumerPositions: events.positions,
		Consumers:         domainConsumers,
		Runners:           actionRunners,
	})
}
