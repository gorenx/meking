package zonemerger

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/zone"
)

type ChildCorporaStore interface {
	ChildCorporaBoundary(ctx context.Context) (ChildCorporaBoundary, bool, error)
	SaveChildCorporaBoundary(ctx context.Context, boundary ChildCorporaBoundary) error
}

type BoundaryApplication struct {
	transactions corpus.Transaction
	store        ChildCorporaStore
}

func NewBoundaryApplication(
	transactions corpus.Transaction,
	store ChildCorporaStore,
) (*BoundaryApplication, error) {
	if transactions == nil || store == nil {
		return nil, errors.New("create Corpus Child Boundary Application: Transactions and Store are required")
	}
	return &BoundaryApplication{transactions: transactions, store: store}, nil
}

func (application *BoundaryApplication) MergeChildCorpora(
	ctx context.Context,
	sourceCorporaID string,
) error {
	if application == nil {
		return errors.New("Corpus Child Boundary Application is not configured")
	}
	if _, err := zone.RequireChildID(ctx); err != nil {
		return fmt.Errorf("resolve Corpus Child Zone: %w", err)
	}
	parsed, err := corpus.ParseCorporaID(sourceCorporaID)
	if err != nil {
		return fmt.Errorf("invalid Child Corpora boundary: %w", err)
	}
	incoming, err := NewChildCorporaBoundary(parsed)
	if err != nil {
		return fmt.Errorf("invalid Child Corpora boundary: %w", err)
	}
	if err = application.transactions.WithTx(ctx, func(ctx context.Context) error {
		stored, found, err := application.store.ChildCorporaBoundary(ctx)
		if err != nil {
			return err
		}
		if !found {
			return application.store.SaveChildCorporaBoundary(ctx, incoming)
		}
		merged, changed, err := stored.Merge(parsed)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return application.store.SaveChildCorporaBoundary(ctx, merged)
	}); err != nil {
		return fmt.Errorf("merge Child Corpus boundary: %w", err)
	}
	return nil
}
