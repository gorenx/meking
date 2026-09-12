package corpus

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus/document"
)

// DocumentRegistration identifies content already present in the configured
// ContentStore when a local observation is made.
type DocumentRegistration struct {
	Name      string
	MediaType string
	Location  document.Location
}

// RegisteredDocument identifies the existing or newly created Document found
// for one ContentStore location.
type RegisteredDocument struct {
	DocumentID      document.ID
	ContentDigest   string
	DocumentCreated bool
}

// RegisterDocument accepts content already owned by ContentStore without
// selecting it for any Zone build.
func (s *Service) RegisterDocument(
	ctx context.Context,
	registration DocumentRegistration,
) (RegisteredDocument, error) {
	if s == nil || s.documents == nil {
		return RegisteredDocument{}, errors.New("Corpus Document registrations are not configured")
	}
	if registration.Location == "" {
		return RegisteredDocument{}, errors.New("register Corpus Document: Location is required")
	}
	_, maximumBytes, err := s.extractors.Resolve(registration.Name, registration.MediaType)
	if err != nil {
		return RegisteredDocument{}, fmt.Errorf("select Corpus Document Extractor: %w", err)
	}
	located, err := s.documents.PrepareExisting(ctx, document.StoredContent{
		Name: registration.Name, MediaType: registration.MediaType,
		Location: registration.Location, MaximumBytes: maximumBytes,
	})
	if err != nil {
		return RegisteredDocument{}, fmt.Errorf("prepare existing Corpus Document: %w", err)
	}
	saved, err := s.documents.Record(ctx, located)
	if err != nil {
		return RegisteredDocument{}, fmt.Errorf("record existing Corpus Document: %w", err)
	}
	return RegisteredDocument{
		DocumentID: saved.Document.ID, ContentDigest: saved.Document.Digest,
		DocumentCreated: saved.DocumentCreated,
	}, nil
}
