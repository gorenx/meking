package report

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"
)

type memoryStore struct {
	reports map[ID]Report
}

func newMemoryStore() *memoryStore {
	return &memoryStore{reports: make(map[ID]Report)}
}

func (s *memoryStore) Insert(_ context.Context, reports []Report) error {
	for _, report := range reports {
		if _, found := s.reports[report.ID]; found {
			return ErrReportConflict
		}
	}
	for _, report := range reports {
		s.reports[report.ID] = report
	}
	return nil
}

func (s *memoryStore) LoadReports(_ context.Context, ids []ID) ([]Report, error) {
	reports := make([]Report, len(ids))
	for index, id := range ids {
		report, found := s.reports[id]
		if !found {
			return nil, ErrReportNotFound
		}
		reports[index] = report
	}
	return reports, nil
}

func TestArchiveSavesAndReadsCompleteBatch(t *testing.T) {
	store := newMemoryStore()
	archive, err := NewArchive(store, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewArchive() error = %v", err)
	}
	generator, err := NewGenerator(
		&scriptedModel{},
		codePointCounter{},
		fixtureConfig(),
	)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	reports, err := generator.Generate(t.Context(), fixtureInput(t))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := archive.Save(t.Context(), reports); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	batch, err := archive.Reports(t.Context(), []ID{reports[1].ID, reports[0].ID})
	if err != nil {
		t.Fatalf("Reports() error = %v", err)
	}
	if !reflect.DeepEqual(batch, []Report{reports[1], reports[0]}) {
		t.Fatalf("Reports() = %#v", batch)
	}
	for _, want := range reports {
		got, err := archive.Report(t.Context(), want.ID)
		if err != nil {
			t.Fatalf("Report(%q) error = %v", want.ID, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Report(%q) = %#v, want %#v", want.ID, got, want)
		}
	}
	if err := archive.Save(t.Context(), reports); !errors.Is(err, ErrReportConflict) {
		t.Fatalf("duplicate Save() error = %v, want ErrReportConflict", err)
	}
}

func TestArchiveRejectsInvalidBatchBeforeStore(t *testing.T) {
	store := newMemoryStore()
	archive, err := NewArchive(store, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewArchive() error = %v", err)
	}
	if err := archive.Save(t.Context(), nil); !errors.Is(err, ErrInvalidReport) {
		t.Fatalf("Save(nil) error = %v, want ErrInvalidReport", err)
	}
	generator, err := NewGenerator(
		&scriptedModel{},
		codePointCounter{},
		fixtureConfig(),
	)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	reports, err := generator.Generate(t.Context(), fixtureInput(t))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := archive.Save(
		t.Context(),
		[]Report{reports[0], reports[0]},
	); !errors.Is(err, ErrInvalidReport) {
		t.Fatalf("Save(duplicate batch ID) error = %v, want ErrInvalidReport", err)
	}
	invalid := reports[0]
	invalid.Title = ""
	if err := archive.Save(
		t.Context(),
		[]Report{invalid},
	); !errors.Is(err, ErrInvalidReport) {
		t.Fatalf("Save(invalid Report) error = %v, want ErrInvalidReport", err)
	}
	if len(store.reports) != 0 {
		t.Fatalf("Store changed after invalid batch: %#v", store.reports)
	}
}
