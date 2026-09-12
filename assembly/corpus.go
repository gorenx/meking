package assembly

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/memoria-space/meking/analysis"
	"github.com/memoria-space/meking/corpus"
	analysisadapter "github.com/memoria-space/meking/corpus/adapter/analysis"
	corpusjournal "github.com/memoria-space/meking/corpus/adapter/journal"
	corpuslocal "github.com/memoria-space/meking/corpus/adapter/local"
	corpussqlite "github.com/memoria-space/meking/corpus/adapter/sqlite"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/corpus/textunits"
	textunitvectors "github.com/memoria-space/meking/corpus/textunits/vectorindex"
	corpusmerger "github.com/memoria-space/meking/corpus/zonemerger"
	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/project"
	"github.com/memoria-space/meking/semantic"
)

type corpusApplications struct {
	service            *corpus.Service
	messages           *message.Log
	documentConversion *corpusjournal.DocumentConversionAction
	textUnitCreation   *corpusjournal.TextUnitCreationAction
	boundaries         *corpusmerger.BoundaryApplication
	merger             *corpusmerger.Application
}

func openCorpus(
	_ context.Context,
	resources databaseResources,
	configuration project.Configuration,
	analysisSession *analysis.Session,
	events eventJournal,
	embedding semantic.Embedder,
	tokens *textunits.TiktokenTokenizer,
	vectors *vectorDatabase,
	logger *slog.Logger,
) (corpusApplications, []journal.Consumer, error) {
	producer, err := corpusjournal.NewProducer(events.publisher)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	store, err := corpussqlite.NewStore(resources.database)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	messages, err := message.NewLog(message.Dependencies{
		Transaction: resources.transactions,
		Entries:     store,
	})
	if err != nil {
		return corpusApplications{}, nil, err
	}
	content, err := corpuslocal.NewContentStore(filepath.Join(
		resources.root,
		configuration.Input.BaseDir,
	))
	if err != nil {
		return corpusApplications{}, nil, err
	}
	documentRepository := store.DocumentRepository()
	documents, err := document.NewManager(documentRepository, content)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	documentCatalog, err := corpussqlite.NewDocumentCatalog(store)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	options := []corpus.ServiceOption{
		corpus.WithDocument(documents),
		corpus.WithDocumentCatalog(documentCatalog),
		corpus.WithChunking(configuration.Chunking),
		corpus.WithLogger(logger),
		corpus.WithEventPublishing(resources.transactions, producer),
	}
	if analysisSession != nil {
		if configuration.Input.Type == project.InputTypeRichDocuments {
			extractor, err := analysisadapter.NewRichTextExtractor(
				analysisSession.Client(),
				configuration.Input.RichDocumentMaxBytes(),
			)
			if err != nil {
				return corpusApplications{}, nil, err
			}
			options = append(options, corpus.WithRichDocumentAnalysis(extractor))
		}
		if configuration.Chunking.Type == textunits.SentenceChunking {
			sentences, err := analysisadapter.NewSentenceAnalyzer(
				analysisSession.Client(),
			)
			if err != nil {
				return corpusApplications{}, nil, err
			}
			options = append(options, corpus.WithSentenceAnalysis(sentences))
		}
	}
	service, err := corpus.NewService(store, options...)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	boundaries, err := corpusmerger.NewBoundaryApplication(resources.transactions, store)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	merger, err := corpusmerger.New(corpusmerger.Dependencies{
		Boundaries: store,
		Corpora:    service,
		Texts:      store.TextStore(),
		Documents:  documents,
		Target:     service,
	})
	if err != nil {
		return corpusApplications{}, nil, err
	}
	const actionReadLimit = 128
	documentConversion, err := corpusjournal.NewDocumentConversionAction(
		corpusjournal.DocumentConversionActionDependencies{
			Journal:   events.service,
			Positions: events.positions,
			Texts:     service,
			Merger:    merger,
			ReadLimit: actionReadLimit,
		},
	)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	textUnitCreation, err := corpusjournal.NewTextUnitCreationAction(
		corpusjournal.TextUnitCreationActionDependencies{
			Journal:   events.service,
			Positions: events.positions,
			TextUnits: service,
			Merger:    merger,
			ReadLimit: actionReadLimit,
		},
	)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	semanticService, err := vectors.NewService(
		embedding,
		tokens,
		configuration.TextUnitVectors,
	)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	vectorService, err := textunitvectors.New(semanticService)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	vectorApplication, err := textunitvectors.NewApplication(
		textunitvectors.ApplicationDependencies{
			Corpora:   service,
			Builder:   vectorService,
			Completer: service,
		})
	if err != nil {
		return corpusApplications{}, nil, err
	}
	vectorConsumer, err := corpusjournal.NewTextUnitVectorConsumer(vectorApplication)
	if err != nil {
		return corpusApplications{}, nil, err
	}
	return corpusApplications{
		service:            service,
		messages:           messages,
		documentConversion: documentConversion,
		textUnitCreation:   textUnitCreation,
		boundaries:         boundaries,
		merger:             merger,
	}, []journal.Consumer{vectorConsumer}, nil
}

func requiredAnalysisCapabilities(configuration project.Configuration) []analysis.Capability {
	required := make([]analysis.Capability, 0, 2)
	if configuration.Input.Type == project.InputTypeRichDocuments {
		required = append(required, analysis.CapabilityMarkItDown)
	}
	if configuration.Chunking.Type == textunits.SentenceChunking {
		required = append(required, analysis.CapabilitySentences)
	}
	return required
}
