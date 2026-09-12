package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/memoria-space/meking/knowledge/extraction"
)

func TestKnowledgeExtractionProgressAdvancesByTextUnit(t *testing.T) {
	database, transactions, ctx := openKnowledgeDatabase(t)
	if _, found, err := database.ExtractionProgress(ctx, "corpora"); err != nil || found {
		t.Fatalf("initial progress = found %t, error %v", found, err)
	}
	if err := transactions.WithTx(ctx, func(ctx context.Context) error {
		return database.AdvanceExtraction(ctx, "corpora", "", "one")
	}); err != nil {
		t.Fatal(err)
	}
	if err := transactions.WithTx(ctx, func(ctx context.Context) error {
		return database.AdvanceExtraction(ctx, "corpora", "one", "two")
	}); err != nil {
		t.Fatal(err)
	}
	last, found, err := database.ExtractionProgress(ctx, "corpora")
	if err != nil || !found || last != "two" {
		t.Fatalf("progress = %q, %t, %v", last, found, err)
	}
	if err := transactions.WithTx(ctx, func(ctx context.Context) error {
		return database.AdvanceExtraction(ctx, "corpora", "one", "three")
	}); !errors.Is(err, extraction.ErrExtractionProgress) {
		t.Fatalf("stale advance error = %v", err)
	}
}
