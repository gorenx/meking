package corpus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
)

// Store is the persistence contract used to assemble Corpus applications.
// Internal applications still accept only the subset they use.
type Store interface {
	BeginCorporaBuild(ctx context.Context) (CorporaID, error)
	OpenTextChunkingProgress(
		ctx context.Context,
		corporaID CorporaID,
		sourceEventID EventID,
		textID text.ID,
	) (TextChunkingProgress, error)
	SaveTextUnitBatch(
		ctx context.Context,
		corporaID CorporaID,
		textID text.ID,
		spans []textunits.TextUnit,
	) error
	AdvanceTextChunkingProgress(
		ctx context.Context,
		corporaID CorporaID,
		progress TextChunkingProgress,
		nextChunkIndex int,
		complete bool,
	) error
	RecordTextUnitVectors(ctx context.Context, completion TextVectorCompletion) (bool, error)
	FinalizeCorpora(ctx context.Context, id CorporaID) (Corpora, bool, error)
	Activate(ctx context.Context, id CorporaID) error
	TextStore() text.Store
	Load(ctx context.Context, id CorporaID) (Corpora, error)
	LoadBuildingCorpora(ctx context.Context, corporaID CorporaID) (Corpora, error)
	LoadChunkedText(ctx context.Context, corporaID CorporaID, textID text.ID) (ChunkedText, bool, error)
	SaveTextUnits(ctx context.Context, corporaID CorporaID, value ChunkedText) error
	Current(ctx context.Context) (Corpora, error)
	CurrentID(ctx context.Context) (CorporaID, error)
	ExistingTextUnits(ctx context.Context, ids []textunits.TextUnitID) (map[textunits.TextUnitID]struct{}, error)
	TextUnits(ctx context.Context, ids []textunits.TextUnitID) ([]textunits.TextUnitBody, error)
	TextUnitLocations(ctx context.Context, setID CorporaID, ids []textunits.TextUnitID) ([]TextUnitLocation, error)
}

// Logger is the Corpus application logging port. Persisted content and source
// paths are never included in log attributes.
type Logger interface {
	LogAttrs(
		ctx context.Context,
		level slog.Level,
		message string,
		attributes ...slog.Attr,
	)
}

type serviceDependencies struct {
	Chunking *textunits.Chunking

	SentenceAnalyzer   textunits.SentenceAnalyzer
	Documents          *document.Manager
	DocumentCatalog    DocumentCatalogReader
	RichTextExtractors text.ExtractorResolver
	EventTransactions  Transaction
	EventProducer      Producer
	Logger             Logger
}

type ServiceOption func(*serviceDependencies) error

func WithDocument(documents *document.Manager) ServiceOption {
	return func(dependencies *serviceDependencies) error {
		if documents == nil {
			return errors.New("configure Corpus Document application: service is required")
		}
		dependencies.Documents = documents
		return nil
	}
}

func WithDocumentCatalog(catalog DocumentCatalogReader) ServiceOption {
	return func(dependencies *serviceDependencies) error {
		if catalog == nil {
			return errors.New("configure Corpus Document Catalog: reader is required")
		}
		dependencies.DocumentCatalog = catalog
		return nil
	}
}

func WithRichDocumentAnalysis(
	extractors text.ExtractorResolver,
) ServiceOption {
	return func(dependencies *serviceDependencies) error {
		if extractors == nil {
			return errors.New("configure Corpus rich Document analysis: ExtractorResolver is required")
		}
		dependencies.RichTextExtractors = extractors
		return nil
	}
}

func WithSentenceAnalysis(sentenceAnalyzer textunits.SentenceAnalyzer) ServiceOption {
	return func(dependencies *serviceDependencies) error {
		if sentenceAnalyzer == nil {
			return errors.New("configure Corpus Sentence analysis: SentenceBoundaryAnalyzer is required")
		}
		dependencies.SentenceAnalyzer = sentenceAnalyzer
		return nil
	}
}

func WithLogger(logger Logger) ServiceOption {
	return func(dependencies *serviceDependencies) error {
		dependencies.Logger = logger
		return nil
	}
}

func WithChunking(chunking textunits.Chunking) ServiceOption {
	return func(dependencies *serviceDependencies) error {
		if err := textunits.ValidateChunking(chunking); err != nil {
			return fmt.Errorf("configure Corpus Chunking: %w", err)
		}
		dependencies.Chunking = &chunking
		return nil
	}
}

// Service is the public Corpus write application. It coordinates source
// persistence, text conversion, and Corpus construction without invoking
// another domain or deciding which downstream task should run.
type Service struct {
	documents        *document.Manager
	documentCatalog  DocumentCatalogReader
	texts            *text.Builder
	extractors       text.ExtractorResolver
	store            Store
	logger           Logger
	transactions     Transaction
	producer         Producer
	chunking         textunits.Chunking
	sentenceAnalyzer textunits.SentenceAnalyzer
}

func NewService(store Store, options ...ServiceOption) (*Service, error) {
	if store == nil {
		return nil, errors.New("create Corpus Service: Store is required")
	}
	dependencies := serviceDependencies{}
	for index, option := range options {
		if option == nil {
			return nil, fmt.Errorf("create Corpus Service: option %d is required", index)
		}
		if err := option(&dependencies); err != nil {
			return nil, fmt.Errorf("create Corpus Service: apply option %d: %w", index, err)
		}
	}
	if dependencies.Documents == nil {
		return nil, errors.New("create Corpus Service: Document application is required")
	}
	documents := dependencies.Documents
	extractors := text.ExtractorResolverFunc(
		func(name, mediaType string) (text.Extractor, int64, error) {
			if dependencies.RichTextExtractors != nil {
				extractor, maximumBytes, err := dependencies.RichTextExtractors.Resolve(name, mediaType)
				if err == nil {
					return extractor, maximumBytes, nil
				}
				if !errors.Is(err, text.ErrUnsupported) {
					return nil, 0, err
				}
			}
			if strings.HasPrefix(mediaType, "text/") {
				return text.PlainTextExtractor{}, document.MaximumContentBytes, nil
			}
			return nil, 0, text.ErrUnsupported
		})
	textStore := store.TextStore()
	texts, err := text.NewBuilder(
		documents,
		extractors,
		text.StandardNormalizer{},
		textStore,
	)
	if err != nil {
		return nil, fmt.Errorf("create Corpus Service Text application: %w", err)
	}
	switch {
	case dependencies.Logger == nil:
		return nil, errors.New("create Corpus Service: logger is required")
	}
	if (dependencies.EventTransactions == nil) != (dependencies.EventProducer == nil) {
		return nil, errors.New("create Corpus Service: Event Transactions and Producer must be configured together")
	}
	if dependencies.EventTransactions != nil && dependencies.Chunking == nil {
		return nil, errors.New("create Corpus Service: Chunking is required for event processing")
	}
	var chunking textunits.Chunking
	if dependencies.EventTransactions != nil {
		chunking = *dependencies.Chunking
	}
	return &Service{
		documents:        documents,
		documentCatalog:  dependencies.DocumentCatalog,
		texts:            texts,
		store:            store,
		logger:           dependencies.Logger,
		extractors:       extractors,
		transactions:     dependencies.EventTransactions,
		producer:         dependencies.EventProducer,
		chunking:         chunking,
		sentenceAnalyzer: dependencies.SentenceAnalyzer,
	}, nil
}

func (s *Service) Corpora(ctx context.Context, id CorporaID) (Corpora, error) {
	if err := ValidateCorporaID(id); err != nil {
		return Corpora{}, err
	}
	return s.store.Load(ctx, id)
}

func (s *Service) BuildingCorpora(ctx context.Context, corporaID CorporaID) (Corpora, error) {
	if err := ValidateCorporaID(corporaID); err != nil {
		return Corpora{}, err
	}
	return s.store.LoadBuildingCorpora(ctx, corporaID)
}

func (s *Service) CurrentCorpora(ctx context.Context) (Corpora, error) {
	return s.store.Current(ctx)
}

func (s *Service) CurrentCorporaID(ctx context.Context) (CorporaID, error) {
	return s.store.CurrentID(ctx)
}

func (s *Service) CorporaChunking(
	ctx context.Context,
	id CorporaID,
) (textunits.Chunking, error) {
	if err := ValidateCorporaID(id); err != nil {
		return textunits.Chunking{}, err
	}
	if _, err := s.store.Load(ctx, id); err != nil {
		return textunits.Chunking{}, err
	}
	return s.chunking, nil
}

func (s *Service) ExistingTextUnits(
	ctx context.Context,
	ids []textunits.TextUnitID,
) (map[textunits.TextUnitID]struct{}, error) {
	return s.store.ExistingTextUnits(ctx, ids)
}

func (s *Service) TextUnits(
	ctx context.Context,
	ids []textunits.TextUnitID,
) ([]textunits.TextUnitBody, error) {
	return s.store.TextUnits(ctx, ids)
}

func (s *Service) TextUnitLocations(
	ctx context.Context,
	setID CorporaID,
	ids []textunits.TextUnitID,
) ([]TextUnitLocation, error) {
	if err := ValidateCorporaID(setID); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if err := textunits.ValidateTextUnitID(id); err != nil {
			return nil, err
		}
	}
	return s.store.TextUnitLocations(ctx, setID, append([]textunits.TextUnitID(nil), ids...))
}

func (s *Service) ListLocatedDocuments(
	ctx context.Context,
	page document.Page,
) (document.LocatedDocumentPage, error) {
	if s == nil || s.documents == nil {
		return document.LocatedDocumentPage{}, errors.New("Corpus Service is not configured")
	}
	return s.documents.ListLocated(ctx, page)
}

func (s *Service) LocatedDocument(
	ctx context.Context,
	location document.Location,
) (document.LocatedDocument, error) {
	if s == nil || s.documents == nil {
		return document.LocatedDocument{}, errors.New("Corpus Service is not configured")
	}
	return s.documents.LocatedByLocation(ctx, location)
}
