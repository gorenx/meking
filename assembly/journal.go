package assembly

import (
	communityjournal "github.com/memoria-space/meking/community/adapter/journal"
	controljournal "github.com/memoria-space/meking/controlplane/adapter/journal"
	corpusjournal "github.com/memoria-space/meking/corpus/adapter/journal"
	epochjournal "github.com/memoria-space/meking/epoch/adapter/journal"
	"github.com/memoria-space/meking/journal"
	journalsqlite "github.com/memoria-space/meking/journal/adapter/sqlite"
	knowledgejournal "github.com/memoria-space/meking/knowledge/adapter/journal"
)

type eventJournal struct {
	service       *journal.Service
	publisher     journal.Publisher
	positions     journal.ConsumerPositions
	actionSignals *controljournal.ActionSignals
}

func openJournal(resources databaseResources) (eventJournal, error) {
	store, err := journalsqlite.NewStore(resources.database, resources.transactions)
	if err != nil {
		return eventJournal{}, err
	}
	contracts := append([]journal.EventContractRegistration{}, corpusjournal.EventContracts()...)
	contracts = append(contracts, knowledgejournal.EventContracts()...)
	contracts = append(contracts, communityjournal.EventContracts()...)
	contracts = append(contracts, epochjournal.EventContracts()...)
	service, err := journal.NewService(journal.ServiceDependencies{
		Contracts:     contracts,
		AtomicBatches: knowledgejournal.AtomicBatchContracts(),
		Reader:        store,
	})
	if err != nil {
		return eventJournal{}, err
	}
	basePublisher, err := journal.NewPublisher(service, store)
	if err != nil {
		return eventJournal{}, err
	}
	actionSignals := controljournal.NewActionSignals()
	publisher, err := controljournal.NewActionPublisher(basePublisher, actionSignals)
	if err != nil {
		return eventJournal{}, err
	}
	return eventJournal{
		service:       service,
		publisher:     publisher,
		positions:     store,
		actionSignals: actionSignals,
	}, nil
}
