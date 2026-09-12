package extraction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge"
	knowledgeevents "github.com/memoria-space/meking/knowledge/integration"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/transaction"
)

var ErrExtractionProgress = errors.New("Knowledge Extraction progress conflicts with the Corpora")

type Request struct {
	CorporaID     string
	SourceEventID journal.EventID
	CorrelationID journal.CorrelationID
	RequestedAt   time.Time
}

type TextUnitReader interface {
	TextUnits(context.Context, string) ([]TextUnitInput, error)
}

type Progress interface {
	ExtractionProgress(context.Context, string) (string, bool, error)
	AdvanceExtraction(context.Context, string, string, string) error
}

type KnowledgeSubmissions interface {
	Submit(context.Context, submission.Command) (submission.Result, error)
}

type Manifests interface {
	CurrentManifest(context.Context) (knowledge.Manifest, error)
}

type Producer interface {
	Publish(context.Context, []journal.Envelope[knowledgeevents.Body]) error
}

type Extractor interface {
	ExtractTextUnit(context.Context, string, TextUnitInput) (Result, error)
}

type Dependencies struct {
	Tx                   transaction.Tx
	TextUnitReader       TextUnitReader
	Progress             Progress
	KnowledgeSubmissions KnowledgeSubmissions
	Manifests            Manifests
	Producer             Producer
	Extractor            Extractor
}

type Application struct {
	Dependencies
}

func NewApplication(dependencies Dependencies) (*Application, error) {
	switch {
	case dependencies.Tx == nil:
		return nil, errors.New("create Knowledge Extraction: transaction is required")
	case dependencies.TextUnitReader == nil:
		return nil, errors.New("create Knowledge Extraction: TextUnit reader is required")
	case dependencies.Progress == nil:
		return nil, errors.New("create Knowledge Extraction: progress is required")
	case dependencies.KnowledgeSubmissions == nil:
		return nil, errors.New("create Knowledge Extraction: Knowledge submissions are required")
	case dependencies.Manifests == nil:
		return nil, errors.New("create Knowledge Extraction: Manifests are required")
	case dependencies.Producer == nil:
		return nil, errors.New("create Knowledge Extraction: Producer is required")
	case dependencies.Extractor == nil:
		return nil, errors.New("create Knowledge Extraction: Extractor is required")
	}
	return &Application{Dependencies: dependencies}, nil
}

func (application *Application) ExtractCorpora(ctx context.Context, request Request) (knowledge.Manifest, error) {
	if err := validateRequest(request); err != nil {
		return knowledge.Manifest{}, err
	}
	units, err := application.TextUnitReader.TextUnits(ctx, request.CorporaID)
	if err != nil {
		return knowledge.Manifest{}, fmt.Errorf("read Corpora TextUnits: %w", err)
	}
	if err := validateTextUnits(units); err != nil {
		return knowledge.Manifest{}, err
	}
	last, found, err := application.Progress.ExtractionProgress(ctx, request.CorporaID)
	if err != nil {
		return knowledge.Manifest{}, err
	}
	start, err := nextTextUnit(units, last, found)
	if err != nil {
		return knowledge.Manifest{}, err
	}
	previous := last
	for _, unit := range units[start:] {
		result, err := application.Extractor.ExtractTextUnit(ctx, request.CorporaID, unit)
		if err != nil {
			return knowledge.Manifest{}, fmt.Errorf("extract TextUnit %q: %w", unit.ID, err)
		}
		source := extractionSource(request.CorporaID, unit.ID)
		err = application.Tx.WithTx(ctx, func(ctx context.Context) error {
			if _, err := application.KnowledgeSubmissions.Submit(ctx, submission.Command{
				Source: source, Entities: result.Entities, Relations: result.Relations, Claims: result.Claims,
			}); err != nil {
				return err
			}
			return application.Progress.AdvanceExtraction(ctx, request.CorporaID, previous, unit.ID)
		})
		if err != nil {
			return knowledge.Manifest{}, fmt.Errorf("commit TextUnit %q Knowledge: %w", unit.ID, err)
		}
		previous = unit.ID
	}

	var current knowledge.Manifest
	err = application.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		current, err = application.Manifests.CurrentManifest(ctx)
		if err != nil {
			return err
		}
		publication := knowledgeevents.PublishedV1{CorporaID: knowledgeevents.CorporaID(request.CorporaID), Knowledge: current}
		if err := publication.Validate(); err != nil {
			return err
		}
		return application.Producer.Publish(ctx, []journal.Envelope[knowledgeevents.Body]{{
			EventID: publicationEventID(request.SourceEventID), StreamID: "knowledge/formal",
			CorrelationID: request.CorrelationID, CausationID: request.SourceEventID,
			OccurredAt: request.RequestedAt.UTC(), Body: publication,
		}})
	})
	if err != nil {
		return knowledge.Manifest{}, fmt.Errorf("publish extracted Knowledge: %w", err)
	}
	return current, nil
}

func nextTextUnit(units []TextUnitInput, last string, found bool) (int, error) {
	if !found {
		return 0, nil
	}
	for index, unit := range units {
		if unit.ID == last {
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("%w: last TextUnit %q is absent", ErrExtractionProgress, last)
}

func extractionSource(corporaID string, textUnitID string) provenance.Source {
	digest := sha256.Sum256([]byte(corporaID + "\x00" + textUnitID))
	return provenance.Source{
		ID: "source/extraction/" + hex.EncodeToString(digest[:]), Kind: provenance.Extraction,
		ProducerID: corporaID + "/" + textUnitID,
	}
}

func publicationEventID(sourceEventID journal.EventID) journal.EventID {
	digest := sha256.Sum256([]byte(string(sourceEventID) + "\x00knowledge.published"))
	return journal.EventID("event/" + hex.EncodeToString(digest[:]))
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.CorporaID) == "" || strings.TrimSpace(string(request.SourceEventID)) == "" ||
		strings.TrimSpace(string(request.CorrelationID)) == "" || request.RequestedAt.IsZero() {
		return errors.New("Knowledge Extraction request is incomplete")
	}
	return nil
}
