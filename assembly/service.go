// Package assembly composes the process-level services and shared resources.
package assembly

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/memoria-space/meking/analysis"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/message"
	corpusmerger "github.com/memoria-space/meking/corpus/zonemerger"
	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/deletion"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/memory"
	"github.com/memoria-space/meking/memory/activation"
	memorysemantic "github.com/memoria-space/meking/memory/adapter/semantic"
	"github.com/memoria-space/meking/project"
	querygraph "github.com/memoria-space/meking/query/graph"
	queryknowledge "github.com/memoria-space/meking/query/knowledge"
	queryreport "github.com/memoria-space/meking/query/report"
	semanticagent "github.com/memoria-space/meking/semantic/adapter/agent"
	"github.com/memoria-space/meking/zone"
	zonemerger "github.com/memoria-space/meking/zone/merger"
)

type Config struct {
	Root            string
	AnalysisCommand string
	Logger          *slog.Logger
}

func (config Config) validate() error {
	switch {
	case strings.TrimSpace(config.Root) == "":
		return errors.New("open service: project root is required")
	case config.Logger == nil:
		return errors.New("open service: logger is required")
	default:
		return nil
	}
}

type Service struct {
	configuration   project.Configuration
	resources       databaseResources
	logger          *slog.Logger
	zones           *zone.Catalog
	zoneMerger      *zonemerger.Application
	documents       *corpus.Service
	messages        *message.Log
	memoryRecorder  *memory.Recorder
	memorySearcher  *memory.Searcher
	observations    *activation.Service
	knowledgeReader *knowledge.Reader
	versions        knowledge.CurrentVersions
	evidenceReader  *provenance.Reader
	resolutions     *resolution.Application
	deletions       *deletion.Application
	corpusMerger    *corpusmerger.Application
	controlPolicies *controlapplication.Policies
	controlStatuses *controlapplication.ActionStatuses
	controlActions  *controlapplication.Actions
	processRuntime  *processRuntime
	journal         *journal.Service
	queries         *Queries
	reports         *queryreport.ReportCatalog
	graph           *querygraph.Catalog
	knowledge       *queryknowledge.Catalog
	analysisSession *analysis.Session
	vectors         *vectorDatabases

	mutex  sync.Mutex
	closed bool
}

func Open(ctx context.Context, config Config) (_ *Service, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	resources, err := openDatabase(ctx, config.Root)
	if err != nil {
		return nil, err
	}
	opened := &Service{resources: resources, logger: config.Logger}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, opened.closeResources())
		}
	}()
	zones, err := openZones(resources)
	if err != nil {
		return nil, err
	}
	opened.zones = zones.catalog
	opened.configuration, err = project.LoadConfiguration(resources.root)
	if err != nil {
		return nil, err
	}
	events, err := openJournal(resources)
	if err != nil {
		return nil, err
	}
	opened.journal = events.service
	controlPlane, err := openControlPlane(resources)
	if err != nil {
		return nil, err
	}
	opened.controlPolicies = controlPlane.policies
	opened.analysisSession, err = openAnalysis(
		ctx,
		resources.root,
		opened.configuration.CacheDirectory,
		config.AnalysisCommand,
		requiredAnalysisCapabilities(opened.configuration),
	)
	if err != nil {
		return nil, err
	}
	models, err := openAgent(opened.configuration)
	if err != nil {
		return nil, err
	}
	tokens, err := corpusTokenizer(opened.configuration)
	if err != nil {
		return nil, err
	}
	embedder, err := semanticagent.NewEmbedder(models.embedding)
	if err != nil {
		return nil, err
	}
	opened.vectors, err = openVectorDatabases(
		resources.root,
		opened.configuration.Embedding().Model,
		config.Logger,
	)
	if err != nil {
		return nil, err
	}
	entitySemantic, err := opened.vectors.Entities.NewService(
		embedder,
		tokens,
		opened.configuration.EntityVectors,
	)
	if err != nil {
		return nil, err
	}
	corpora, corpusConsumers, err := openCorpus(
		ctx,
		resources,
		opened.configuration,
		opened.analysisSession,
		events,
		embedder,
		tokens,
		opened.vectors.TextUnits,
		config.Logger,
	)
	if err != nil {
		return nil, err
	}
	documents := corpora.service
	opened.documents = documents
	opened.messages = corpora.messages
	opened.corpusMerger = corpora.merger
	knowledgeApplications, knowledgeConsumers, err := openKnowledge(knowledgeDependencies{
		databaseResources: resources,
		eventJournal:      events,
		configuration:     opened.configuration,
		corpora:           documents,
		messages:          corpora.messages,
		completion:        models.completion,
		entitySemantic:    entitySemantic,
		logger:            config.Logger,
	})
	if err != nil {
		return nil, err
	}
	knowledgeReader := knowledgeApplications.reader
	opened.knowledgeReader = knowledgeApplications.reader
	opened.versions = knowledgeApplications.versions
	opened.evidenceReader = knowledgeApplications.provenance
	if err := opened.openActivation(ctx); err != nil {
		return nil, err
	}
	opened.memoryRecorder, err = memory.NewRecorder(memory.Dependencies{
		Messages:        corpora.messages,
		Submissions:     knowledgeApplications.submissions,
		ConflictCatalog: knowledgeApplications.resolutions,
	})
	if err != nil {
		return nil, err
	}
	entityMatcher, err := memorysemantic.NewMatcher(
		knowledgeApplications.entityVectors,
		entitySemantic,
		embedder,
	)
	if err != nil {
		return nil, err
	}
	opened.memorySearcher, err = memory.NewSearcher(memory.SearchDependencies{
		Knowledge: knowledgeApplications.reader,
		Evidence:  knowledgeApplications.provenance,
		Entities:  entityMatcher,
		Corpora:   documents,
		Messages:  corpora.messages,
	})
	if err != nil {
		return nil, err
	}
	opened.resolutions = knowledgeApplications.resolutions
	opened.deletions = knowledgeApplications.deletions
	communities, err := openCommunity(
		resources,
		opened.configuration,
		events,
		documents,
		knowledgeApplications,
		models.completion,
		embedder,
		tokens,
		opened.vectors.Reports,
		config.Logger,
	)
	if err != nil {
		return nil, err
	}
	epochApplications, err := openEpoch(
		resources,
		events,
		documents,
		knowledgeReader,
		communities.store,
		opened.vectors,
	)
	if err != nil {
		return nil, err
	}
	actionControl, err := openControlActions(
		opened.controlPolicies,
		corpora,
		knowledgeApplications,
		communities,
		epochApplications,
	)
	if err != nil {
		return nil, err
	}
	opened.controlActions = actionControl.invocations
	opened.controlStatuses = actionControl.statuses
	automaticEvaluator, err := openAutomaticControl(
		opened.zones,
		opened.controlActions,
		events.actionSignals,
	)
	if err != nil {
		return nil, err
	}
	consumers := append([]journal.Consumer{}, corpusConsumers...)
	consumers = append(consumers, knowledgeConsumers...)
	opened.processRuntime, err = openProcessRuntime(
		events,
		opened.zones,
		consumers,
		[]processRunner{
			corpora.documentConversion,
			corpora.textUnitCreation,
			knowledgeApplications.extractionAction,
			knowledgeApplications.entityVectorAction,
			communities.structureDerivation,
			epochApplications.publicationAction,
			communities.reportGeneration,
			automaticEvaluator,
		},
	)
	if err != nil {
		return nil, err
	}
	zoneMerger, err := openZoneMergerFlow(
		zones,
		corpora,
		knowledgeApplications,
	)
	if err != nil {
		return nil, err
	}
	opened.zoneMerger = zoneMerger
	queryApplications, err := openQueries(
		opened.configuration,
		models.queryCompletion,
		embedder,
		documents,
		knowledgeApplications,
		communities,
		epochApplications,
		opened.vectors,
	)
	if err != nil {
		return nil, err
	}
	opened.queries = queryApplications.queries
	opened.reports = queryApplications.reports
	opened.graph = queryApplications.graph
	opened.knowledge = queryApplications.knowledge
	return opened, nil
}

// Zones returns the shared Zone Catalog assembled for this process.
func (service *Service) Zones() *zone.Catalog {
	if service == nil {
		return nil
	}
	return service.zones
}

// ZoneMerger returns the process-routed Child merge application. Delivery
// middleware binds a Zone Context before invoking it.
func (service *Service) ZoneMerger() *zonemerger.Application {
	if service == nil {
		return nil
	}
	return service.zoneMerger
}

func (service *Service) Documents() *corpus.Service {
	if service == nil {
		return nil
	}
	return service.documents
}

func (service *Service) Messages() *message.Log {
	if service == nil {
		return nil
	}
	return service.messages
}

func (service *Service) MemoryRecorder() *memory.Recorder {
	if service == nil {
		return nil
	}
	return service.memoryRecorder
}

func (service *Service) MemorySearcher() *memory.Searcher {
	if service == nil {
		return nil
	}
	return service.memorySearcher
}

func (service *Service) ConflictResolutions() *resolution.Application {
	if service == nil {
		return nil
	}
	return service.resolutions
}

func (service *Service) KnowledgeDeletions() *deletion.Application {
	if service == nil {
		return nil
	}
	return service.deletions
}

func (service *Service) CorpusMerger() *corpusmerger.Application {
	if service == nil {
		return nil
	}
	return service.corpusMerger
}

func (service *Service) ControlPolicies() *controlapplication.Policies {
	if service == nil {
		return nil
	}
	return service.controlPolicies
}

func (service *Service) ControlStatuses() *controlapplication.ActionStatuses {
	if service == nil {
		return nil
	}
	return service.controlStatuses
}

func (service *Service) ControlActions() *controlapplication.Actions {
	if service == nil {
		return nil
	}
	return service.controlActions
}

// Run dispatches committed Journal events and advances accepted domain Actions
// until the caller cancels the process or a background component fails.
func (service *Service) Run(ctx context.Context) error {
	if service == nil || service.processRuntime == nil {
		return errors.New("run Project service: process runtime is required")
	}
	return service.processRuntime.Run(ctx)
}

func (service *Service) Journal() *journal.Service {
	if service == nil {
		return nil
	}
	return service.journal
}

func (service *Service) Queries() *Queries {
	if service == nil {
		return nil
	}
	return service.queries
}

func (service *Service) DRIFTEnabled() bool {
	return service != nil && service.configuration.Community.ReportVectors != nil
}

func (service *Service) Reports() *queryreport.ReportCatalog {
	if service == nil {
		return nil
	}
	return service.reports
}

func (service *Service) Graph() *querygraph.Catalog {
	if service == nil {
		return nil
	}
	return service.graph
}

func (service *Service) Knowledge() *queryknowledge.Catalog {
	if service == nil {
		return nil
	}
	return service.knowledge
}

func (service *Service) Close() error {
	if service == nil {
		return nil
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.closed {
		return nil
	}
	if err := service.processRuntime.Close(); err != nil {
		return err
	}
	service.closed = true
	return service.closeResources()
}

func (service *Service) closeResources() error {
	if service == nil {
		return nil
	}
	vectors := service.vectors
	analysisSession := service.analysisSession
	service.graph = nil
	service.knowledge = nil
	service.reports = nil
	service.queries = nil
	service.vectors = nil
	service.analysisSession = nil
	service.documents = nil
	service.messages = nil
	service.memoryRecorder = nil
	service.memorySearcher = nil
	service.observations = nil
	service.knowledgeReader = nil
	service.evidenceReader = nil
	service.resolutions = nil
	service.deletions = nil
	service.controlPolicies = nil
	service.controlStatuses = nil
	service.controlActions = nil
	service.zoneMerger = nil
	service.processRuntime = nil
	var vectorErr, analysisSessionErr error
	if vectors != nil {
		vectorErr = vectors.Close()
	}
	if analysisSession != nil {
		analysisSessionErr = analysisSession.Close()
	}
	return errors.Join(
		vectorErr,
		analysisSessionErr,
		service.resources.close(),
	)
}
