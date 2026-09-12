package adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/epoch"
	querybase "github.com/memoria-space/meking/query"
)

const (
	testQueryStructureID = community.StructureID("22222222-2222-4222-8222-222222222222")
	testQueryReportSetID = communityreport.ReportSetID("33333333-3333-4333-8333-333333333333")
)

type epochProvider struct {
	current epoch.Epoch
	err     error
}

func (provider epochProvider) Current(context.Context) (epoch.Epoch, error) {
	return provider.current, provider.err
}

func (provider epochProvider) Epoch(context.Context, epoch.ID) (epoch.Epoch, error) {
	return provider.current, provider.err
}

type reportPublicationProvider struct {
	publication communityreport.Publication
	err         error
}

func (provider reportPublicationProvider) LoadReportPublication(
	context.Context,
	community.StructureID,
) (communityreport.Publication, error) {
	return provider.publication, provider.err
}

func TestEpochReaderProjectsCompletedReportPublication(t *testing.T) {
	publishedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	reader, err := NewEpochReader(
		epochProvider{current: epoch.Epoch{
			ID:          4,
			CorporaID:   "11111111-1111-4111-8111-111111111111",
			StructureID: epoch.StructureID(testQueryStructureID),
			PublishedAt: publishedAt,
		}},
		reportPublicationProvider{publication: communityreport.Publication{
			EpochID:      4,
			StructureID:  testQueryStructureID,
			ReportSetID:  testQueryReportSetID,
			VectorsReady: true,
			CreatedAt:    publishedAt,
		}},
	)
	if err != nil {
		t.Fatalf("NewEpochReader() error = %v", err)
	}
	current, err := reader.Current(t.Context())
	want := querybase.Epoch{
		ID:          4,
		CorporaID:   "11111111-1111-4111-8111-111111111111",
		StructureID: string(testQueryStructureID),
		ReportSetID: string(testQueryReportSetID),
	}
	if err != nil || !current.Equal(want) {
		t.Fatalf("Current() = (%#v, %v)", current, err)
	}
}

func TestEpochReaderLeavesReportsEmptyWhileGenerationLags(t *testing.T) {
	reader, err := NewEpochReader(
		epochProvider{current: epoch.Epoch{
			ID:          4,
			CorporaID:   "11111111-1111-4111-8111-111111111111",
			StructureID: epoch.StructureID(testQueryStructureID),
			PublishedAt: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
		}},
		reportPublicationProvider{err: communityreport.ErrPublicationNotFound},
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := reader.Current(t.Context())
	if err != nil || current.ReportSetID != "" || current.StructureID != string(testQueryStructureID) {
		t.Fatalf("Current() = (%#v, %v)", current, err)
	}
}

func TestEpochReaderMapsMissingEpoch(t *testing.T) {
	reader, err := NewEpochReader(
		epochProvider{err: epoch.ErrEpochNotFound},
		reportPublicationProvider{},
	)
	if err != nil {
		t.Fatalf("NewEpochReader() error = %v", err)
	}
	if _, err := reader.Current(t.Context()); !errors.Is(err, querybase.ErrNoEpoch) {
		t.Fatalf("Current() error = %v, want ErrNoEpoch", err)
	}
	if _, err := reader.Epoch(t.Context(), 1); !errors.Is(err, querybase.ErrEpochNotFound) {
		t.Fatalf("Epoch() error = %v, want ErrEpochNotFound", err)
	}
}

func TestEpochReaderRequiresProviders(t *testing.T) {
	if _, err := NewEpochReader(nil, reportPublicationProvider{}); err == nil {
		t.Fatal("NewEpochReader(nil, publications) error = nil")
	}
	if _, err := NewEpochReader(epochProvider{}, nil); err == nil {
		t.Fatal("NewEpochReader(epochs, nil) error = nil")
	}
}
