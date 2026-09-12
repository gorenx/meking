package assembly

import (
	"context"
	"fmt"

	"github.com/memoria-space/meking/agent"
	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/project"
	prompttext "github.com/memoria-space/meking/project/prompts"
	querybase "github.com/memoria-space/meking/query"
	queryadapter "github.com/memoria-space/meking/query/adapter"
	queryapplication "github.com/memoria-space/meking/query/application"
	querybasic "github.com/memoria-space/meking/query/basic"
	querybasicadapter "github.com/memoria-space/meking/query/basic/adapter"
	querydrift "github.com/memoria-space/meking/query/drift"
	querydriftadapter "github.com/memoria-space/meking/query/drift/adapter"
	queryglobal "github.com/memoria-space/meking/query/global"
	queryglobaladapter "github.com/memoria-space/meking/query/global/adapter"
	querygraph "github.com/memoria-space/meking/query/graph"
	querygraphadapter "github.com/memoria-space/meking/query/graph/adapter"
	queryknowledge "github.com/memoria-space/meking/query/knowledge"
	queryknowledgeadapter "github.com/memoria-space/meking/query/knowledge/adapter"
	querylocal "github.com/memoria-space/meking/query/local"
	querylocaladapter "github.com/memoria-space/meking/query/local/adapter"
	queryquestion "github.com/memoria-space/meking/query/question"
	queryquestionadapter "github.com/memoria-space/meking/query/question/adapter"
	queryreport "github.com/memoria-space/meking/query/report"
	queryreportadapter "github.com/memoria-space/meking/query/report/adapter"
	querysource "github.com/memoria-space/meking/query/source"
	querysourceadapter "github.com/memoria-space/meking/query/source/adapter"
	"github.com/memoria-space/meking/semantic"
)

// Queries creates request-scoped Query applications over Project-shared
// readers, Agent clients, and vector databases.
type Queries struct {
	configuration project.Configuration
	completion    *agent.OpenAICompletion
	embedding     semantic.Embedder
	corpora       *corpus.Service
	knowledge     *knowledge.Reader
	provenance    *provenance.Reader
	vectors       *vectorDatabases
	epochs        querybase.EpochReader
	reports       *queryreport.Reader
	sources       querysource.Reader
	tokens        querybase.TokenCounter
}

type queryApplications struct {
	queries   *Queries
	reports   *queryreport.ReportCatalog
	graph     *querygraph.Catalog
	knowledge *queryknowledge.Catalog
}

func openQueries(
	configuration project.Configuration,
	completion *agent.OpenAICompletion,
	embedding semantic.Embedder,
	corpora *corpus.Service,
	knowledge knowledgeApplications,
	communities communityApplications,
	epochs epochApplications,
	vectors *vectorDatabases,
) (queryApplications, error) {
	epochReader, err := queryadapter.NewEpochReader(epochs.reader, communities.store)
	if err != nil {
		return queryApplications{}, err
	}
	publicationReader, err := queryreportadapter.NewPublicationReader(
		communities.store,
		communities.store,
		communities.reports,
	)
	if err != nil {
		return queryApplications{}, err
	}
	reports, err := queryreport.NewReader(publicationReader)
	if err != nil {
		return queryApplications{}, err
	}
	reportCatalog, err := queryreport.NewReportCatalog(epochReader, publicationReader)
	if err != nil {
		return queryApplications{}, err
	}
	knowledgeGraph, err := querygraphadapter.NewKnowledgeReader(
		knowledge.reader,
		knowledge.provenance,
	)
	if err != nil {
		return queryApplications{}, err
	}
	structures, err := querygraphadapter.NewStructureReader(communities.store, communities.store)
	if err != nil {
		return queryApplications{}, err
	}
	graph, err := querygraph.NewCatalog(knowledgeGraph, structures)
	if err != nil {
		return queryApplications{}, err
	}
	knowledgeBrowseReader, err := queryknowledgeadapter.NewReader(
		knowledge.reader,
		knowledge.provenance,
	)
	if err != nil {
		return queryApplications{}, err
	}
	sources, err := querysourceadapter.NewSourceReader(corpora)
	if err != nil {
		return queryApplications{}, err
	}
	knowledgeCatalog, err := queryknowledge.NewCatalog(epochReader, knowledgeBrowseReader, sources)
	if err != nil {
		return queryApplications{}, err
	}
	tokenizer, err := corpusTokenizer(configuration)
	if err != nil {
		return queryApplications{}, err
	}
	tokens, err := queryadapter.NewTokenCounter(tokenizer)
	if err != nil {
		return queryApplications{}, err
	}
	queries := &Queries{
		configuration: configuration,
		completion:    completion,
		embedding:     embedding,
		corpora:       corpora,
		knowledge:     knowledge.reader,
		provenance:    knowledge.provenance,
		vectors:       vectors,
		epochs:        epochReader,
		reports:       reports,
		sources:       sources,
		tokens:        tokens,
	}
	return queryApplications{
		queries: queries, reports: reportCatalog, graph: graph, knowledge: knowledgeCatalog,
	}, nil
}

func (queries *Queries) Run(
	ctx context.Context,
	request queryapplication.Request,
) (queryapplication.Execution, error) {
	application, err := queries.application()
	if err != nil {
		return queryapplication.Execution{Method: request.Method}, err
	}
	return application.Run(ctx, request)
}

func (queries *Queries) Stream(
	ctx context.Context,
	request queryapplication.Request,
	emit querybase.TextDeltaHandler,
) (queryapplication.Execution, error) {
	application, err := queries.application()
	if err != nil {
		return queryapplication.Execution{Method: request.Method}, err
	}
	return application.Stream(ctx, request, emit)
}

func (queries *Queries) Suggest(
	ctx context.Context,
	request queryapplication.SuggestionRequest,
) (queryapplication.SuggestionExecution, error) {
	application, err := queries.application()
	if err != nil {
		return queryapplication.SuggestionExecution{}, err
	}
	return application.Suggest(ctx, request)
}

func (queries *Queries) application() (*queryapplication.Service, error) {
	if queries == nil || queries.completion == nil || queries.embedding == nil || queries.corpora == nil ||
		queries.knowledge == nil || queries.vectors == nil || queries.epochs == nil ||
		queries.reports == nil || queries.sources == nil || queries.tokens == nil {
		return nil, querybase.NewInternalFailure(fmt.Errorf("Project Query is not configured"))
	}
	embedder, err := queryadapter.NewEmbedder(
		queries.configuration.Embedding().Model,
		queries.embedding,
	)
	if err != nil {
		return nil, querybase.NewInternalFailure(err)
	}
	basic, err := queries.basic(queries.completion, embedder)
	if err != nil {
		return nil, err
	}
	local, err := queries.local(queries.completion, embedder)
	if err != nil {
		return nil, err
	}
	global, err := queries.global(queries.completion)
	if err != nil {
		return nil, err
	}
	drift, err := queries.drift(queries.completion, embedder, local)
	if err != nil {
		return nil, err
	}
	questions, err := queries.questions(queries.completion, local)
	if err != nil {
		return nil, err
	}
	return queryapplication.New(queryapplication.Dependencies{
		Basic: basic, Local: local, Global: global, DRIFT: drift,
		Questions:              questions,
		GlobalMaxContextTokens: queries.configuration.Query.Global.MaxContextTokens,
	})
}

func (queries *Queries) basic(
	completion *agent.OpenAICompletion,
	embedder *queryadapter.Embedder,
) (*querybasic.Searcher, error) {
	vectors, err := querybasicadapter.NewVectorStore(queries.vectors.TextUnits)
	if err != nil {
		return nil, err
	}
	model, err := querybasicadapter.NewCompletionModel(completion)
	if err != nil {
		return nil, err
	}
	return querybasic.NewSearcher(
		queries.epochs,
		vectors,
		embedder,
		queries.tokens,
		model,
		queries.sources,
		prompttext.BasicSearchSystem,
		querybasic.DefaultConfig(),
	)
}

func (queries *Queries) local(
	completion *agent.OpenAICompletion,
	embedder *queryadapter.Embedder,
) (*querylocal.Searcher, error) {
	vectors, err := querylocaladapter.NewEntityVectorStore(queries.vectors.Entities)
	if err != nil {
		return nil, err
	}
	knowledgeReader, err := querylocaladapter.NewKnowledgeReader(
		queries.knowledge,
		queries.provenance,
	)
	if err != nil {
		return nil, err
	}
	model, err := querylocaladapter.NewCompletionModel(completion)
	if err != nil {
		return nil, err
	}
	return querylocal.NewSearcher(
		queries.epochs,
		queries.reports,
		vectors,
		embedder,
		knowledgeReader,
		queries.tokens,
		model,
		queries.sources,
		prompttext.LocalSearchSystem,
		querylocal.DefaultConfig(),
	)
}

func (queries *Queries) global(
	completion *agent.OpenAICompletion,
) (*queryglobal.Searcher, error) {
	model, err := queryglobaladapter.NewCompletionModel(completion)
	if err != nil {
		return nil, err
	}
	evidence, err := queryglobaladapter.NewEvidenceReader(
		queries.reports,
		queries.knowledge,
		queries.provenance,
	)
	if err != nil {
		return nil, err
	}
	configuration := queries.configuration.Query
	selector, err := queryglobal.NewDynamicCommunitySelector(
		model,
		prompttext.GlobalSearchRate,
		queryglobal.DynamicSelectionConfig{
			Threshold:      configuration.Global.DynamicSearchThreshold,
			KeepParent:     configuration.Global.DynamicSearchKeepParent,
			Repeats:        configuration.Global.DynamicSearchNumRepeats,
			UseSummary:     configuration.Global.DynamicSearchUseSummary,
			MaxLevel:       configuration.Global.DynamicSearchMaxLevel,
			MaxConcurrency: queries.configuration.MaxConcurrent,
		},
	)
	if err != nil {
		return nil, err
	}
	contextBuilder, err := queryglobal.NewContextBuilder(queries.tokens, selector)
	if err != nil {
		return nil, err
	}
	mapper, err := queryglobal.NewMapper(
		model,
		configuration.GlobalMapPrompt,
		queryglobal.MapConfig{
			MaxLength:      configuration.Global.MapMaxLength,
			MaxConcurrency: queries.configuration.MaxConcurrent,
		},
	)
	if err != nil {
		return nil, err
	}
	reducer, err := queryglobal.NewReducer(
		model,
		queries.tokens,
		configuration.GlobalReducePrompt,
		configuration.GlobalKnowledgePrompt,
		queryglobal.ReduceConfig{
			DataMaxTokens: configuration.Global.DataMaxTokens,
			MaxLength:     configuration.Global.ReduceMaxLength,
		},
	)
	if err != nil {
		return nil, err
	}
	return queryglobal.NewSearcher(
		queries.epochs,
		evidence,
		contextBuilder,
		mapper,
		reducer,
		queries.sources,
	)
}

func (queries *Queries) drift(
	completion *agent.OpenAICompletion,
	embedder *queryadapter.Embedder,
	local *querylocal.Searcher,
) (*querydrift.Searcher, error) {
	vectors, err := querydriftadapter.NewReportVectorStore(queries.vectors.Reports)
	if err != nil {
		return nil, err
	}
	model, err := querydriftadapter.NewCompletionModel(completion)
	if err != nil {
		return nil, err
	}
	configuration := queries.configuration.Query
	primerConfig := configuration.Drift.Primer
	primerConfig.MaxConcurrency = boundedConcurrency(
		primerConfig.MaxConcurrency,
		queries.configuration.MaxConcurrent,
	)
	primer, err := querydrift.NewPrimer(
		queries.epochs,
		queries.reports,
		vectors,
		embedder,
		model,
		queries.tokens,
		prompttext.DriftHypothetical,
		prompttext.DriftPrimer,
		primerConfig,
	)
	if err != nil {
		return nil, err
	}
	localSessions, err := querydriftadapter.NewLocalSessionOpener(local)
	if err != nil {
		return nil, err
	}
	traversalConfig := configuration.Drift.Traversal
	traversalConfig.MaxConcurrency = boundedConcurrency(
		traversalConfig.MaxConcurrency,
		queries.configuration.MaxConcurrent,
	)
	traversal, err := querydrift.NewTraversal(
		model,
		queries.tokens,
		configuration.DriftSearchPrompt,
		traversalConfig,
	)
	if err != nil {
		return nil, err
	}
	reducer, err := querydrift.NewReducer(
		model,
		queries.tokens,
		configuration.DriftReducePrompt,
		configuration.Drift.Reduce,
		traversalConfig.Limits,
	)
	if err != nil {
		return nil, err
	}
	return querydrift.NewSearcher(primer, localSessions, traversal, reducer, queries.sources)
}

func (queries *Queries) questions(
	completion *agent.OpenAICompletion,
	local *querylocal.Searcher,
) (*queryquestion.Generator, error) {
	evidence, err := queryquestionadapter.NewLocalEvidenceProvider(local)
	if err != nil {
		return nil, err
	}
	model, err := queryquestionadapter.NewCompletionModel(completion)
	if err != nil {
		return nil, err
	}
	return queryquestion.NewGenerator(
		evidence,
		model,
		queries.tokens,
		prompttext.QuestionGenerationSystem,
	)
}

func boundedConcurrency(configured, shared int) int {
	if shared < configured {
		return shared
	}
	return configured
}
