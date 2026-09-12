package assembly

import (
	"log/slog"

	"github.com/memoria-space/meking/agent"
	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge"
	knowledgecorpus "github.com/memoria-space/meking/knowledge/adapter/corpus"
	knowledgejournal "github.com/memoria-space/meking/knowledge/adapter/journal"
	knowledgesqlite "github.com/memoria-space/meking/knowledge/adapter/sqlite"
	"github.com/memoria-space/meking/knowledge/deletion"
	"github.com/memoria-space/meking/knowledge/extraction"
	extractionmodel "github.com/memoria-space/meking/knowledge/extraction/adapter/agent"
	extractioncorpus "github.com/memoria-space/meking/knowledge/extraction/adapter/corpus"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
	entityvectors "github.com/memoria-space/meking/knowledge/vectorindex"
	knowledgemerger "github.com/memoria-space/meking/knowledge/zonemerger"
	"github.com/memoria-space/meking/project"
	"github.com/memoria-space/meking/semantic"
)

type knowledgeApplications struct {
	reader             *knowledge.Reader
	versions           knowledge.CurrentVersions
	provenance         *provenance.Reader
	submissions        *submission.Application
	resolutions        *resolution.Application
	deletions          *deletion.Application
	extractionAction   *knowledgejournal.ExtractionAction
	entityVectorAction *knowledgejournal.EntityVectorAction
	entityVectors      *entityvectors.Service
	merger             *knowledgemerger.Application
}

type knowledgeDependencies struct {
	databaseResources
	eventJournal
	configuration  project.Configuration
	corpora        *corpus.Service
	messages       *message.Log
	completion     *agent.OpenAICompletion
	entitySemantic *semantic.Service
	logger         *slog.Logger
}

func openKnowledge(dependencies knowledgeDependencies) (knowledgeApplications, []journal.Consumer, error) {
	database, err := knowledgesqlite.New(dependencies.database)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	candidates := database.Candidates()
	reader, err := knowledge.NewReader(database)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	provenanceReader, err := provenance.NewReader(database)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	evidence, err := knowledgecorpus.NewEvidenceVerifier(dependencies.corpora, dependencies.messages)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	submissions, err := submission.New(submission.Dependencies{
		Tx:                  dependencies.transactions,
		KnowledgeIdentities: database,
		VersionHistory:      database,
		Sources:             database,
		Candidates:          candidates,
		EvidenceVerifier:    evidence,
	})
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	resolutions, err := resolution.New(resolution.Dependencies{
		Tx:                  dependencies.transactions,
		KnowledgeIdentities: database,
		VersionHistory:      database,
		Candidates:          candidates,
		Provenance:          database,
	})
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	deletions, err := deletion.New(deletion.Dependencies{
		Tx:                  dependencies.transactions,
		VersionHistory:      database,
		KnowledgeReferences: database,
		Provenance:          database,
	})
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	merger, err := knowledgemerger.New(knowledgemerger.Dependencies{
		Tx:               dependencies.transactions,
		Conflicts:        database,
		Provenance:       provenanceReader,
		Identities:       database,
		CurrentVersions:  database,
		CurrentKnowledge: reader,
		Submissions:      submissions,
		Resolutions:      resolutions,
	})
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	producer, err := knowledgejournal.NewProducer(dependencies.publisher)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	extractionTextUnits, err := extractioncorpus.NewReader(dependencies.corpora)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	extractionCompletion, err := extractionmodel.NewCompletion(dependencies.completion)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	extractionModel, err := extraction.NewModel(
		extractionCompletion,
		dependencies.configuration.MaxConcurrent,
	)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	extractionService, err := extraction.NewService(extractionModel, dependencies.configuration.Extraction)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	extractions, err := extraction.NewApplication(extraction.Dependencies{
		Tx:                   dependencies.transactions,
		TextUnitReader:       extractionTextUnits,
		Progress:             database,
		KnowledgeSubmissions: submissions,
		Manifests:            database,
		Producer:             producer,
		Extractor:            extractionService,
	})
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	const actionReadLimit = 128
	extractionAction, err := knowledgejournal.NewExtractionAction(
		knowledgejournal.ExtractionActionDependencies{
			Journal:    dependencies.service,
			Positions:  dependencies.positions,
			Extraction: extractions,
			ReadLimit:  actionReadLimit,
			Logger:     dependencies.logger,
		},
	)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	entityVectorService, err := entityvectors.New(reader, dependencies.entitySemantic)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	entityVectorAction, err := knowledgejournal.NewEntityVectorAction(
		knowledgejournal.EntityVectorIndexing{
			Tx:        dependencies.transactions,
			Journal:   dependencies.service,
			Positions: dependencies.positions,
			Vectors:   entityVectorService,
			Events:    producer,
			ReadLimit: actionReadLimit,
		},
	)
	if err != nil {
		return knowledgeApplications{}, nil, err
	}
	return knowledgeApplications{
		reader:             reader,
		versions:           database,
		provenance:         provenanceReader,
		submissions:        submissions,
		resolutions:        resolutions,
		deletions:          deletions,
		extractionAction:   extractionAction,
		entityVectorAction: entityVectorAction,
		entityVectors:      entityVectorService,
		merger:             merger,
	}, nil, nil
}
