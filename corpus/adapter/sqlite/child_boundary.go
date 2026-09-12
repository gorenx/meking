package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"
	corpusmerger "github.com/memoria-space/meking/corpus/zonemerger"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

var _ corpusmerger.ChildCorporaStore = (*Store)(nil)

func (s *Store) ChildCorporaBoundary(
	ctx context.Context,
) (corpusmerger.ChildCorporaBoundary, bool, error) {
	executor, err := transactionsqlite.Current(ctx, s.database)
	if err != nil {
		return corpusmerger.ChildCorporaBoundary{}, false, fmt.Errorf("join Corpus Child boundary transaction: %w", err)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return corpusmerger.ChildCorporaBoundary{}, false, err
	}
	childZoneID, err := zone.RequireChildID(ctx)
	if err != nil || childZoneID == zoneID {
		return corpusmerger.ChildCorporaBoundary{}, false, corpus.ErrInvalidCorpus
	}
	stored, err := newStatements(executor).GetChildCorporaBoundary(ctx,
		db.GetChildCorporaBoundaryParams{
			ZoneID: string(zoneID), ChildZoneID: string(childZoneID),
		})
	if errors.Is(err, sql.ErrNoRows) {
		return corpusmerger.ChildCorporaBoundary{}, false, nil
	}
	if err != nil {
		return corpusmerger.ChildCorporaBoundary{}, false, fmt.Errorf("read Corpus Child boundary: %w", err)
	}
	sourceCorporaID, err := corpus.NewCorporaID(stored)
	if err != nil {
		return corpusmerger.ChildCorporaBoundary{}, false, corpus.ErrCorpusDataIntegrity
	}
	boundary, err := corpusmerger.NewChildCorporaBoundary(sourceCorporaID)
	if err != nil {
		return corpusmerger.ChildCorporaBoundary{}, false, corpus.ErrCorpusDataIntegrity
	}
	return boundary, true, nil
}

func (s *Store) SaveChildCorporaBoundary(
	ctx context.Context,
	boundary corpusmerger.ChildCorporaBoundary,
) error {
	executor, err := transactionsqlite.Current(ctx, s.database)
	if err != nil {
		return fmt.Errorf("join Corpus Child boundary transaction: %w", err)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	childZoneID, err := zone.RequireChildID(ctx)
	if err != nil || childZoneID == zoneID {
		return corpus.ErrInvalidCorpus
	}
	sourceSequence, err := corpus.CorporaIDSequence(boundary.SourceCorporaID())
	if err != nil {
		return err
	}
	if err := newStatements(executor).SaveChildCorporaBoundary(ctx,
		db.SaveChildCorporaBoundaryParams{
			ZoneID:          string(zoneID),
			ChildZoneID:     string(childZoneID),
			SourceCorporaID: sourceSequence,
		}); err != nil {
		return fmt.Errorf("save Corpus Child boundary: %w", err)
	}
	return nil
}

func (s *Store) ChildCorporaBoundaries(ctx context.Context) (map[string]corpus.CorporaID, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, err
	}
	executor := transactionsqlite.Executor(s.database)
	if current, currentErr := transactionsqlite.Current(ctx, s.database); currentErr == nil {
		executor = current
	}
	rows, err := newStatements(executor).ListChildCorporaBoundaries(ctx, string(zoneID))
	if err != nil {
		return nil, fmt.Errorf("list Corpus Child boundaries: %w", err)
	}
	result := make(map[string]corpus.CorporaID, len(rows))
	for _, row := range rows {
		corporaID, err := corpus.NewCorporaID(row.SourceCorporaID)
		if err != nil {
			return nil, corpus.ErrCorpusDataIntegrity
		}
		result[row.ChildZoneID] = corporaID
	}
	return result, nil
}
