package corpus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/corpus/document"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/internal/uuid"
	"github.com/memoria-space/meking/zone"
)

func WithEventPublishing(
	transactions Transaction,
	producer Producer,
) ServiceOption {
	return func(dependencies *serviceDependencies) error {
		switch {
		case transactions == nil:
			return errors.New("configure Corpus event publishing: Transactions are required")
		case producer == nil:
			return errors.New("configure Corpus event publishing: Producer is required")
		}
		dependencies.EventTransactions = transactions
		dependencies.EventProducer = producer
		return nil
	}
}

// DocumentReceipt identifies source content durably stored in the Project.
type DocumentReceipt struct {
	DocumentID    document.ID
	ContentDigest string
}

func (s *Service) SubmitDocument(
	ctx context.Context,
	upload document.UploadCommand,
) (DocumentReceipt, error) {
	if s == nil || s.documents == nil || s.transactions == nil || s.producer == nil {
		return DocumentReceipt{}, errors.New("Corpus Document uploads are not configured")
	}
	if upload.Content == nil {
		return DocumentReceipt{}, errors.New("submit Corpus Document: Content is required")
	}
	_, maximumBytes, err := s.extractors.Resolve(upload.Name, upload.MediaType)
	if err != nil {
		return DocumentReceipt{}, fmt.Errorf("select Corpus Document Extractor: %w", err)
	}
	upload.MaximumBytes = maximumBytes
	prepared, err := s.documents.PrepareUpload(ctx, upload)
	if err != nil {
		return DocumentReceipt{}, fmt.Errorf("prepare Corpus Document upload: %w", err)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return DocumentReceipt{}, errors.Join(err, prepared.Discard())
	}
	requestID, err := uuid.NewV4()
	if err != nil {
		return DocumentReceipt{}, errors.Join(err, prepared.Discard())
	}
	var saved document.SaveResult
	err = s.transactions.WithTx(ctx, func(ctx context.Context) error {
		saved, err = s.documents.Record(ctx, prepared.LocatedDocument)
		if err != nil {
			return err
		}
		selected, err := s.documents.Associate(ctx, []document.ID{saved.Document.ID})
		if err != nil {
			return err
		}
		if len(selected) != 1 || selected[0].Document.ID != saved.Document.ID {
			return errors.New("submit Corpus Document: association returned an inconsistent Document")
		}
		body := corpusevents.DocumentRecordedV1{
			DocumentID:    corpusevents.DocumentID(saved.Document.ID),
			ContentDigest: saved.Document.Digest,
			Source: corpusevents.DocumentSource{
				ZoneID:     string(zoneID),
				DocumentID: corpusevents.DocumentID(saved.Document.ID),
			},
		}
		return s.producer.Publish(ctx, []Event{{
			EventID:       EventID("document-recorded/" + requestID),
			StreamID:      StreamID(corpusevents.DocumentStream(body.DocumentID)),
			CorrelationID: CorrelationID("document-submission/" + requestID),
			OccurredAt:    time.Now().UTC(),
			Body:          body,
		}})
	})
	if err != nil {
		return DocumentReceipt{}, errors.Join(err, prepared.Discard())
	}
	prepared.Confirm()
	return DocumentReceipt{
		DocumentID:    saved.Document.ID,
		ContentDigest: saved.Document.Digest,
	}, nil
}
