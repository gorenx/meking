package assembly

import (
	"log/slog"

	"github.com/memoria-space/meking/agent"
	communityagent "github.com/memoria-space/meking/community/adapter/agent"
	communitycorpus "github.com/memoria-space/meking/community/adapter/corpus"
	communityjournal "github.com/memoria-space/meking/community/adapter/journal"
	communityknowledge "github.com/memoria-space/meking/community/adapter/knowledge"
	"github.com/memoria-space/meking/community/adapter/sqlite"
	communityreport "github.com/memoria-space/meking/community/report"
	reportvectors "github.com/memoria-space/meking/community/report/vectorindex"
	"github.com/memoria-space/meking/community/reportgeneration"
	"github.com/memoria-space/meking/community/structurebuild"
	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/project"
	"github.com/memoria-space/meking/semantic"
)

type communityApplications struct {
	store               *sqlite.Store
	reports             *communityreport.Archive
	structureDerivation *communityjournal.StructureDerivationAction
	reportGeneration    *communityjournal.ReportGenerationAction
}

func openCommunity(
	resources databaseResources,
	configuration project.Configuration,
	events eventJournal,
	corpora *corpus.Service,
	knowledge knowledgeApplications,
	completion *agent.OpenAICompletion,
	embedding semantic.Embedder,
	tokens *textunits.TiktokenTokenizer,
	vectors *vectorDatabase,
	logger *slog.Logger,
) (communityApplications, error) {
	store, err := sqlite.NewStore(resources.database)
	if err != nil {
		return communityApplications{}, err
	}
	producer, err := communityjournal.NewProducer(events.publisher)
	if err != nil {
		return communityApplications{}, err
	}
	knowledgeInput, err := communityknowledge.NewReader(
		knowledge.reader,
		knowledge.provenance,
	)
	if err != nil {
		return communityApplications{}, err
	}
	reports, err := communityreport.NewArchive(store, logger)
	if err != nil {
		return communityApplications{}, err
	}
	corpusInput, err := communitycorpus.NewReader(corpora)
	if err != nil {
		return communityApplications{}, err
	}
	reportModel, err := communityagent.NewReportModel(completion)
	if err != nil {
		return communityApplications{}, err
	}
	reportGenerator, err := communityreport.NewGenerator(
		reportModel,
		tokens,
		configuration.Community.Reports,
	)
	if err != nil {
		return communityApplications{}, err
	}
	var reportVectorService *reportvectors.Service
	if configuration.Community.ReportVectors != nil {
		semanticService, err := vectors.NewService(
			embedding,
			tokens,
			*configuration.Community.ReportVectors,
		)
		if err != nil {
			return communityApplications{}, err
		}
		reportVectorService, err = reportvectors.New(semanticService)
		if err != nil {
			return communityApplications{}, err
		}
	}
	builder, err := structurebuild.NewBuilder(structurebuild.Dependencies{
		Transactions:            resources.transactions,
		Knowledge:               knowledgeInput,
		CommunitySets:           store,
		Structures:              store,
		Producer:                producer,
		Detection:               configuration.Community.Detection,
		RelationChangeThreshold: configuration.Community.RelationChangeThreshold,
	})
	if err != nil {
		return communityApplications{}, err
	}
	reportBuilder, err := reportgeneration.NewBuilder(reportgeneration.Dependencies{
		Transactions:  resources.transactions,
		Structures:    store,
		CommunitySets: store,
		Knowledge:     knowledgeInput,
		Corpus:        corpusInput,
		Generator:     reportGenerator,
		Reports:       reports,
		ReportSets:    store,
		Publications:  store,
		Vectors:       reportVectorService,
	})
	if err != nil {
		return communityApplications{}, err
	}
	const actionReadLimit = 128
	structureDerivation, err := communityjournal.NewStructureDerivationAction(
		communityjournal.StructureDerivationActionDependencies{
			Journal:   events.service,
			Positions: events.positions,
			Builder:   builder,
			ReadLimit: actionReadLimit,
		},
	)
	if err != nil {
		return communityApplications{}, err
	}
	reportGeneration, err := communityjournal.NewReportGenerationAction(
		communityjournal.ReportGenerationActionDependencies{
			Journal:   events.service,
			Positions: events.positions,
			Builder:   reportBuilder,
			ReadLimit: actionReadLimit,
		},
	)
	if err != nil {
		return communityApplications{}, err
	}
	return communityApplications{
		store:               store,
		reports:             reports,
		structureDerivation: structureDerivation,
		reportGeneration:    reportGeneration,
	}, nil
}
