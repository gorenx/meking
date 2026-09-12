package adapter_test

import (
	"context"
	"errors"
	"github.com/memoria-space/meking/corpus/textunits"
	"testing"

	"github.com/memoria-space/meking/corpus"
	querybase "github.com/memoria-space/meking/query"
	querybasicadapter "github.com/memoria-space/meking/query/basic/adapter"
)

const adapterCorporaID corpus.CorporaID = "20000000-0000-4000-8000-000000000001"

func TestCorpusReaderCachesExactTokenizer(t *testing.T) {
	catalog := &corpusCatalogStub{
		chunking: textunits.Chunking{
			Type: textunits.TokenChunking, Size: 300, Overlap: 20,
			EncodingModel: "o200k_base",
		},
	}
	reader, err := querybasicadapter.NewCorpusReader(catalog)
	if err != nil {
		t.Fatalf("NewCorpusReader() error = %v", err)
	}
	first, err := reader.Count(t.Context(), string(adapterCorporaID), "first")
	if err != nil || first <= 0 {
		t.Fatalf("Count(first) = %d, %v", first, err)
	}
	second, err := reader.Count(t.Context(), string(adapterCorporaID), "second")
	if err != nil || second <= 0 || catalog.chunkingCalls != 1 {
		t.Fatalf("Count(second) = %d, %v; Chunking calls = %d", second, err, catalog.chunkingCalls)
	}
}

func TestCorpusReaderClassifiesMissingExactChunking(t *testing.T) {
	tests := []struct {
		name     string
		catalog  *corpusCatalogStub
		category querybase.FailureCategory
	}{
		{
			name: "missing exact Chunking", catalog: &corpusCatalogStub{
				chunkingErr: corpus.ErrCorporaNotFound,
			}, category: querybase.FailurePublicationIncomplete,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader, _ := querybasicadapter.NewCorpusReader(test.catalog)
			_, err := reader.Count(t.Context(), string(adapterCorporaID), "text")
			var failure *querybase.Failure
			if !errors.As(err, &failure) || failure.Category != test.category {
				t.Fatalf("error = %#v, want %q", err, test.category)
			}
		})
	}
}

type corpusCatalogStub struct {
	chunking      textunits.Chunking
	chunkingErr   error
	chunkingCalls int
}

func (s *corpusCatalogStub) CorporaChunking(
	_ context.Context,
	_ corpus.CorporaID,
) (textunits.Chunking, error) {
	s.chunkingCalls++
	return s.chunking, s.chunkingErr
}
