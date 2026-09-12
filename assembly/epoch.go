package assembly

import (
	communitysqlite "github.com/memoria-space/meking/community/adapter/sqlite"
	"github.com/memoria-space/meking/corpus"
	textunitvectors "github.com/memoria-space/meking/corpus/textunits/vectorindex"
	"github.com/memoria-space/meking/epoch"
	epochjournal "github.com/memoria-space/meking/epoch/adapter/journal"
	epochreadiness "github.com/memoria-space/meking/epoch/adapter/readiness"
	epochsqlite "github.com/memoria-space/meking/epoch/adapter/sqlite"
	epochlifecycle "github.com/memoria-space/meking/epoch/lifecycle"
	"github.com/memoria-space/meking/knowledge"
	entityvectors "github.com/memoria-space/meking/knowledge/vectorindex"
)

type epochApplications struct {
	reader            *epoch.Reader
	publicationAction *epochjournal.PublicationAction
}

func openEpoch(
	resources databaseResources,
	events eventJournal,
	corpora *corpus.Service,
	knowledgeReader *knowledge.Reader,
	communityStore *communitysqlite.Store,
	vectors *vectorDatabases,
) (epochApplications, error) {
	store, err := epochsqlite.NewStore(resources.database)
	if err != nil {
		return epochApplications{}, err
	}
	entities, err := entityvectors.NewValidator(knowledgeReader, vectors.Entities)
	if err != nil {
		return epochApplications{}, err
	}
	textUnits, err := textunitvectors.NewValidator(vectors.TextUnits)
	if err != nil {
		return epochApplications{}, err
	}
	readiness, err := epochreadiness.NewPublicationReadiness(
		corpora,
		communityStore,
		communityStore,
		entities,
		textUnits,
	)
	if err != nil {
		return epochApplications{}, err
	}
	producer, err := epochjournal.NewProducer(events.publisher)
	if err != nil {
		return epochApplications{}, err
	}
	application, err := epochlifecycle.NewApplication(epochlifecycle.ApplicationDependencies{
		Transactions: resources.transactions,
		Store:        store,
		Readiness:    readiness,
		Producer:     producer,
	})
	if err != nil {
		return epochApplications{}, err
	}
	const actionReadLimit = 128
	publicationAction, err := epochjournal.NewPublicationAction(
		epochjournal.PublicationActionDependencies{
			Journal:   events.service,
			Positions: events.positions,
			Publisher: application,
			ReadLimit: actionReadLimit,
		},
	)
	if err != nil {
		return epochApplications{}, err
	}
	reader, err := epoch.NewReader(store)
	if err != nil {
		return epochApplications{}, err
	}
	return epochApplications{
		reader:            reader,
		publicationAction: publicationAction,
	}, nil
}
